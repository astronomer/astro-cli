package fromfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"sort"
	"strings"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/ghodss/yaml"
)

var (
	errEmptyFile                      = errors.New("has no content")
	errCreateFailed                   = errors.New("failed to create deployment with input")
	errUpdateFailed                   = errors.New("failed to update deployment with input")
	errRequiredField                  = errors.New("missing required field")
	errInvalidEmail                   = errors.New("invalid email")
	errCannotUpdateExistingDeployment = errors.New("already exists")
	errNotFound                       = errors.New("does not exist")
	errInvalidValue                   = errors.New("is not valid")
	errNotPermitted                   = errors.New("is not permitted")
	errNoUseWorkerQueues              = errors.New("don't use 'worker_queues' to update default queue with KubernetesExecutor, use 'default_task_pod_cpu' and 'default_task_pod_memory' instead")
	errUseDefaultWokerType            = errors.New("don't use 'worker_queues' to update default queue with KubernetesExecutor, use 'default_worker_type' instead")

	canCiCdDeploy = deployment.CanCiCdDeploy
)

const (
	jsonFormat   = "json"
	createAction = "create"
	updateAction = "update"
	defaultQueue = "default"
)

// CreateOrUpdate takes a file and creates a deployment with the confiuration specified in the file.
// inputFile can be in yaml or json format
// It returns an error if any required information is missing or incorrectly specified.
func CreateOrUpdate(inputFile, action string, astroPlatformCore astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error { //nolint
	var (
		err                                           error
		errHelp, clusterID, workspaceID, outputFormat string
		dataBytes                                     []byte
		formattedDeployment                           inspect.FormattedDeployment
		existingDeployment                            astroplatformcore.Deployment
		existingDeployments                           []astroplatformcore.Deployment
		nodePools                                     []astroplatformcore.NodePool
		jsonOutput                                    bool
		dagDeploy                                     bool
		envVars                                       []astroplatformcore.DeploymentEnvironmentVariableRequest
	)

	// get file contents as []byte
	dataBytes, err = os.ReadFile(inputFile)
	if err != nil {
		return err
	}
	// return errEmptyFile if we have no dataBytes
	if len(dataBytes) == 0 {
		return fmt.Errorf("%s %w", inputFile, errEmptyFile)
	}
	// set outputFormat if json
	jsonOutput = isJSON(dataBytes)
	// unmarshal to a formattedDeployment
	err = yaml.Unmarshal(dataBytes, &formattedDeployment)
	if err != nil {
		return err
	}
	// validate required fields
	err = checkRequiredFields(&formattedDeployment, action)
	if err != nil {
		return err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	deploymentType := transformDeploymentType(formattedDeployment.Deployment.Configuration.DeploymentType)
	if !deployment.IsDeploymentStandard(deploymentType) {
		// map cluster name to id and collect node pools for cluster
		clusterID, nodePools, err = getClusterInfoFromName(formattedDeployment.Deployment.Configuration.ClusterName, c.OrganizationShortName, astroPlatformCore)
		if err != nil {
			return err
		}
	}

	existingDeployments, err = deployment.CoreGetDeployments(workspaceID, c.Organization, astroPlatformCore)
	if err != nil {
		return err
	}
	if action == createAction {
		// map workspace name to id
		workspaceID, err = getWorkspaceIDFromName(formattedDeployment.Deployment.Configuration.WorkspaceName, c.Organization, coreClient)
		if err != nil {
			return err
		}
		// get correct value for dag deploy
		if formattedDeployment.Deployment.Configuration.DagDeployEnabled == nil { //nolint:staticcheck
			if organization.IsOrgHosted() {
				dagDeploy = true
			} else {
				dagDeploy = false
			}
		} else {
			dagDeploy = *formattedDeployment.Deployment.Configuration.DagDeployEnabled
		}
		// check if deployment exists
		if deploymentExists(existingDeployments, formattedDeployment.Deployment.Configuration.Name) {
			// create does not allow updating existing deployments
			errHelp = fmt.Sprintf("use deployment update --deployment-file %s instead", inputFile)
			return fmt.Errorf("deployment: %s %w: %s", formattedDeployment.Deployment.Configuration.Name,
				errCannotUpdateExistingDeployment, errHelp)
		}
		// this deployment does not exist so create it
		// transform formattedDeployment to DeploymentCreateInput
		err = createOrUpdateDeployment(&formattedDeployment, clusterID, workspaceID, createAction, &astroplatformcore.Deployment{}, nodePools, dagDeploy, envVars, astroPlatformCore)
		if err != nil {
			return err
		}
		if hasEnvVarsOrAlertEmails(&formattedDeployment) {
			action = updateAction
		}
	}
	// get new existing deployments list
	existingDeployments, err = deployment.CoreGetDeployments(workspaceID, c.Organization, astroPlatformCore)
	if err != nil {
		return err
	}
	if action == updateAction {
		// check if deployment does not exist
		if !deploymentExists(existingDeployments, formattedDeployment.Deployment.Configuration.Name) {
			// update does not allow creating new deployments
			errHelp = fmt.Sprintf("use deployment create --deployment-file %s instead", inputFile)
			return fmt.Errorf("deployment: %s %w: %s", formattedDeployment.Deployment.Configuration.Name,
				errNotFound, errHelp)
		}
		// this deployment exists so update it
		existingDeployment = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name)
		workspaceID = existingDeployment.WorkspaceId
		// determine dagDeploy
		if formattedDeployment.Deployment.Configuration.DagDeployEnabled == nil { //nolint:staticcheck
			dagDeploy = existingDeployment.IsDagDeployEnabled
		} else {
			dagDeploy = *formattedDeployment.Deployment.Configuration.DagDeployEnabled
		}
		if !deployment.IsDeploymentStandard(deploymentType) {
			clusterID = *existingDeployment.ClusterId
		}
		// create environment variables
		envVars = createEnvVarsRequest(&formattedDeployment)
		if err != nil {
			return fmt.Errorf("%w \n failed to %s alert emails", err, action)
		}
		// transform formattedDeployment to DeploymentUpdateInput
		err = createOrUpdateDeployment(&formattedDeployment, clusterID, workspaceID, updateAction, &existingDeployment, nodePools, dagDeploy, envVars, astroPlatformCore)
		if err != nil {
			return err
		}
	}
	// Get deployment created or updated
	existingDeployment = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name)
	if jsonOutput {
		outputFormat = jsonFormat
	}
	return inspect.Inspect(workspaceID, "", existingDeployment.Id, outputFormat, astroPlatformCore, coreClient, out, "", false)
}

