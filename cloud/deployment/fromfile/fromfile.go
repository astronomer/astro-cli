package fromfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"sort"

	"github.com/astronomer/astro-cli/astro-client"
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
	canCiCdDeploy                     = deployment.CanCiCdDeploy
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
func CreateOrUpdate(inputFile, action string, client astro.Client, astroPlatformCore astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error { //nolint
	var (
		err                                            error
		errHelp, clusterID, workspaceID, outputFormat  string
		dataBytes                                      []byte
		formattedDeployment                            inspect.FormattedDeployment
		createInput                                    astro.CreateDeploymentInput
		updateInput                                    astro.UpdateDeploymentInput
		existingDeployment, createdOrUpdatedDeployment astro.Deployment
		existingDeployments                            []astro.Deployment
		nodePools                                      []astrocore.NodePool
		jsonOutput                                     bool
		dagDeploy                                      bool
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
		clusterID, nodePools, err = getClusterInfoFromName(formattedDeployment.Deployment.Configuration.ClusterName, c.OrganizationShortName, coreClient)
		if err != nil {
			return err
		}
	}

	existingDeployments, err = deployment.GetDeployments(workspaceID, c.Organization, client)
	if err != nil {
		return err
	}
	switch action {
	case createAction:
		if deployment.IsDeploymentStandard(deploymentType) {
			getSharedClusterParams := astrocore.GetSharedClusterParams{
				Region:        formattedDeployment.Deployment.Configuration.Region,
				CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(formattedDeployment.Deployment.Configuration.CloudProvider),
			}
			response, err := coreClient.GetSharedClusterWithResponse(context.Background(), &getSharedClusterParams)
			if err != nil {
				return err
			}
			err = astrocore.NormalizeAPIError(response.HTTPResponse, response.Body)
			if err != nil {
				return err
			}
			clusterID = response.JSON200.Id
		}
		// map workspace name to id
		workspaceID, err = getWorkspaceIDFromName(formattedDeployment.Deployment.Configuration.WorkspaceName, c.Organization, coreClient)
		if err != nil {
			return err
		}
		// get correct value for dag deploy
		if formattedDeployment.Deployment.Configuration.DagDeployEnabled == nil {
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
		createInput, _, err = getCreateOrUpdateInput(&formattedDeployment, clusterID, workspaceID, createAction, &astro.Deployment{}, nodePools, dagDeploy, client)
		if err != nil {
			return err
		}
		// create the deployment
		createdOrUpdatedDeployment, err = client.CreateDeployment(&createInput)
		if err != nil {
			return fmt.Errorf("%s: %w %+v", err.Error(), errCreateFailed, createInput)
		}
	case updateAction:
		// check if deployment does not exist
		if !deploymentExists(existingDeployments, formattedDeployment.Deployment.Configuration.Name) {
			// update does not allow creating new deployments
			errHelp = fmt.Sprintf("use deployment create --deployment-file %s instead", inputFile)
			return fmt.Errorf("deployment: %s %w: %s", formattedDeployment.Deployment.Configuration.Name,
				errNotFound, errHelp)
		}
		// this deployment exists so update it
		existingDeployment = deploymentFromName(existingDeployments, formattedDeployment.Deployment.Configuration.Name)
		workspaceID = existingDeployment.Workspace.ID

		// determine dagDeploy
		if formattedDeployment.Deployment.Configuration.DagDeployEnabled == nil {
			dagDeploy = existingDeployment.DagDeployEnabled
		} else {
			dagDeploy = *formattedDeployment.Deployment.Configuration.DagDeployEnabled
		}

		if deployment.IsDeploymentStandard(deploymentType) {
			clusterID = existingDeployment.Cluster.ID
		}

		// transform formattedDeployment to DeploymentUpdateInput
		_, updateInput, err = getCreateOrUpdateInput(&formattedDeployment, clusterID, workspaceID, updateAction, &existingDeployment, nodePools, dagDeploy, client)
		if err != nil {
			return err
		}

		if formattedDeployment.Deployment.Configuration.APIKeyOnlyDeployments && updateInput.DagDeployEnabled {
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

		// update the deployment
		createdOrUpdatedDeployment, err = client.UpdateDeployment(&updateInput)
		if err != nil {
			return fmt.Errorf("%s: %w %+v", err.Error(), errUpdateFailed, updateInput)
		}
	}
	// create environment variables
	if hasEnvVars(&formattedDeployment) {
		_, err = createEnvVars(&formattedDeployment, createdOrUpdatedDeployment.ID, client)
		if err != nil {
			return fmt.Errorf("%w \n failed to %s alert emails", err, action)
		}
	}
	// create alert emails
	if hasAlertEmails(&formattedDeployment) {
		_, err = createAlertEmails(&formattedDeployment, createdOrUpdatedDeployment.ID, client)
		if err != nil {
			return err
		}
	}
	if jsonOutput {
		outputFormat = jsonFormat
	}
	return inspect.Inspect(workspaceID, "", createdOrUpdatedDeployment.ID, outputFormat, client, astroPlatformCore, coreClient, out, "", false)
}

// getCreateOrUpdateInput transforms an inspect.FormattedDeployment into astro.CreateDeploymentInput or
// astro.UpdateDeploymentInput based on the action requested.
// If worker-queues were requested, it gets node pool id work the workers and validates queue options.
// If no queue options were specified, it sets default values.
// It returns an error if getting default options fail.
// It returns an error if worker-queue options are not valid.
// It returns an error if node pool id could not be found for the worker type.
func getCreateOrUpdateInput(deploymentFromFile *inspect.FormattedDeployment, clusterID, workspaceID, action string, existingDeployment *astro.Deployment, nodePools []astrocore.NodePool, dagDeploy bool, client astro.Client) (astro.CreateDeploymentInput, astro.UpdateDeploymentInput, error) { //nolint
	var (
		defaultOptions astro.WorkerQueueDefaultOptions
		configOptions  astro.DeploymentConfig
		listQueues     []astroplatformcore.WorkerQueue
		astroMachine   astroplatformcore.WorkerMachine
		createInput    astro.CreateDeploymentInput
		updateInput    astro.UpdateDeploymentInput
		err            error
	)
	deploymentType := transformDeploymentType(deploymentFromFile.Deployment.Configuration.DeploymentType)

	// Add worker queues if they were requested
	if hasQueues(deploymentFromFile) {
		// transform inspect.WorkerQ to []astro.WorkerQueue
		listQueues, err = getQueues(deploymentFromFile, nodePools, existingDeployment.WorkerQueues)
		if err != nil {
			return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{}, err
		}
		if deploymentFromFile.Deployment.Configuration.Executor == deployment.CeleryExecutor {
			// get defaults for min-count, max-count and concurrency from API
			defaultOptions, err = workerqueue.GetWorkerQueueDefaultOptions(client)
			if err != nil {
				return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{}, err
			}
			if deployment.IsDeploymentStandard(deploymentType) || deployment.IsDeploymentDedicated(deploymentType) {
				configOptions, err = client.GetDeploymentConfig()
				if err != nil {
					return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{}, err
				}
			}
			for i := range listQueues {
				// set default values if none were specified
				a := workerqueue.SetWorkerQueueValues(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, &listQueues[i], defaultOptions)
				if deployment.IsDeploymentStandard(deploymentType) || deployment.IsDeploymentDedicated(deploymentType) {
					astroMachines := configOptions.AstroMachines
					for j := range astroMachines {
						if astroMachines[j].Type == *listQueues[i].AstroMachine {

							astroMachine.Name = astroMachines[j].Type
							astroMachine.Spec.Cpu = astroMachines[j].CPU
							astroMachine.Spec.Memory = astroMachines[j].Memory
							astroMachine.Concurrency.Ceiling = float32(astroMachines[j].ConcurrentTasksMax)
							astroMachine.Concurrency.Default = float32(astroMachines[j].ConcurrentTasks)
							concurrency := float32(astroMachines[j].ConcurrentTasks)
							astroMachine.Spec.Concurrency = &concurrency
						}
					}
					// check if queue is valid
					err = workerqueue.IsHostedCeleryWorkerQueueInputValid(a, defaultOptions, &astroMachine)
					if err != nil {
						return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{}, err
					}
				} else {
					// check if queue is valid
					err = workerqueue.IsCeleryWorkerQueueInputValid(a, defaultOptions)
					if err != nil {
						return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{}, err
					}
				}
				// add it to the list of queues to be created
				listQueues[i] = *a
			}
		} else {
			// executor is KubernetesExecutor
			// check if more than one queue is requested
			if len(listQueues) > 1 {
				return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{},
					fmt.Errorf("%s %w more than one worker queue. (%d) were requested",
						deployment.KubeExecutor, workerqueue.ErrNotSupported, len(listQueues))
			}
			for i := range listQueues {
				err = workerqueue.IsKubernetesWorkerQueueInputValid(&listQueues[i])
				if err != nil {
					return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{}, err
				}
			}
		}
	}
	// tempory code
	var astroListQueues []astro.WorkerQueue
	for i := range listQueues {
		var workerQueue astro.WorkerQueue
		workerQueue.ID = listQueues[i].Id
		workerQueue.Name = listQueues[i].Name
		workerQueue.IsDefault = listQueues[i].IsDefault
		workerQueue.MaxWorkerCount = listQueues[i].MaxWorkerCount
		workerQueue.MinWorkerCount = listQueues[i].MinWorkerCount
		workerQueue.WorkerConcurrency = listQueues[i].WorkerConcurrency
		workerQueue.NodePoolID = *listQueues[i].NodePoolId
		workerQueue.PodCPU = listQueues[i].PodCpu
		workerQueue.PodRAM = listQueues[i].PodMemory
		astroListQueues = append(astroListQueues, workerQueue)
	}
	switch action {
	case createAction:
		createInput = astro.CreateDeploymentInput{
			WorkspaceID:           workspaceID,
			ClusterID:             clusterID,
			Label:                 deploymentFromFile.Deployment.Configuration.Name,
			Description:           deploymentFromFile.Deployment.Configuration.Description,
			RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
			DagDeployEnabled:      dagDeploy,
			SchedulerSize:         deploymentFromFile.Deployment.Configuration.SchedulerSize,
			APIKeyOnlyDeployments: deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
			IsHighAvailability:    deploymentFromFile.Deployment.Configuration.IsHighAvailability,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: deploymentFromFile.Deployment.Configuration.Executor,
				Scheduler: astro.Scheduler{
					AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
			},
			WorkerQueues: astroListQueues,
		}
	case updateAction:
		// check if cluster is being changed
		if clusterID != existingDeployment.Cluster.ID {
			return astro.CreateDeploymentInput{}, astro.UpdateDeploymentInput{},
				fmt.Errorf("changing an existing deployment's cluster %w", errNotPermitted)
		}
		updateInput = astro.UpdateDeploymentInput{
			ID:                    existingDeployment.ID,
			ClusterID:             clusterID,
			Label:                 deploymentFromFile.Deployment.Configuration.Name,
			Description:           deploymentFromFile.Deployment.Configuration.Description,
			DagDeployEnabled:      dagDeploy,
			SchedulerSize:         deploymentFromFile.Deployment.Configuration.SchedulerSize,
			APIKeyOnlyDeployments: deploymentFromFile.Deployment.Configuration.APIKeyOnlyDeployments,
			IsHighAvailability:    deploymentFromFile.Deployment.Configuration.IsHighAvailability,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: deploymentFromFile.Deployment.Configuration.Executor,
				Scheduler: astro.Scheduler{
					AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
			},
			WorkerQueues: astroListQueues,
		}
	}
	return createInput, updateInput, nil
}

// checkRequiredFields ensures all required fields are present in inspect.FormattedDeployment.
// It returns errRequiredField if required fields are missing, errInvalidValue if values are not valid and nil if not.
func checkRequiredFields(deploymentFromFile *inspect.FormattedDeployment, action string) error {
	if deploymentFromFile.Deployment.Configuration.Name == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.name")
	}
	if deploymentFromFile.Deployment.Configuration.ClusterName == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.cluster_name")
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
func deploymentExists(existingDeployments []astro.Deployment, deploymentNameToCreate string) bool {
	for i := range existingDeployments {
		if existingDeployments[i].Label == deploymentNameToCreate {
			// deployment exists
			return true
		}
	}
	return false
}

// deploymentFromName takes existingDeployments and deploymentName as its arguments.
// It returns the existing deployment that matches deploymentName.
func deploymentFromName(existingDeployments []astro.Deployment, deploymentName string) astro.Deployment {
	for i := range existingDeployments {
		if existingDeployments[i].Label == deploymentName {
			// deployment that matched name
			return existingDeployments[i]
		}
	}
	return astro.Deployment{}
}

// getClusterInfoFromName takes clusterName and organizationShortName as its arguments.
// It returns the clusterID and list of nodepools if the cluster is found in the organization.
// It returns an errClusterNotFound if the cluster does not exist in the organization.
func getClusterInfoFromName(clusterName, organizationShortName string, coreClient astrocore.CoreClient) (string, []astrocore.NodePool, error) {
	var (
		existingClusters []astrocore.Cluster
		err              error
	)

	existingClusters, err = organization.ListClusters(organizationShortName, coreClient)
	if err != nil {
		return "", nil, err
	}

	for _, cluster := range existingClusters { //nolint
		if cluster.Name == clusterName {
			return cluster.Id, cluster.NodePools, nil
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
func getNodePoolIDFromWorkerType(workerType, clusterName string, nodePools []astrocore.NodePool) (string, error) {
	var (
		pool astrocore.NodePool
		err  error
	)
	for _, pool = range nodePools { //nolint
		if pool.NodeInstanceType == workerType {
			return pool.Id, nil
		}
	}
	err = fmt.Errorf("worker_type: %s %w in cluster: %s", workerType, errNotFound, clusterName)
	return "", err
}

// createEnvVars takes a deploymentFromFile, deploymentID and a client as its arguments.
// It updates the deployment identified by deploymentID with requested environment variables.
// It returns an error if it fails to modify the environment variables for a deployment.
func createEnvVars(deploymentFromFile *inspect.FormattedDeployment, deploymentID string, client astro.Client) ([]astro.EnvironmentVariablesObject, error) {
	var (
		updateEnvVarsInput astro.EnvironmentVariablesInput
		listOfVars         []astro.EnvironmentVariable
		envVarObjects      []astro.EnvironmentVariablesObject
		err                error
	)
	requestedVars := deploymentFromFile.Deployment.EnvVars
	listOfVars = make([]astro.EnvironmentVariable, len(requestedVars))
	for i, envVar := range requestedVars {
		listOfVars[i].Key = envVar.Key
		listOfVars[i].IsSecret = envVar.IsSecret
		listOfVars[i].Value = envVar.Value
	}
	updateEnvVarsInput = astro.EnvironmentVariablesInput{
		DeploymentID:         deploymentID,
		EnvironmentVariables: listOfVars,
	}
	envVarObjects, err = client.ModifyDeploymentVariable(updateEnvVarsInput)
	if err != nil {
		return envVarObjects, err
	}
	return envVarObjects, nil
}

// getQueues takes a deploymentFromFile as its arguments.
// It returns a list of worker queues to be created or updated.
func getQueues(deploymentFromFile *inspect.FormattedDeployment, nodePools []astrocore.NodePool, existingQueues []astro.WorkerQueue) ([]astroplatformcore.WorkerQueue, error) {
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
					qList[i].Id = existingQueues[i].ID
				}
			}
		}
		// add new queue or update existing queue properties to list of queues to return
		qList[i].Name = requestedQueues[i].Name
		qList[i].IsDefault = requestedQueues[i].Name == defaultQueue
		if requestedQueues[i].MinWorkerCount != nil {
			qList[i].MinWorkerCount = *requestedQueues[i].MinWorkerCount
		}
		qList[i].MaxWorkerCount = requestedQueues[i].MaxWorkerCount
		qList[i].WorkerConcurrency = requestedQueues[i].WorkerConcurrency
		qList[i].WorkerConcurrency = requestedQueues[i].WorkerConcurrency
		qList[i].PodCpu = requestedQueues[i].PodCPU
		qList[i].PodMemory = requestedQueues[i].PodRAM
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

// createAlertEmails takes a deploymentFromFile and deploymentID and a client as its arguments.
// It creates or updates alert emails for the deployment identified by deploymentID.
// It returns an error if it fails to update the alert emails for a deployment.
func createAlertEmails(deploymentFromFile *inspect.FormattedDeployment, deploymentID string, client astro.Client) (astro.DeploymentAlerts, error) {
	var (
		alertsInput astro.UpdateDeploymentAlertsInput
		alertEmails []string
		alerts      astro.DeploymentAlerts
		err         error
	)

	alertEmails = deploymentFromFile.Deployment.AlertEmails
	alertsInput = astro.UpdateDeploymentAlertsInput{
		DeploymentID: deploymentID,
		AlertEmails:  alertEmails,
	}
	alerts, err = client.UpdateAlertEmails(alertsInput)
	if err != nil {
		return astro.DeploymentAlerts{}, err
	}
	return alerts, nil
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

// isValidExecutor returns true for valid executor values and false if not.
func isValidExecutor(executor string) bool {
	return executor == deployment.CeleryExecutor || executor == deployment.KubeExecutor
}

// temporary code
func transformDeploymentType(deploymentType string) astroplatformcore.DeploymentType {
	var transformedDeploymentType astroplatformcore.DeploymentType
	if deploymentType == "STANDARD" || deploymentType == "standard" {
		transformedDeploymentType = astroplatformcore.DeploymentTypeSTANDARD
	}
	if deploymentType == "DEDICATED" || deploymentType == "dedicated" {
		transformedDeploymentType = astroplatformcore.DeploymentTypeDEDICATED
	}
	if deploymentType == "HYBRID" || deploymentType == "hybrid" {
		transformedDeploymentType = astroplatformcore.DeploymentTypeHYBRID
	}
	return transformedDeploymentType
}
