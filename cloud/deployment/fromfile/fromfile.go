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
	"time"

	"github.com/ghodss/yaml"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	errEmptyFile                      = errors.New("has no content")
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
func CreateOrUpdate(inputFile, action string, astroV1Client astrov1.APIClient, out io.Writer, waitForStatus bool, waitTime time.Duration, force bool) error { //nolint
	var (
		err                                           error
		errHelp, clusterID, workspaceID, outputFormat string
		dataBytes                                     []byte
		formattedDeployment                           inspect.FormattedDeployment
		existingDeployment                            astrov1.Deployment
		existingDeployments                           []astrov1.Deployment
		nodePools                                     []astrov1.NodePool
		jsonOutput                                    bool
		dagDeploy                                     bool
		envVars                                       []astrov1.DeploymentEnvironmentVariableRequest
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
		clusterID, nodePools, err = getClusterInfoFromName(formattedDeployment.Deployment.Configuration.ClusterName, c.Organization, astroV1Client)
		if err != nil {
			return err
		}
	}
	workspaceID, err = getWorkspaceIDFromName(formattedDeployment.Deployment.Configuration.WorkspaceName, astroV1Client)
	if err != nil {
		return err
	}
	existingDeployments, err = deployment.ListDeployments(workspaceID, c.Organization, astroV1Client)
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
		err = createOrUpdateDeployment(&formattedDeployment, clusterID, workspaceID, createAction, &astrov1.Deployment{}, nodePools, dagDeploy, envVars, astroV1Client, waitForStatus, waitTime, force)
		if err != nil {
			return err
		}
		// the public v1 CreateDeployment API doesn't accept env vars or alert emails,
		// so when a deployment file specifies them we follow up with an update call.
		if hasEnvVarsOrAlertEmails(&formattedDeployment) {
			action = updateAction
		}
	}
	// get new existing deployments list
	existingDeployments, err = deployment.ListDeployments(workspaceID, c.Organization, astroV1Client)
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
		existingDeployment, err = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name, astroV1Client)
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
		err = createOrUpdateDeployment(&formattedDeployment, clusterID, workspaceID, updateAction, &existingDeployment, nodePools, dagDeploy, envVars, astroV1Client, waitForStatus, waitTime, force)
		if err != nil {
			return err
		}
	}
	// Get deployment created or updated
	existingDeployment, err = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name, astroV1Client)
	if err != nil {
		return err
	}
	if jsonOutput {
		outputFormat = jsonFormat
	}
	return inspect.Inspect(workspaceID, "", existingDeployment.Id, outputFormat, astroV1Client, out, "", false, true)
}

