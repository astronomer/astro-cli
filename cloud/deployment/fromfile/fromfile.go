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
	errUseDefaultWorkerType           = errors.New("don't use 'worker_queues' to update default queue with KubernetesExecutor, use 'default_worker_type' instead")
	errDeploymentsWithSameName        = errors.New("you currently have two deployments with the name specified in the your worksapce. Make sure your deployment's name is unique in the workspace to update it with a deployment file")
	canCiCdDeploy                     = deployment.CanCiCdDeploy
)

const (
	jsonFormat      = "json"
	createAction    = "create"
	updateAction    = "update"
	defaultQueue    = "default"
	HostedDedicated = "HOSTED_DEDICATED"
	HostedStandard  = "HOSTED_STANDARD"
	HostedShared    = "HOSTED_SHARED"
)

// CreateOrUpdate takes a file and creates a deployment with the confiuration specified in the file.
// inputFile can be in yaml or json format
// It returns an error if any required information is missing or incorrectly specified.
func CreateOrUpdate(inputFile, action string, astroPlatformCore astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer, waitForStatus bool) error { //nolint
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
		clusterID, nodePools, err = getClusterInfoFromName(formattedDeployment.Deployment.Configuration.ClusterName, c.Organization, astroPlatformCore)
		if err != nil {
			return err
		}
	}
	workspaceID, err = getWorkspaceIDFromName(formattedDeployment.Deployment.Configuration.WorkspaceName, coreClient)
	if err != nil {
		return err
	}
	existingDeployments, err = deployment.CoreGetDeployments(workspaceID, c.Organization, astroPlatformCore)
	if err != nil {
		return err
	}
	if action == createAction {
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
		err = createOrUpdateDeployment(&formattedDeployment, clusterID, workspaceID, createAction, &astroplatformcore.Deployment{}, nodePools, dagDeploy, envVars, coreClient, astroPlatformCore, waitForStatus)
		if err != nil {
			return err
		}
		// users can not creat a deployment with env vars or alert emails in v1beta1 so to create them with a deployment file we need update right after deployment creation
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
		existingDeployment, err = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name, astroPlatformCore)
		if err != nil {
			return err
		}
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
		err = createOrUpdateDeployment(&formattedDeployment, clusterID, workspaceID, updateAction, &existingDeployment, nodePools, dagDeploy, envVars, coreClient, astroPlatformCore, waitForStatus)
		if err != nil {
			return err
		}
	}
	// Get deployment created or updated
	existingDeployment, err = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name, astroPlatformCore)
	if err != nil {
		return err
	}
	if jsonOutput {
		outputFormat = jsonFormat
	}
	return inspect.Inspect(workspaceID, "", existingDeployment.Id, outputFormat, astroPlatformCore, coreClient, out, "", false, true)
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
func createOrUpdateDeployment(deploymentFromFile *inspect.FormattedDeployment, clusterID, workspaceID, action string, existingDeployment *astroplatformcore.Deployment, nodePools []astroplatformcore.NodePool, dagDeploy bool, envVars []astroplatformcore.DeploymentEnvironmentVariableRequest, coreClient astrocore.CoreClient, astroPlatformCore astroplatformcore.CoreClient, waitForStatus bool) error { //nolint
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
					if strings.EqualFold(string(astroMachines[j].Name), *listQueues[i].AstroMachine) {
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
				workerQueue.AstroMachine = astroplatformcore.WorkerQueueRequestAstroMachine(strings.ToUpper(*listQueues[i].AstroMachine))
				// set default values if none were specified
				requestedWorkerQueue := workerqueue.SetWorkerQueueValues(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, workerQueue, defaultOptions, &astroMachine)
				// check if queue is valid
				if deploymentFromFile.Deployment.Configuration.Executor == deployment.KubeExecutor || deploymentFromFile.Deployment.Configuration.Executor == deployment.KUBERNETES {
					return errNoUseWorkerQueues
				}
				err = workerqueue.IsHostedWorkerQueueInputValid(requestedWorkerQueue, defaultOptions, &astroMachine)
				if err != nil {
					return err
				}
				// add it to the list of queues to be created
				listQueuesRequest = append(listQueuesRequest, requestedWorkerQueue)
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
					return errUseDefaultWorkerType
				}
				// set default values if none were specified
				requestedHybridWorkerQueue := workerqueue.SetWorkerQueueValuesHybrid(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, workerQueue, defaultOptions)
				err = workerqueue.IsWorkerQueueInputValid(requestedHybridWorkerQueue, defaultOptions)
				if err != nil {
					return err
				}
				workerQueue = requestedHybridWorkerQueue
				// add it to the list of queues to be created
				listHybridQueuesRequest = append(listHybridQueuesRequest, workerQueue)
			}
		}
	}
	switch action {
	case createAction:
		createDeploymentRequest := astroplatformcore.CreateDeploymentRequest{}
		var defaultTaskPodCPU *string
		var defaultTaskPodMemory *string
		var resourceQuotaCPU *string
		var resourceQuotaMemory *string
		var remoteExecution *astroplatformcore.DeploymentRemoteExecutionRequest
		if deployment.IsDeploymentStandard(deploymentType) || deployment.IsDeploymentDedicated(deploymentType) {
			coreCloudProvider := deployment.GetCoreCloudProvider(deploymentFromFile.Deployment.Configuration.CloudProvider)
			coreDeploymentType := astrocore.GetDeploymentOptionsParamsDeploymentType(deploymentType)

			deploymentOptionsParams := astrocore.GetDeploymentOptionsParams{
				DeploymentType: &coreDeploymentType,
				CloudProvider:  &coreCloudProvider,
			}

			configOption, err := deployment.GetDeploymentOptions("", deploymentOptionsParams, coreClient)
			if err != nil {
				return err
			}
			remoteExecutionEnabled := deploymentFromFile.Deployment.Configuration.RemoteExecution != nil && deploymentFromFile.Deployment.Configuration.RemoteExecution.Enabled
			defaultTaskPodCPU = deployment.CreateDefaultTaskPodCPU(deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU, remoteExecutionEnabled, &configOption)
			defaultTaskPodMemory = deployment.CreateDefaultTaskPodMemory(deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory, remoteExecutionEnabled, &configOption)
			resourceQuotaCPU = deployment.CreateResourceQuotaCPU(deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU, remoteExecutionEnabled, &configOption)
			resourceQuotaMemory = deployment.CreateResourceQuotaMemory(deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory, remoteExecutionEnabled, &configOption)
			if remoteExecutionEnabled {
				remoteExecution = &astroplatformcore.DeploymentRemoteExecutionRequest{
					Enabled:                true,
					AllowedIpAddressRanges: deploymentFromFile.Deployment.Configuration.RemoteExecution.AllowedIPAddressRanges,
					TaskLogBucket:          deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogBucket,
					TaskLogUrlPattern:      deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogURLPattern,
				}
			}
		}
		if deployment.IsDeploymentStandard(deploymentType) {
			var requestedCloudProvider astroplatformcore.CreateStandardDeploymentRequestCloudProvider
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.CloudProvider) {
			case "GCP":
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderGCP
			case "AWS":
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAWS
			case "AZURE":
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAZURE
			}

			var requestedExecutor astroplatformcore.CreateStandardDeploymentRequestExecutor
			switch deploymentFromFile.Deployment.Configuration.Executor {
			case deployment.CeleryExecutor, deployment.CELERY:
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorCELERY
			case deployment.KubeExecutor, deployment.KUBERNETES:
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorKUBERNETES
			case deployment.AstroExecutor, deployment.ASTRO:
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorASTRO
			}

			var schedulerSize astroplatformcore.CreateStandardDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeSMALL
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeLARGE
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
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
				IsDevelopmentMode:    &deploymentFromFile.Deployment.Configuration.IsDevelopmentMode,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateStandardDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			if standardDeploymentRequest.IsDevelopmentMode != nil && *standardDeploymentRequest.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				standardDeploymentRequest.ScalingSpec = &astroplatformcore.DeploymentScalingSpecRequest{
					HibernationSpec: &astroplatformcore.DeploymentHibernationSpecRequest{
						Schedules: &hibernationSchedules,
					},
				}
			}
			err := createDeploymentRequest.FromCreateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.CreateDedicatedDeploymentRequestExecutor
			switch deploymentFromFile.Deployment.Configuration.Executor {
			case deployment.CeleryExecutor, deployment.CELERY:
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorCELERY
			case deployment.KubeExecutor, deployment.KUBERNETES:
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorKUBERNETES
			case deployment.AstroExecutor, deployment.ASTRO:
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorASTRO
			}

			var schedulerSize astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeSMALL
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeLARGE
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
			}

			dedicatedDeploymentRequest := astroplatformcore.CreateDedicatedDeploymentRequest{
				AstroRuntimeVersion:  deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				IsDevelopmentMode:    &deploymentFromFile.Deployment.Configuration.IsDevelopmentMode,
				ClusterId:            clusterID,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			if dedicatedDeploymentRequest.IsDevelopmentMode != nil && *dedicatedDeploymentRequest.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				dedicatedDeploymentRequest.ScalingSpec = &astroplatformcore.DeploymentScalingSpecRequest{
					HibernationSpec: &astroplatformcore.DeploymentHibernationSpecRequest{
						Schedules: &hibernationSchedules,
					},
				}
			}
			err := createDeploymentRequest.FromCreateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if !deployment.IsDeploymentStandard(deploymentType) && !deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.CreateHybridDeploymentRequestExecutor
			switch deploymentFromFile.Deployment.Configuration.Executor {
			case deployment.CeleryExecutor, deployment.CELERY:
				requestedExecutor = astroplatformcore.CreateHybridDeploymentRequestExecutorCELERY
			case deployment.KubeExecutor, deployment.KUBERNETES:
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
				Type:             astroplatformcore.CreateHybridDeploymentRequestTypeHYBRID,
				WorkerQueues:     &listHybridQueuesRequest,
				WorkloadIdentity: &deploymentFromFile.Deployment.Configuration.WorkloadIdentity,
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

		d, err := deployment.CoreCreateDeployment("", createDeploymentRequest, astroPlatformCore)
		if err != nil {
			return err
		}
		deploymentID := d.Id

		if waitForStatus {
			err = deployment.HealthPoll(
				deploymentID,
				workspaceID,
				deployment.SleepTime,
				deployment.TickNum,
				deployment.TimeoutNum,
				astroPlatformCore,
			)
			if err != nil {
				return err
			}
		}

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
		defaultTaskPodCPU := deployment.UpdateDefaultTaskPodCPU(deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU, existingDeployment, nil)
		defaultTaskPodMemory := deployment.UpdateDefaultTaskPodMemory(deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory, existingDeployment, nil)
		resourceQuotaCPU := deployment.UpdateResourceQuotaCPU(deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU, existingDeployment)
		resourceQuotaMemory := deployment.UpdateResourceQuotaMemory(deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory, existingDeployment)
		var remoteExecution *astroplatformcore.DeploymentRemoteExecutionRequest
		if deployment.IsRemoteExecutionEnabled(existingDeployment) {
			remoteExecution = &astroplatformcore.DeploymentRemoteExecutionRequest{
				Enabled:                true,
				AllowedIpAddressRanges: deploymentFromFile.Deployment.Configuration.RemoteExecution.AllowedIPAddressRanges,
				TaskLogBucket:          deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogBucket,
				TaskLogUrlPattern:      deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogURLPattern,
			}
		}
		if deployment.IsDeploymentStandard(deploymentType) {
			var requestedExecutor astroplatformcore.UpdateStandardDeploymentRequestExecutor
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.Executor) {
			case strings.ToUpper(deployment.CeleryExecutor), deployment.CELERY:
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorCELERY
			case strings.ToUpper(deployment.KubeExecutor), deployment.KUBERNETES:
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(deployment.AstroExecutor), deployment.ASTRO:
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorASTRO
			}

			var schedulerSize astroplatformcore.UpdateStandardDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeSMALL
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeMEDIUM
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeLARGE
			case string(astrocore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
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
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			if existingDeployment.IsDevelopmentMode != nil && *existingDeployment.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				standardDeploymentRequest.ScalingSpec = &astroplatformcore.DeploymentScalingSpecRequest{
					HibernationSpec: &astroplatformcore.DeploymentHibernationSpecRequest{
						Schedules: &hibernationSchedules,
					},
				}
			}
			err := updateDeploymentRequest.FromUpdateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.UpdateDedicatedDeploymentRequestExecutor
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.Executor) {
			case strings.ToUpper(deployment.CeleryExecutor), deployment.CELERY:
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorCELERY
			case strings.ToUpper(deployment.KubeExecutor), deployment.KUBERNETES:
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(deployment.AstroExecutor), deployment.ASTRO:
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorASTRO
			}

			var schedulerSize astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeSMALL
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeLARGE
			case string(astrocore.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
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
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkerQueues:         &listQueuesRequest,
				SchedulerSize:        schedulerSize,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			if existingDeployment.IsDevelopmentMode != nil && *existingDeployment.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				dedicatedDeploymentRequest.ScalingSpec = &astroplatformcore.DeploymentScalingSpecRequest{
					HibernationSpec: &astroplatformcore.DeploymentHibernationSpecRequest{
						Schedules: &hibernationSchedules,
					},
				}
			}
			err := updateDeploymentRequest.FromUpdateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if !deployment.IsDeploymentStandard(deploymentType) && !deployment.IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.UpdateHybridDeploymentRequestExecutor
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.Executor) {
			case strings.ToUpper(deployment.CeleryExecutor), deployment.CELERY:
				requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorCELERY
			case strings.ToUpper(deployment.KubeExecutor), deployment.KUBERNETES:
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
				WorkloadIdentity:     &deploymentFromFile.Deployment.Configuration.WorkloadIdentity,
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
					"\nEither disable ci-cd enforcement or please cancel this operation and use API Tokens instead.")
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

