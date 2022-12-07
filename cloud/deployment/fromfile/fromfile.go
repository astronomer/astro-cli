package fromfile

import (
	"errors"
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"
	"github.com/astronomer/astro-cli/config"
	"github.com/ghodss/yaml"
)

var (
	errEmptyFile                      = errors.New("has no content")
	errCreateFailed                   = errors.New("failed to create deployment with input")
	errRequiredField                  = errors.New("missing required field")
	errCannotUpdateExistingDeployment = errors.New("already exists")
	errNotFound                       = errors.New("does not exist")
)

// TODO we need an io.Writer to create happy path output
func Create(inputFile string, client astro.Client) error {
	var (
		err                             error
		errHelp, clusterID, workspaceID string
		dataBytes                       []byte
		formattedDeployment             inspect.FormattedDeployment
		createInput                     astro.CreateDeploymentInput
		existingDeployments             []astro.Deployment
		createdDeployment               astro.Deployment
		nodePools                       []astro.NodePool
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
	// unmarshal to a formattedDeployment
	// TODO make this a var so we can test it
	err = yaml.Unmarshal(dataBytes, &formattedDeployment)
	if err != nil {
		return err
	}
	// validate required fields
	err = checkRequiredFields(&formattedDeployment)
	if err != nil {
		return err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	// map workspace name to id
	workspaceID, err = getWorkspaceIDFromName(formattedDeployment.Deployment.Configuration.WorkspaceName, c.Organization, client)
	if err != nil {
		return err
	}
	// map cluster name to id and collect node pools for cluster
	clusterID, nodePools, err = getClusterInfoFromName(formattedDeployment.Deployment.Configuration.ClusterName, c.Organization, client)
	if err != nil {
		return err
	}
	existingDeployments, err = client.ListDeployments(c.Organization, workspaceID)
	if err != nil {
		return err
	}
	// check if deployment exists
	if deploymentExists(existingDeployments, formattedDeployment.Deployment.Configuration.Name) {
		// create does not allow updating existing deployments
		errHelp = fmt.Sprintf("use deployment update --from-file %s instead", inputFile)
		return fmt.Errorf("deployment: %s %w: %s", formattedDeployment.Deployment.Configuration.Name,
			errCannotUpdateExistingDeployment, errHelp)
	}
	// this deployment does not exist so create it

	// transform formattedDeployment to DeploymentCreateInput
	createInput, err = getCreateInput(&formattedDeployment, clusterID, workspaceID, nodePools, client)
	if err != nil {
		return err
	}
	// create the deployment
	createdDeployment, err = client.CreateDeployment(&createInput)
	if err != nil {
		return fmt.Errorf("%s: %w %+v", err.Error(), errCreateFailed, createInput)
	}
	// create environment variables
	if hasEnvVars(&formattedDeployment) {
		_, err = createEnvVars(&formattedDeployment, createdDeployment.ID, client)
		if err != nil {
			return err
		}
	}
	// TODO add happy path output by calling inspect
	return nil
}

// getCreateInput transforms an inspect.FormattedDeployment into astro.CreateDeploymentInput.
// If worker-queues were requested, it gets node pool id work the workers and validates queue options.
// If no queue options were specified, it sets default values.
// It returns an error if getting default options fail.
// It returns an error if worker-queue options are not valid.
// It returns an error if node pool id could not be found for the worker type.
func getCreateInput(deploymentFromFile *inspect.FormattedDeployment, clusterID, workspaceID string, nodePools []astro.NodePool, client astro.Client) (astro.CreateDeploymentInput, error) {
	var (
		defaultOptions astro.WorkerQueueDefaultOptions
		listQueues     []astro.WorkerQueue
		err            error
	)

	// TODO add Alert Emails using updateDeploymentAlerts mutation
	// Add worker queues if they were requested
	if hasQueues(deploymentFromFile) {
		// get defaults for min-count, max-count and concurrency from API
		defaultOptions, err = workerqueue.GetWorkerQueueDefaultOptions(client)
		if err != nil {
			return astro.CreateDeploymentInput{}, err
		}
		// transform inspect.WorkerQ to []astro.WorkerQueue
		listQueues, err = getQueues(deploymentFromFile, nodePools)
		if err != nil {
			return astro.CreateDeploymentInput{}, err
		}
		for i, q := range listQueues {
			// set default values if none were specified
			a := workerqueue.SetWorkerQueueValues(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, &q, defaultOptions)
			// check if queue is valid
			err = workerqueue.IsWorkerQueueInputValid(a, defaultOptions)
			if err != nil {
				return astro.CreateDeploymentInput{}, err
			}
			// add it to the list of queues to be created
			listQueues[i] = *a
		}
	}
	createInput := astro.CreateDeploymentInput{
		WorkspaceID:           workspaceID,
		ClusterID:             clusterID,
		Label:                 deploymentFromFile.Deployment.Configuration.Name,
		Description:           deploymentFromFile.Deployment.Configuration.Description,
		RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
		DagDeployEnabled:      deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
				Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
			},
		},
		WorkerQueues: listQueues,
	}
	return createInput, nil
}