// createOrUpdateDeployment transforms an inspect.FormattedDeployment into a v1 CreateDeploymentInput or
// UpdateDeploymentInput based on the action requested. Then the deployment is created or updated
// If worker-queues were requested, it gets node pool id work the workers and validates queue options.
// If no queue options were specified, it sets default values.
// It returns an error if getting default options fail.
// It returns an error if worker-queue options are not valid.
// It returns an error if node pool id could not be found for the worker type.
//
//nolint:dupl
func createOrUpdateDeployment(deploymentFromFile *inspect.FormattedDeployment, clusterID, workspaceID, action string, existingDeployment *astrov1.Deployment, nodePools []astrov1.NodePool, dagDeploy bool, envVars []astrov1.DeploymentEnvironmentVariableRequest, astroV1Client astrov1.APIClient, waitForStatus bool, waitTime time.Duration, force bool) error { //nolint
	var (
		defaultOptions          astrov1.WorkerQueueOptions
		configOptions           astrov1.DeploymentOptions
		listQueues              []astrov1.WorkerQueue
		astroMachine            astrov1.WorkerMachine
		listQueuesRequest       []astrov1.WorkerQueueRequest
		listHybridQueuesRequest []astrov1.HybridWorkerQueueRequest
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
		deploymentOptions, err := deployment.GetPlatformDeploymentOptions("", astrov1.GetDeploymentOptionsParams{}, astroV1Client)
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
				var workerQueue astrov1.WorkerQueueRequest
				workerQueue.Id = &listQueues[i].Id
				workerQueue.Name = listQueues[i].Name
				workerQueue.IsDefault = listQueues[i].IsDefault
				workerQueue.MaxWorkerCount = listQueues[i].MaxWorkerCount
				workerQueue.MinWorkerCount = listQueues[i].MinWorkerCount
				workerQueue.WorkerConcurrency = listQueues[i].WorkerConcurrency
				workerQueue.AstroMachine = astrov1.WorkerQueueRequestAstroMachine(strings.ToUpper(*listQueues[i].AstroMachine))
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
				var workerQueue astrov1.HybridWorkerQueueRequest
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
		createDeploymentRequest := astrov1.CreateDeploymentRequest{}
		var defaultTaskPodCPU *string
		var defaultTaskPodMemory *string
		var resourceQuotaCPU *string
		var resourceQuotaMemory *string
		var remoteExecution *astrov1.DeploymentRemoteExecutionRequest
		if deployment.IsDeploymentStandard(deploymentType) || deployment.IsDeploymentDedicated(deploymentType) {
			coreCloudProvider := deployment.GetCoreCloudProvider(deploymentFromFile.Deployment.Configuration.CloudProvider)
			coreDeploymentType := astrov1.GetDeploymentOptionsParamsDeploymentType(deploymentType)

			deploymentOptionsParams := astrov1.GetDeploymentOptionsParams{
				DeploymentType: &coreDeploymentType,
				CloudProvider:  &coreCloudProvider,
			}

			configOption, err := deployment.GetDeploymentOptions("", deploymentOptionsParams, astroV1Client)
			if err != nil {
				return err
			}
			remoteExecutionEnabled := deploymentFromFile.Deployment.Configuration.RemoteExecution != nil && deploymentFromFile.Deployment.Configuration.RemoteExecution.Enabled
			defaultTaskPodCPU = deployment.CreateDefaultTaskPodCPU(deploymentFromFile.Deployment.Configuration.DefaultTaskPodCPU, remoteExecutionEnabled, &configOption)
			defaultTaskPodMemory = deployment.CreateDefaultTaskPodMemory(deploymentFromFile.Deployment.Configuration.DefaultTaskPodMemory, remoteExecutionEnabled, &configOption)
			resourceQuotaCPU = deployment.CreateResourceQuotaCPU(deploymentFromFile.Deployment.Configuration.ResourceQuotaCPU, remoteExecutionEnabled, &configOption)
			resourceQuotaMemory = deployment.CreateResourceQuotaMemory(deploymentFromFile.Deployment.Configuration.ResourceQuotaMemory, remoteExecutionEnabled, &configOption)
			if remoteExecutionEnabled {
				remoteExecution = &astrov1.DeploymentRemoteExecutionRequest{
					Enabled:                true,
					AllowedIpAddressRanges: deploymentFromFile.Deployment.Configuration.RemoteExecution.AllowedIPAddressRanges,
					TaskLogBucket:          deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogBucket,
					TaskLogUrlPattern:      deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogURLPattern,
				}
			}
		}
		if deployment.IsDeploymentStandard(deploymentType) {
			var requestedCloudProvider astrov1.CreateStandardDeploymentRequestCloudProvider
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.CloudProvider) {
			case "GCP":
				requestedCloudProvider = astrov1.CreateStandardDeploymentRequestCloudProviderGCP
			case "AWS":
				requestedCloudProvider = astrov1.CreateStandardDeploymentRequestCloudProviderAWS
			case "AZURE":
				requestedCloudProvider = astrov1.CreateStandardDeploymentRequestCloudProviderAZURE
			}

			var requestedExecutor astrov1.CreateStandardDeploymentRequestExecutor
			switch deploymentFromFile.Deployment.Configuration.Executor {
			case deployment.CeleryExecutor, deployment.CELERY:
				requestedExecutor = astrov1.CreateStandardDeploymentRequestExecutorCELERY
			case deployment.KubeExecutor, deployment.KUBERNETES:
				requestedExecutor = astrov1.CreateStandardDeploymentRequestExecutorKUBERNETES
			case deployment.AstroExecutor, deployment.ASTRO:
				requestedExecutor = astrov1.CreateStandardDeploymentRequestExecutorASTRO
			}

			var schedulerSize astrov1.CreateStandardDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
			}

			standardDeploymentRequest := astrov1.CreateStandardDeploymentRequest{
				AstroRuntimeVersion:  new(deploymentFromFile.Deployment.Configuration.RunTimeVersion),
				CloudProvider:        &requestedCloudProvider,
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             new(requestedExecutor),
				IsCicdEnforced:       new(deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments),
				Region:               &deploymentFromFile.Deployment.Configuration.Region,
				IsDagDeployEnabled:   new(dagDeploy),
				IsHighAvailability:   new(deploymentFromFile.Deployment.Configuration.IsHighAvailability),
				IsDevelopmentMode:    &deploymentFromFile.Deployment.Configuration.IsDevelopmentMode,
				WorkspaceId:          workspaceID,
				Type:                 new(astrov1.CreateStandardDeploymentRequestTypeSTANDARD),
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			if len(listQueuesRequest) > 0 {
				standardDeploymentRequest.WorkerQueues = &listQueuesRequest
			}
			if schedulerSize != "" {
				standardDeploymentRequest.SchedulerSize = &schedulerSize
			}
			if standardDeploymentRequest.IsDevelopmentMode != nil && *standardDeploymentRequest.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				standardDeploymentRequest.ScalingSpec = &astrov1.DeploymentScalingSpecRequest{
					HibernationSpec: &astrov1.DeploymentHibernationSpecRequest{
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
			var requestedExecutor astrov1.CreateDedicatedDeploymentRequestExecutor
			switch deploymentFromFile.Deployment.Configuration.Executor {
			case deployment.CeleryExecutor, deployment.CELERY:
				requestedExecutor = astrov1.CreateDedicatedDeploymentRequestExecutorCELERY
			case deployment.KubeExecutor, deployment.KUBERNETES:
				requestedExecutor = astrov1.CreateDedicatedDeploymentRequestExecutorKUBERNETES
			case deployment.AstroExecutor, deployment.ASTRO:
				requestedExecutor = astrov1.CreateDedicatedDeploymentRequestExecutorASTRO
			}

			var schedulerSize astrov1.CreateDedicatedDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astrov1.CreateDedicatedDeploymentRequestSchedulerSizeSMALL
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astrov1.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astrov1.CreateDedicatedDeploymentRequestSchedulerSizeLARGE
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astrov1.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
			}

			dedicatedDeploymentRequest := astrov1.CreateDedicatedDeploymentRequest{
				AstroRuntimeVersion:  new(deploymentFromFile.Deployment.Configuration.RunTimeVersion),
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             new(requestedExecutor),
				IsCicdEnforced:       new(deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments),
				IsDagDeployEnabled:   new(dagDeploy),
				IsHighAvailability:   new(deploymentFromFile.Deployment.Configuration.IsHighAvailability),
				IsDevelopmentMode:    &deploymentFromFile.Deployment.Configuration.IsDevelopmentMode,
				ClusterId:            new(clusterID),
				WorkspaceId:          workspaceID,
				Type:                 new(astrov1.CreateDedicatedDeploymentRequestTypeDEDICATED),
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			if len(listQueuesRequest) > 0 {
				dedicatedDeploymentRequest.WorkerQueues = &listQueuesRequest
			}
			if schedulerSize != "" {
				dedicatedDeploymentRequest.SchedulerSize = &schedulerSize
			}
			if dedicatedDeploymentRequest.IsDevelopmentMode != nil && *dedicatedDeploymentRequest.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				dedicatedDeploymentRequest.ScalingSpec = &astrov1.DeploymentScalingSpecRequest{
					HibernationSpec: &astrov1.DeploymentHibernationSpecRequest{
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
			var requestedExecutor astrov1.CreateHybridDeploymentRequestExecutor
			switch deploymentFromFile.Deployment.Configuration.Executor {
			case deployment.CeleryExecutor, deployment.CELERY:
				requestedExecutor = astrov1.CreateHybridDeploymentRequestExecutorCELERY
			case deployment.KubeExecutor, deployment.KUBERNETES:
				requestedExecutor = astrov1.CreateHybridDeploymentRequestExecutorKUBERNETES
			}

			hybridDeploymentRequest := astrov1.CreateHybridDeploymentRequest{
				AstroRuntimeVersion: new(deploymentFromFile.Deployment.Configuration.RunTimeVersion),
				Description:         &deploymentFromFile.Deployment.Configuration.Description,
				Name:                deploymentFromFile.Deployment.Configuration.Name,
				Executor:            new(requestedExecutor),
				IsCicdEnforced:      new(deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments),
				IsDagDeployEnabled:  new(dagDeploy),
				ClusterId:           new(clusterID),
				WorkspaceId:         workspaceID,
				Type:                new(astrov1.CreateHybridDeploymentRequestTypeHYBRID),
				WorkloadIdentity:    &deploymentFromFile.Deployment.Configuration.WorkloadIdentity,
			}
			if len(listHybridQueuesRequest) > 0 {
				hybridDeploymentRequest.WorkerQueues = &listHybridQueuesRequest
			}
			schedulerAU := deploymentFromFile.Deployment.Configuration.SchedulerAU
			schedulerReplicas := deploymentFromFile.Deployment.Configuration.SchedulerCount
			if schedulerAU != 0 || schedulerReplicas != 0 {
				hybridDeploymentRequest.Scheduler = &astrov1.CreateDeploymentInstanceSpecRequest{}
				if schedulerAU != 0 {
					hybridDeploymentRequest.Scheduler.Au = &schedulerAU
				}
				if schedulerReplicas != 0 {
					hybridDeploymentRequest.Scheduler.Replicas = &schedulerReplicas
				}
			}
			if requestedExecutor == astrov1.CreateHybridDeploymentRequestExecutorKUBERNETES {
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

		deploymentResponse, err := deployment.CoreCreateDeployment("", createDeploymentRequest, astroV1Client)
		if err != nil {
			return err
		}

		if waitForStatus {
			err = deployment.HealthPoll(
				deploymentResponse.Id,
				workspaceID,
				deployment.SleepTime,
				deployment.TickNum,
				int(waitTime.Seconds()),
				astroV1Client,
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
		updateDeploymentRequest := astrov1.UpdateDeploymentRequest{}
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
		var remoteExecution *astrov1.DeploymentRemoteExecutionRequest
		if deployment.IsRemoteExecutionEnabled(existingDeployment) {
			remoteExecution = &astrov1.DeploymentRemoteExecutionRequest{
				Enabled:                true,
				AllowedIpAddressRanges: deploymentFromFile.Deployment.Configuration.RemoteExecution.AllowedIPAddressRanges,
				TaskLogBucket:          deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogBucket,
				TaskLogUrlPattern:      deploymentFromFile.Deployment.Configuration.RemoteExecution.TaskLogURLPattern,
			}
		}
		if deployment.IsDeploymentStandard(deploymentType) {
			var requestedExecutor astrov1.UpdateStandardDeploymentRequestExecutor
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.Executor) {
			case strings.ToUpper(deployment.CeleryExecutor), deployment.CELERY:
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorCELERY
			case strings.ToUpper(deployment.KubeExecutor), deployment.KUBERNETES:
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(deployment.AstroExecutor), deployment.ASTRO:
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorASTRO
			}

			var schedulerSize astrov1.UpdateStandardDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeSMALL
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeMEDIUM
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeLARGE
			case string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
			}

			standardDeploymentRequest := astrov1.UpdateStandardDeploymentRequest{
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				WorkspaceId:          workspaceID,
				Type:                 astrov1.UpdateStandardDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkerQueues:         new(deployment.ConvertCreateQueuesToUpdate(listQueuesRequest)),
				SchedulerSize:        schedulerSize,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			if existingDeployment.IsDevelopmentMode != nil && *existingDeployment.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				standardDeploymentRequest.ScalingSpec = &astrov1.DeploymentScalingSpecRequest{
					HibernationSpec: &astrov1.DeploymentHibernationSpecRequest{
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
			var requestedExecutor astrov1.UpdateDedicatedDeploymentRequestExecutor
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.Executor) {
			case strings.ToUpper(deployment.CeleryExecutor), deployment.CELERY:
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorCELERY
			case strings.ToUpper(deployment.KubeExecutor), deployment.KUBERNETES:
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(deployment.AstroExecutor), deployment.ASTRO:
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorASTRO
			}

			var schedulerSize astrov1.UpdateDedicatedDeploymentRequestSchedulerSize
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.SchedulerSize) {
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeSMALL):
				schedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeSMALL
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM):
				schedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeLARGE):
				schedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeLARGE
			case string(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE):
				schedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			}

			var deplWorkloadIdentity *string
			if deploymentFromFile.Deployment.Configuration.WorkloadIdentity != "" {
				deplWorkloadIdentity = &deploymentFromFile.Deployment.Configuration.WorkloadIdentity
			}

			dedicatedDeploymentRequest := astrov1.UpdateDedicatedDeploymentRequest{
				Description:          &deploymentFromFile.Deployment.Configuration.Description,
				Name:                 deploymentFromFile.Deployment.Configuration.Name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled:   dagDeploy,
				IsHighAvailability:   deploymentFromFile.Deployment.Configuration.IsHighAvailability,
				WorkspaceId:          workspaceID,
				Type:                 astrov1.UpdateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    defaultTaskPodCPU,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCPU,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkerQueues:         new(deployment.ConvertCreateQueuesToUpdate(listQueuesRequest)),
				SchedulerSize:        schedulerSize,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			if existingDeployment.IsDevelopmentMode != nil && *existingDeployment.IsDevelopmentMode {
				hibernationSchedules := ToDeploymentHibernationSchedules(deploymentFromFile.Deployment.HibernationSchedules)
				dedicatedDeploymentRequest.ScalingSpec = &astrov1.DeploymentScalingSpecRequest{
					HibernationSpec: &astrov1.DeploymentHibernationSpecRequest{
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
			var requestedExecutor astrov1.UpdateHybridDeploymentRequestExecutor
			switch strings.ToUpper(deploymentFromFile.Deployment.Configuration.Executor) {
			case strings.ToUpper(deployment.CeleryExecutor), deployment.CELERY:
				requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutorCELERY
			case strings.ToUpper(deployment.KubeExecutor), deployment.KUBERNETES:
				requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutorKUBERNETES
			}

			hybridDeploymentRequest := astrov1.UpdateHybridDeploymentRequest{
				Description:        &deploymentFromFile.Deployment.Configuration.Description,
				Name:               deploymentFromFile.Deployment.Configuration.Name,
				Executor:           requestedExecutor,
				IsCicdEnforced:     deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
				IsDagDeployEnabled: dagDeploy,
				WorkspaceId:        workspaceID,
				Scheduler: astrov1.UpdateDeploymentInstanceSpecRequest{
					Au:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
				Type:                 astrov1.UpdateHybridDeploymentRequestTypeHYBRID,
				ContactEmails:        &deploymentFromFile.Deployment.AlertEmails,
				EnvironmentVariables: envVars,
				WorkerQueues:         new(deployment.ConvertHybridQueuesToUpdate(listHybridQueuesRequest)),
				WorkloadIdentity:     &deploymentFromFile.Deployment.Configuration.WorkloadIdentity,
			}
			if requestedExecutor == astrov1.UpdateHybridDeploymentRequestExecutorKUBERNETES {
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
		if !force && deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments && dagDeploy {
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
		_, err := deployment.CoreUpdateDeployment("", existingDeployment.Id, updateDeploymentRequest, astroV1Client)
		if err != nil {
			return err
		}
	}
	return nil
}

func ToDeploymentHibernationSchedules(hibernationSchedules []inspect.HibernationSchedule) []astrov1.DeploymentHibernationSchedule {
	schedules := make([]astrov1.DeploymentHibernationSchedule, 0, len(hibernationSchedules))
	for i := range hibernationSchedules {
		var schedule astrov1.DeploymentHibernationSchedule
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
func deploymentExists(existingDeployments []astrov1.Deployment, deploymentNameToCreate string) bool {
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
func deploymentFromName(existingDeployments []astrov1.Deployment, deploymentName string, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	var existingDeployment astrov1.Deployment
	var j int

	for i := range existingDeployments {
		if existingDeployments[i].Name == deploymentName {
			// deployment that matched name
			existingDeployment = existingDeployments[i]
			j++
		}
	}
	if j > 1 {
		return astrov1.Deployment{}, errDeploymentsWithSameName
	}

	if j == 0 {
		return astrov1.Deployment{}, nil
	}

	existingDeployment, err := deployment.GetDeploymentByID("", existingDeployment.Id, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	return existingDeployment, nil
}

// getClusterInfoFromName takes clusterName and org as its arguments.
// It returns the clusterID and list of nodepools if the cluster is found in the organization.
// It returns an errClusterNotFound if the cluster does not exist in the organization.
func getClusterInfoFromName(clusterName, organizationID string, astroV1Client astrov1.APIClient) (string, []astrov1.NodePool, error) {
	var (
		existingClusters []astrov1.Cluster
		err              error
		nodePools        []astrov1.NodePool
	)

	existingClusters, err = organization.ListClusters(organizationID, astroV1Client)
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
func getWorkspaceIDFromName(workspaceName string, client astrov1.APIClient) (string, error) {
	var (
		existingWorkspaces []astrov1.Workspace
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
func getNodePoolIDFromWorkerType(workerType, clusterName string, nodePools []astrov1.NodePool) (string, error) {
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
func getQueues(deploymentFromFile *inspect.FormattedDeployment, nodePools []astrov1.NodePool, existingQueues []astrov1.WorkerQueue) ([]astrov1.WorkerQueue, error) {
	var (
		qList      []astrov1.WorkerQueue
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
	qList = make([]astrov1.WorkerQueue, len(requestedQueues))
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

func createEnvVarsRequest(deploymentFromFile *inspect.FormattedDeployment) (envVars []astrov1.DeploymentEnvironmentVariableRequest) {
	envVars = []astrov1.DeploymentEnvironmentVariableRequest{}
	for i := range deploymentFromFile.Deployment.EnvVars {
		var envVar astrov1.DeploymentEnvironmentVariableRequest
		envVar.IsSecret = deploymentFromFile.Deployment.EnvVars[i].IsSecret
		envVar.Key = deploymentFromFile.Deployment.EnvVars[i].Key
		envVar.Value = deploymentFromFile.Deployment.EnvVars[i].Value
		envVars = append(envVars, envVar)
	}
	return envVars
}

func transformDeploymentType(deploymentType string) astrov1.DeploymentType {
	var transformedDeploymentType astrov1.DeploymentType

	switch strings.ToUpper(deploymentType) {
	case "STANDARD":
		transformedDeploymentType = astrov1.DeploymentTypeSTANDARD
	case HostedShared:
		transformedDeploymentType = astrov1.DeploymentTypeSTANDARD
	case HostedStandard:
		transformedDeploymentType = astrov1.DeploymentTypeSTANDARD
	case "DEDICATED":
		transformedDeploymentType = astrov1.DeploymentTypeDEDICATED
	case HostedDedicated:
		transformedDeploymentType = astrov1.DeploymentTypeDEDICATED
	case "HYBRID":
		transformedDeploymentType = astrov1.DeploymentTypeHYBRID
	}

	return transformedDeploymentType
}