func ToDeploymentHibernationSchedules(hibernationSchedules []inspect.HibernationSchedule) []astroplatformcore.DeploymentHibernationSchedule {
	schedules := make([]astroplatformcore.DeploymentHibernationSchedule, 0, len(hibernationSchedules))
	for i := range hibernationSchedules {
		var schedule astroplatformcore.DeploymentHibernationSchedule
		schedule.HibernateAtCron = hibernationSchedules[i].HibernateAt
		schedule.WakeAtCron = hibernationSchedules[i].WakeAt
		schedule.Description = &hibernationSchedules[i].Description
		schedule.IsEnabled = hibernationSchedules[i].Enabled
		schedules = append(schedules, schedule)
	}
	return schedules
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
	// check if deployment is using Airflow 3 by validating runtime version
	if !deployment.IsValidExecutor(deploymentFromFile.Deployment.Configuration.Executor, deploymentFromFile.Deployment.Configuration.RunTimeVersion, deploymentFromFile.Deployment.Configuration.DeploymentType) {
		return fmt.Errorf("executor %s %w. It can be CeleryExecutor, KubernetesExecutor, or AstroExecutor", deploymentFromFile.Deployment.Configuration.Executor, errInvalidValue)
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
func deploymentFromName(existingDeployments []astroplatformcore.Deployment, deploymentName string, platformCoreClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	var existingDeployment astroplatformcore.Deployment
	var j int

	for i := range existingDeployments {
		if existingDeployments[i].Name == deploymentName {
			// deployment that matched name
			existingDeployment = existingDeployments[i]
			j++
		}
	}
	if j > 1 {
		return astroplatformcore.Deployment{}, errDeploymentsWithSameName
	}

	if j == 0 {
		return astroplatformcore.Deployment{}, nil
	}

	existingDeployment, err := deployment.CoreGetDeployment("", existingDeployment.Id, platformCoreClient)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	return existingDeployment, nil
}

// getClusterInfoFromName takes clusterName and org as its arguments.
// It returns the clusterID and list of nodepools if the cluster is found in the organization.
// It returns an errClusterNotFound if the cluster does not exist in the organization.
func getClusterInfoFromName(clusterName, organizationID string, platformCoreClient astroplatformcore.CoreClient) (string, []astroplatformcore.NodePool, error) {
	var (
		existingClusters []astroplatformcore.Cluster
		err              error
		nodePools        []astroplatformcore.NodePool
	)

	existingClusters, err = organization.ListClusters(organizationID, platformCoreClient)
	if err != nil {
		return "", nil, err
	}

	for _, cluster := range existingClusters { //nolint
		if cluster.Name == clusterName {
			if cluster.NodePools != nil {
				nodePools = *cluster.NodePools
			}
			return cluster.Id, nodePools, nil
		}
	}
	err = fmt.Errorf("cluster_name: %s %w in organization", clusterName, errNotFound)
	return "", nil, err
}

// getWorkspaceIDFromName takes workspaceName as its argument.
// It returns the workspaceID if the workspace is found in the organization.
// It returns an errWorkspaceNotFound if the workspace does not exist in the organization.
func getWorkspaceIDFromName(workspaceName string, client astrocore.CoreClient) (string, error) {
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
	return "", nil
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
	if hasEnvVars(deploymentFromFile) || hasAlertEmails(deploymentFromFile) {
		return true
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
		if action == createAction || (action == updateAction && !envVar.IsSecret) {
			if envVar.Value == nil {
				missingField := fmt.Sprintf("deployment.environment_variables[%d].value", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
		}
	}
	return nil
}

func createEnvVarsRequest(deploymentFromFile *inspect.FormattedDeployment) (envVars []astroplatformcore.DeploymentEnvironmentVariableRequest) {
	envVars = []astroplatformcore.DeploymentEnvironmentVariableRequest{}
	for i := range deploymentFromFile.Deployment.EnvVars {
		var envVar astroplatformcore.DeploymentEnvironmentVariableRequest
		envVar.IsSecret = deploymentFromFile.Deployment.EnvVars[i].IsSecret
		envVar.Key = deploymentFromFile.Deployment.EnvVars[i].Key
		envVar.Value = deploymentFromFile.Deployment.EnvVars[i].Value
		envVars = append(envVars, envVar)
	}
	return envVars
}

func transformDeploymentType(deploymentType string) astroplatformcore.DeploymentType {
	var transformedDeploymentType astroplatformcore.DeploymentType

	switch strings.ToUpper(deploymentType) {
	case "STANDARD":
		transformedDeploymentType = astroplatformcore.DeploymentTypeSTANDARD
	case HostedShared:
		transformedDeploymentType = astroplatformcore.DeploymentTypeSTANDARD
	case HostedStandard:
		transformedDeploymentType = astroplatformcore.DeploymentTypeSTANDARD
	case "DEDICATED":
		transformedDeploymentType = astroplatformcore.DeploymentTypeDEDICATED
	case HostedDedicated:
		transformedDeploymentType = astroplatformcore.DeploymentTypeDEDICATED
	case "HYBRID":
		transformedDeploymentType = astroplatformcore.DeploymentTypeHYBRID
	}

	return transformedDeploymentType
}