// createOrUpdateDeployment transforms an inspect.FormattedDeployment into astroplateformcore CreateDeploymentInput or
// UpdateDeploymentInput based on the action requested. Then the deployment is created or updated
// If worker-queues were requested, it gets node pool id work the workers and validates queue options.
// If no queue options were specified, it sets default values.
// It returns an error if getting default options fail.
// It returns an error if worker-queue options are not valid.
// It returns an error if node pool id could not be found for the worker type.
//
//nolint:dupl
func createOrUpdateDeployment(deploymentFromFile *inspect.FormattedDeployment, clusterID, workspaceID, action string, existingDeployment *astroplatformcore.Deployment, nodePools []astroplatformcore.NodePool, dagDeploy bool, envVars []astroplatformcore.DeploymentEnvironmentVariableRequest, astroPlatformCore astroplatformcore.CoreClient) error { //nolint
	var (
		defaultOptions          astroplatformcore.WorkerQueueOptions
		configOptions           astroplatformcore.DeploymentOptions
		listQueues              []astroplatformcore.WorkerQueue
		astroMachine            astroplatformcore.WorkerMachine
		listQueuesRequest       []astroplatformcore.WorkerQueueRequest
		listHybridQueuesRequest []astroplatformcore.HybridWorkerQueueRequest
		err                     error
	)
	deploymentType := transformDeploymentType(deploymentFromFile.Deployment.Configuration.DeploymentType)

	// Add worker queues if they were requested
	if hasQueues(deploymentFromFile) {
		// transform inspect.WorkerQ to []astro.WorkerQueue
		if existingDeployment.WorkerQueues != nil {
			listQueues = *existingDeployment.WorkerQueues
		}
		listQueues, err = getQueues(deploymentFromFile, nodePools, listQueues)
		if err != nil {
			return err
		}
		// get defaults for min-count, max-count and concurrency from API
		deploymentOptions, err := deployment.GetPlatformDeploymentOptions("", astroplatformcore.GetDeploymentOptionsParams{}, astroPlatformCore)
		if err != nil {
			return err
		}

		defaultOptions = deploymentOptions.WorkerQueues
		configOptions = deploymentOptions
		for i := range listQueues {
			if deployment.IsDeploymentStandard(deploymentType) || deployment.IsDeploymentDedicated(deploymentType) {
				astroMachines := configOptions.WorkerMachines
				for j := range astroMachines {
					if string(astroMachines[j].Name) == *listQueues[i].AstroMachine {
						astroMachine = astroMachines[j]
					}
				}
				var workerQueue astroplatformcore.WorkerQueueRequest
				workerQueue.Id = &listQueues[i].Id
				workerQueue.Name = listQueues[i].Name
				workerQueue.IsDefault = listQueues[i].IsDefault
				workerQueue.MaxWorkerCount = listQueues[i].MaxWorkerCount
				workerQueue.MinWorkerCount = listQueues[i].MinWorkerCount
				workerQueue.WorkerConcurrency = listQueues[i].WorkerConcurrency
				workerQueue.AstroMachine = astroplatformcore.WorkerQueueRequestAstroMachine(*listQueues[i].AstroMachine)
				// set default values if none were specified
				a := workerqueue.SetWorkerQueueValues(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, &workerQueue, defaultOptions)
				// check if queue is valid
				if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
					return errNoUseWorkerQueues
				}
				err = workerqueue.IsHostedCeleryWorkerQueueInputValid(a, defaultOptions, &astroMachine)
				if err != nil {
					return err
				}
				// add it to the list of queues to be created
				listQueuesRequest = append(listQueuesRequest, *a)
			} else {
				var workerQueue astroplatformcore.HybridWorkerQueueRequest
				workerQueue.Id = &listQueues[i].Id
				workerQueue.Name = listQueues[i].Name
				workerQueue.IsDefault = listQueues[i].IsDefault
				workerQueue.MaxWorkerCount = listQueues[i].MaxWorkerCount
				workerQueue.MinWorkerCount = listQueues[i].MinWorkerCount
				workerQueue.WorkerConcurrency = listQueues[i].WorkerConcurrency
				workerQueue.NodePoolId = *listQueues[i].NodePoolId
				// check if queue is valid
				if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
					return errUseDefaultWokerType
				}
				// set default values if none were specified
				a := workerqueue.SetWorkerQueueValuesHybrid(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, &workerQueue, defaultOptions)
				err = workerqueue.IsCeleryWorkerQueueInputValid(a, defaultOptions)
				if err != nil {
					return err
				}
				workerQueue = *a
				// add it to the list of queues to be created
				listHybridQueuesRequest = append(listHybridQueuesRequest, workerQueue)
			}
		}
	}
	switch action {
	case createAction:
		createDeploymentRequest := astroplatformcore.CreateDeploymentRequest{}
		if deployment.IsDeploymentStandard(deploymentType) {
			var requestedCloudProvider astroplatformcore.CreateStandardDeploymentRequestCloudProvider
			if deploymentFromFile.Deployment.Configuration.CloudProvider == "gcp" || deploymentFromFile.Deployment.Configuration.CloudProvider == "GCP" {
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderGCP
			}
			if deploymentFromFile.Deployment.Configuration.CloudProvider == "aws" || deploymentFromFile.Deployment.Configuration.CloudProvider == "AWS" {
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAWS
			}
			if deploymentFromFile.Deployment.Configuration.CloudProvider == "azure" || deploymentFromFile.Deployment.Configuration.CloudProvider == "AZURE" {
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAZURE
			}
			var requestedExecutor astroplatformcore.CreateStandardDeploymentRequestExecutor
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.CELERY {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorCELERY
			}
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorKUBERNETES
			}
			var schedulerSize astroplatformcore.CreateStandardDeploymentRequestSchedulerSize
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SmallScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SMALL {
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeSMALL
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MediumScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MEDIUM {
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LargeScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LARGE {
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeLARGE
			}
			standardDeploymentRequest := astroplatformcore.CreateStandardDeploymentRequest{
				AstroRuntimeVersion:  deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				CloudProvider:        &requestedCloudProvider,
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				Region:               &deploymentFromFile.Deployment.Configuration.Region,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateStandardDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU,
				DefaultTaskPodMemory: deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory,
				ResourceQuotaCpu:     deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU,
				ResourceQuotaMemory:  deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
			}
			err := createDeploymentRequest.FromCreateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.CreateDedicatedDeploymentRequestExecutor
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.CELERY {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorCELERY
			}
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			var schedulerSize astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSize
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SmallScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SMALL {
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeSMALL
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MediumScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MEDIUM {
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LargeScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LARGE {
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeLARGE
			}
			dedicatedDeploymentRequest := astroplatformcore.CreateDedicatedDeploymentRequest{
				AstroRuntimeVersion:  deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				ClusterId:            clusterID,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU,
				DefaultTaskPodMemory: deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory,
				ResourceQuotaCpu:     deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU,
				ResourceQuotaMemory:  deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
			}
			err := createDeploymentRequest.FromCreateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if !deployment.IsDeploymentStandard(deploymentType) && !deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.CreateHybridDeploymentRequestExecutor
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.CELERY {
				requestedExecutor = astroplatformcore.CreateHybridDeploymentRequestExecutorCELERY
			}
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
				requestedExecutor = astroplatformcore.CreateHybridDeploymentRequestExecutorKUBERNETES
			}
			hybridDeploymentRequest := astroplatformcore.CreateHybridDeploymentRequest{
				AstroRuntimeVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				Description:         &deploymentFromFile.Deployment.Configuration.Description,
				Name:                deploymentFromFile.Deployment.Configuration.Name,
				Executor:            requestedExecutor,
				IsCicdEnforced:      deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:  dagDeploy,
				ClusterId:           clusterID,
				WorkspaceId:         workspaceID,
				Scheduler: astroplatformcore.DeploymentInstanceSpecRequest{
					Au:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
				Type:         astroplatformcore.CreateHybridDeploymentRequestTypeHYBRID,
				WorkerQueues: &listHybridQueuesRequest,
			}
			if requestedExecutor == astroplatformcore.CreateHybridDeploymentRequestExecutorKUBERNETES {
				// map worker type to node pool id
				taskPodNodePoolID, err := getNodePoolIDFromWorkerType(deploymentFromFile.Deployment.Configuration.DefaultWorkerType, deploymentFromFile.Deployment.Configuration.ClusterName, nodePools)
				if err != nil {
					return err
				}
				hybridDeploymentRequest.TaskPodNodePoolId = &taskPodNodePoolID
			}
			err := createDeploymentRequest.FromCreateHybridDeploymentRequest(hybridDeploymentRequest)
			if err != nil {
				return err
			}
		}

		_, err := deployment.CoreCreateDeployment("", createDeploymentRequest, astroPlatformCore)
		if err != nil {
			return err
		}

	case updateAction:
		// build query input
		updateDeploymentRequest := astroplatformcore.UpdateDeploymentRequest{}
		// check if cluster is being changed
		if existingDeployment.ClusterId != nil {
			if clusterID != *existingDeployment.ClusterId {
				return fmt.Errorf("changing an existing deployment's cluster %w", errNotPermitted)
			}
		}
		if deployment.IsDeploymentStandard(deploymentType) {
			var requestedExecutor astroplatformcore.UpdateStandardDeploymentRequestExecutor
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.CELERY {
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorCELERY
			}
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorKUBERNETES
			}
			var schedulerSize astroplatformcore.UpdateStandardDeploymentRequestSchedulerSize
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SmallScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SMALL {
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeSMALL
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MediumScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MEDIUM {
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeMEDIUM
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LargeScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LARGE {
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeLARGE
			}
			standardDeploymentRequest := astroplatformcore.UpdateStandardDeploymentRequest{
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.UpdateStandardDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU,
				DefaultTaskPodMemory: deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory,
				ResourceQuotaCpu:     deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU,
				ResourceQuotaMemory:  deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
			}
			err := updateDeploymentRequest.FromUpdateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.UpdateDedicatedDeploymentRequestExecutor
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.CELERY {
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorCELERY
			}
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			var schedulerSize astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSize
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SmallScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.SMALL {
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeSMALL
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MediumScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.MEDIUM {
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			}
			if deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LargeScheduler || deploymentFromFile.Deployment.Configuration.SchedulerSize == deployment.LARGE {
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeLARGE
			}
			dedicatedDeploymentRequest := astroplatformcore.UpdateDedicatedDeploymentRequest{
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.UpdateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU,
				DefaultTaskPodMemory: deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory,
				ResourceQuotaCpu:     deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU,
				ResourceQuotaMemory:  deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
			}
			err := updateDeploymentRequest.FromUpdateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if !deployment.IsDeploymentStandard(deploymentType) && !deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.UpdateHybridDeploymentRequestExecutor
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.CELERY {
				requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorCELERY
			}
			if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
				requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorKUBERNETES
			}
			hybridDeploymentRequest := astroplatformcore.UpdateHybridDeploymentRequest{
				Description:        &deploymentFromFile.Deployment.Configuration.Description,
				Name:               deploymentFromFile.Deployment.Configuration.Name,
				Executor:           requestedExecutor,
				IsCicdEnforced:     deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled: dagDeploy,
				WorkspaceId:        workspaceID,
				Scheduler: astroplatformcore.DeploymentInstanceSpecRequest{
					Au:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
				Type:                 astroplatformcore.UpdateHybridDeploymentRequestTypeHYBRID,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
				WorkerQueues:         &listHybridQueuesRequest,
			}
			if requestedExecutor == astroplatformcore.UpdateHybridDeploymentRequestExecutorKUBERNETES {
				taskPodNodePoolID, err := getNodePoolIDFromWorkerType(deploymentFromFile.Deployment.Configuration.DefaultWorkerType, deploymentFromFile.Deployment.Configuration.ClusterName, nodePools)
				if err != nil {
					return err
				}
				hybridDeploymentRequest.TaskPodNodePoolId = &taskPodNodePoolID
			}
			err := updateDeploymentRequest.FromUpdateHybridDeploymentRequest(hybridDeploymentRequest)
			if err != nil {
				return err
			}
		}
		// update deployment
		if deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments && dagDeploy {
			c, err := config.GetCurrentContext()
			if err != nil {
				return err
			}
			if !canCiCdDeploy(c.Token) {
				fmt.Printf("\nWarning: You are trying to update dag deploy setting on a deployment with ci-cd enforcement enabled. You will not be able to deploy your dags using the CLI and that dags will not be visible in the UI and new tasks will not start." +
					"\nEither disable ci-cd enforcement or please cancel this operation and use API Tokens or API Keys instead.")
				y, _ := input.Confirm("\n\nAre you sure you want to continue?")

				if !y {
					fmt.Println("Canceling Deployment update")
					return nil
				}
			}
		}
		_, err := deployment.CoreUpdateDeployment("", existingDeployment.Id, updateDeploymentRequest, astroPlatformCore)
		if err != nil {
			return err
		}
	}
	return nil
}

// checkRequiredFields ensures all required fields are present in inspect.FormattedDeployment.
// It returns errRequiredField if required fields are missing, errInvalidValue if values are not valid and nil if not.
func checkRequiredFields(deploymentFromFile *inspect.FormattedDeployment, action string) error {
	if deploymentFromFile.Deployment.Configuration.Name == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.name")
	}
	if deploymentFromFile.Deployment.Configuration.Executor == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.executor")
	}
	if !isValidExecutor(deploymentFromFile.Deployment.Configuration.Executor) {
		return fmt.Errorf("executor %s %w. It can either be CeleryExecutor or KubernetesExecutor", deploymentFromFile.Deployment.Configuration.Executor, errInvalidValue)
	}
	// if alert emails are requested
	if hasAlertEmails(deploymentFromFile) {
		err := checkAlertEmails(deploymentFromFile)
		if err != nil {
			return err
		}
	}
	// if environment variables are requested
	if hasEnvVars(deploymentFromFile) {
		err := checkEnvVars(deploymentFromFile, action)
		if err != nil {
			return err
		}
	}
	// if worker queues were requested check queue name, isDefault and worker type
	if hasQueues(deploymentFromFile) {
		var hasDefaultQueue bool
		for i, queue := range deploymentFromFile.Deployment.WorkerQs {
			if queue.Name == "" {
				missingField := fmt.Sprintf("deployment.worker_queues[%d].name", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
			if queue.Name == defaultQueue {
				hasDefaultQueue = true
			}
			if queue.WorkerType == "" {
				missingField := fmt.Sprintf("deployment.worker_queues[%d].worker_type", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
		}
		if !hasDefaultQueue {
			missingField := "default queue is missing under deployment.worker_queues"
			return fmt.Errorf("%w: %s", errRequiredField, missingField)
		}
	}
	return nil
}

// deploymentExists deploymentToCreate as its argument.
// It returns true if deploymentToCreate exists.
// It returns false if deploymentToCreate does not exist.
func deploymentExists(existingDeployments []astroplatformcore.Deployment, deploymentNameToCreate string) bool {
	for i := range existingDeployments {
		if existingDeployments[i].Name == deploymentNameToCreate {
			// deployment exists
			return true
		}
	}
	return false
}

// deploymentFromName takes existingDeployments and deploymentName as its arguments.
// It returns the existing deployment that matches deploymentName.
func deploymentFromName(existingDeployments []astroplatformcore.Deployment, deploymentName string) astroplatformcore.Deployment {
	for i := range existingDeployments {
		if existingDeployments[i].Name == deploymentName {
			// deployment that matched name
			return existingDeployments[i]
		}
	}
	return astroplatformcore.Deployment{}
}

// getClusterInfoFromName takes clusterName and organizationShortName as its arguments.
// It returns the clusterID and list of nodepools if the cluster is found in the organization.
// It returns an errClusterNotFound if the cluster does not exist in the organization.
func getClusterInfoFromName(clusterName, organizationShortName string, corePlatformClient astroplatformcore.CoreClient) (string, []astroplatformcore.NodePool, error) {
	var (
		existingClusters []astroplatformcore.Cluster
		err              error
	)

	existingClusters, err = organization.ListClusters(organizationShortName, corePlatformClient)
	if err != nil {
		return "", nil, err
	}

	for _, cluster := range existingClusters { //nolint
		if cluster.Name == clusterName {
			return cluster.Id, *cluster.NodePools, nil
		}
	}
	err = fmt.Errorf("cluster_name: %s %w in organization: %s", clusterName, errNotFound, organizationShortName)
	return "", nil, err
}

// getWorkspaceIDFromName takes workspaceName and organizationID as its arguments.
// It returns the workspaceID if the workspace is found in the organization.
// It returns an errWorkspaceNotFound if the workspace does not exist in the organization.
func getWorkspaceIDFromName(workspaceName, organizationID string, client astrocore.CoreClient) (string, error) {
	var (
		existingWorkspaces []astrocore.Workspace
		err                error
	)

	existingWorkspaces, err = workspace.GetWorkspaces(client)
	if err != nil {
		return "", err
	}
	for i := range existingWorkspaces {
		if existingWorkspaces[i].Name == workspaceName {
			return existingWorkspaces[i].Id, nil
		}
	}
	err = fmt.Errorf("workspace_name: %s %w in organization: %s", workspaceName, errNotFound, organizationID)
	return "", err
}

// getNodePoolIDFromWorkerType maps the node pool id in nodePools to a worker type.
// It returns an error if the node pool id does not exist in any node pool in nodePools.
func getNodePoolIDFromWorkerType(workerType, clusterName string, nodePools []astroplatformcore.NodePool) (string, error) {
	if workerType == "" {
		return nodePools[0].Id, nil
	}
	for i := range nodePools { //nolint
		if nodePools[i].NodeInstanceType == workerType {
			return nodePools[i].Id, nil
		}
	}
	err := fmt.Errorf("worker_type: %s %w in cluster: %s", workerType, errNotFound, clusterName)
	return "", err
}

// getQueues takes a deploymentFromFile as its arguments.
// It returns a list of worker queues to be created or updated.
func getQueues(deploymentFromFile *inspect.FormattedDeployment, nodePools []astroplatformcore.NodePool, existingQueues []astroplatformcore.WorkerQueue) ([]astroplatformcore.WorkerQueue, error) {
	var (
		qList      []astroplatformcore.WorkerQueue
		nodePoolID string
		err        error
	)
	requestedQueues := deploymentFromFile.Deployment.WorkerQs
	deploymentType := transformDeploymentType(deploymentFromFile.Deployment.Configuration.DeploymentType)
	// sort existing queues by name
	if len(existingQueues) > 1 {
		sort.Slice(existingQueues, func(i, j int) bool {
			return existingQueues[i].Name < existingQueues[j].Name
		})
	}
	// sort requested queues by name
	if len(requestedQueues) > 1 {
		sort.Slice(requestedQueues, func(i, j int) bool {
			return requestedQueues[i].Name < requestedQueues[j].Name
		})
	}
	qList = make([]astroplatformcore.WorkerQueue, len(requestedQueues))
	for i := range requestedQueues {
		// check if requested queue exists
		if i < len(existingQueues) {
			// add existing queue to list of queues to return
			if requestedQueues[i].Name == existingQueues[i].Name {
				// update existing queue
				qList[i].Name = existingQueues[i].Name
				if deploymentFromFile.Deployment.Configuration.Executor != deployment.KubeExecutor {
					// only add id when executor is Celery
					qList[i].Id = existingQueues[i].Id
				}
			}
		}
		// add new queue or update existing queue properties to list of queues to return
		qList[i].Name = requestedQueues[i].Name
		qList[i].IsDefault = requestedQueues[i].Name == defaultQueue
		qList[i].MinWorkerCount = requestedQueues[i].MinWorkerCount
		qList[i].MaxWorkerCount = requestedQueues[i].MaxWorkerCount
		qList[i].WorkerConcurrency = requestedQueues[i].WorkerConcurrency
		qList[i].WorkerConcurrency = requestedQueues[i].WorkerConcurrency
		if deployment.IsDeploymentDedicated(deploymentType) || deployment.IsDeploymentStandard(deploymentType) {
			qList[i].AstroMachine = &requestedQueues[i].WorkerType
		} else {
			// map worker type to node pool id
			nodePoolID, err = getNodePoolIDFromWorkerType(requestedQueues[i].WorkerType, deploymentFromFile.Deployment.Configuration.ClusterName, nodePools)
			if err != nil {
				return nil, err
			}
			qList[i].NodePoolId = &nodePoolID
		}
	}
	return qList, nil
}

// hasEnvVars returns true if environment variables exist in deploymentFromFile.
// it returns false if they don't.
func hasEnvVars(deploymentFromFile *inspect.FormattedDeployment) bool {
	return len(deploymentFromFile.Deployment.EnvVars) > 0
}

// hasQueues returns true if worker queues exist in deploymentFromFile.
// it returns false if they don't.
func hasQueues(deploymentFromFile *inspect.FormattedDeployment) bool {
	return len(deploymentFromFile.Deployment.WorkerQs) > 0
}

// hasAlertEmails returns true if alert emails exist in deploymentFromFile.
// it returns false if they don't.
func hasAlertEmails(deploymentFromFile *inspect.FormattedDeployment) bool {
	return len(deploymentFromFile.Deployment.AlertEmails) > 0
}

func hasEnvVarsOrAlertEmails(deploymentFromFile *inspect.FormattedDeployment) bool {
	if hasEnvVars(deploymentFromFile) {
		return hasEnvVars(deploymentFromFile)
	}
	if hasAlertEmails(deploymentFromFile) {
		return hasAlertEmails(deploymentFromFile)
	}
	return false
}

// isJSON returns true if data is in JSON format.
// It returns false if not.
func isJSON(data []byte) bool {
	var js interface{}
	return json.Unmarshal(data, &js) == nil
}

// isValidEmail returns true if email is a valid email address.
// It returns false if not.
func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// checkAlertEmails returns an error if email in deploymentFromFile.AlertEmails is not valid.
func checkAlertEmails(deploymentFromFile *inspect.FormattedDeployment) error {
	for _, email := range deploymentFromFile.Deployment.AlertEmails {
		if !isValidEmail(email) {
			return fmt.Errorf("%w: %s", errInvalidEmail, email)
		}
	}
	return nil
}

// checkEnvVars returns an error if either key or value are missing for any deploymentFromFile.Deployment.EnvVars.
func checkEnvVars(deploymentFromFile *inspect.FormattedDeployment, action string) error {
	for i, envVar := range deploymentFromFile.Deployment.EnvVars {
		if envVar.Key == "" {
			missingField := fmt.Sprintf("deployment.environment_variables[%d].key", i)
			return fmt.Errorf("%w: %s", errRequiredField, missingField)
		}
		if action == createAction {
			if envVar.Value == "" {
				missingField := fmt.Sprintf("deployment.environment_variables[%d].value", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
		}
	}
	return nil
}

func createEnvVarsRequest(deploymentFromFile *inspect.FormattedDeployment) (envVars []astroplatformcore.DeploymentEnvironmentVariableRequest) {
	for i := range deploymentFromFile.Deployment.EnvVars {
		var envVar astroplatformcore.DeploymentEnvironmentVariableRequest
		envVar.IsSecret = deploymentFromFile.Deployment.EnvVars[i].IsSecret
		envVar.Key = deploymentFromFile.Deployment.EnvVars[i].Key
		envVar.Value = &deploymentFromFile.Deployment.EnvVars[i].Value
		envVars = append(envVars, envVar)
	}
	return envVars
}

// isValidExecutor returns true for valid executor values and false if not.
func isValidExecutor(executor string) bool {
	return executor == deployment.CeleryExecutor || executor == deployment.KubeExecutor || executor == deployment.CELERY || executor == deployment.KUBERNETES
}

func transformDeploymentType(deploymentType string) astroplatformcore.DeploymentType {
	var transformedDeploymentType astroplatformcore.DeploymentType

	switch strings.ToUpper(deploymentType) {
	case "STANDARD":
		transformedDeploymentType = astroplatformcore.DeploymentTypeSTANDARD
	case "DEDICATED":
		transformedDeploymentType = astroplatformcore.DeploymentTypeDEDICATED
	case "HYBRID":
		transformedDeploymentType = astroplatformcore.DeploymentTypeHYBRID
	}

	return transformedDeploymentType
}