// checkRequiredFields ensures all required fields are present in inspect.FormattedDeployment.
// It returns errRequiredField if required fields are missing and nil if not.
func checkRequiredFields(deploymentFromFile *inspect.FormattedDeployment) error {
	if deploymentFromFile.Deployment.Configuration.Name == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.name")
	}
	if deploymentFromFile.Deployment.Configuration.ClusterName == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.cluster_name")
	}
	// TODO check queue name, isDefault and worker type
	return nil
}

// DeploymentExists deploymentToCreate as its argument.
// It returns true if deploymentToCreate exists.
// It returns false if deploymentToCreate does not exist.
func deploymentExists(existingDeployments []astro.Deployment, deploymentNameToCreate string) bool {
	// TODO use pointers to make it more efficient
	for _, deployment := range existingDeployments {
		if deployment.Label == deploymentNameToCreate {
			// deployment exists
			return true
		}
	}
	return false
}

// getClusterInfoFromName takes clusterName and organizationID as its arguments.
// It returns the clusterID and list of nodepools if the cluster is found in the organization.
// It returns an errClusterNotFound if the cluster does not exist in the organization.
func getClusterInfoFromName(clusterName, organizationID string, client astro.Client) (string, []astro.NodePool, error) {
	var (
		existingClusters []astro.Cluster
		err              error
	)
	existingClusters, err = client.ListClusters(organizationID)
	if err != nil {
		return "", nil, err
	}
	for _, cluster := range existingClusters {
		if cluster.Name == clusterName {
			return cluster.ID, cluster.NodePools, nil
		}
	}
	err = fmt.Errorf("cluster_name: %s %w in organization: %s", clusterName, errNotFound, organizationID)
	return "", nil, err
}

// getWorkspaceIDFromName takes workspaceName and organizationID as its arguments.
// It returns the workspaceID if the workspace is found in the organization.
// It returns an errWorkspaceNotFound if the workspace does not exist in the organization.
func getWorkspaceIDFromName(workspaceName, organizationID string, client astro.Client) (string, error) {
	var (
		existingWorkspaces []astro.Workspace
		err                error
	)
	existingWorkspaces, err = client.ListWorkspaces(organizationID)
	if err != nil {
		return "", err
	}
	for _, workspace := range existingWorkspaces {
		if workspace.Label == workspaceName {
			return workspace.ID, nil
		}
	}
	err = fmt.Errorf("workspace_name: %s %w in organization: %s", workspaceName, errNotFound, organizationID)
	return "", err
}

func getNodePoolIDFromWorkerType(workerType, clusterName string, nodePools []astro.NodePool) (string, error) {
	var (
		pool astro.NodePool
		err  error
	)
	for _, pool = range nodePools {
		if pool.NodeInstanceType == workerType {
			return pool.ID, nil
		}
	}
	err = fmt.Errorf("worker_type: %s %w in cluster: %s", workerType, errNotFound, clusterName)
	return "", err
}

// createEnvVars takes a deploymentFromFile and deploymentID as its arguments.
// If environment variables were requested in the deploymentFromFile, it creates them.
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
// It returns a list of worker queues to be created.
func getQueues(deploymentFromFile *inspect.FormattedDeployment, nodePools []astro.NodePool) ([]astro.WorkerQueue, error) {
	var (
		qList      []astro.WorkerQueue
		nodePoolID string
		err        error
	)
	requestedQueues := deploymentFromFile.Deployment.WorkerQs
	qList = make([]astro.WorkerQueue, len(requestedQueues))
	for i, queue := range requestedQueues {
		qList[i].Name = queue.Name
		qList[i].IsDefault = queue.IsDefault
		qList[i].MinWorkerCount = queue.MinWorkerCount
		qList[i].MaxWorkerCount = queue.MaxWorkerCount
		qList[i].WorkerConcurrency = queue.WorkerConcurrency
		// map worker type to node pool id
		nodePoolID, err = getNodePoolIDFromWorkerType(queue.WorkerType, deploymentFromFile.Deployment.Configuration.ClusterName, nodePools)
		if err != nil {
			return nil, err
		}
		qList[i].NodePoolID = nodePoolID
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
