package fromfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const (
	jsonFormat = "json"
)

// Create takes a file and creates a deployment with the confiuration specified in the file.
// inputFile can be in yaml or json format
// It returns an error if any required information is missing or incorrectly specified.
func Create(inputFile string, client astro.Client, out io.Writer) error {
	var (
		err                                           error
		errHelp, clusterID, workspaceID, outputFormat string
		dataBytes                                     []byte
		formattedDeployment                           inspect.FormattedDeployment
		createInput                                   astro.CreateDeploymentInput
		existingDeployments                           []astro.Deployment
		createdDeployment                             astro.Deployment
		nodePools                                     []astro.NodePool
		jsonOutput                                    bool
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
	// create alert emails
	if hasAlertEmails(&formattedDeployment) {
		_, err = createAlertEmails(&formattedDeployment, createdDeployment.ID, client)
		if err != nil {
			return err
		}
	}
	// TODO add happy path output by calling inspect
	if jsonOutput {
		outputFormat = jsonFormat
	}
	return inspect.Inspect(workspaceID, "", createdDeployment.ID, outputFormat, client, out, "")
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
		for i := range listQueues {
			// set default values if none were specified
			a := workerqueue.SetWorkerQueueValues(listQueues[i].MinWorkerCount, listQueues[i].MaxWorkerCount, listQueues[i].WorkerConcurrency, &listQueues[i], defaultOptions)
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
	// if worker queues were requested check queue name, isDefault and worker type
	if hasQueues(deploymentFromFile) {
		for i, queue := range deploymentFromFile.Deployment.WorkerQs {
			if queue.Name == "" {
				missingField := fmt.Sprintf("deployment.worker_queues[%d].name", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
			if queue.IsDefault && queue.Name != "default" {
				missingField := fmt.Sprintf("deployment.worker_queues[%d].name = default", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
			if queue.WorkerType == "" {
				missingField := fmt.Sprintf("deployment.worker_queues[%d].worker_type", i)
				return fmt.Errorf("%w: %s", errRequiredField, missingField)
			}
		}
	}
	return nil
}

// DeploymentExists deploymentToCreate as its argument.
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
	for i := range existingWorkspaces {
		if existingWorkspaces[i].Label == workspaceName {
			return existingWorkspaces[i].ID, nil
		}
	}
	err = fmt.Errorf("workspace_name: %s %w in organization: %s", workspaceName, errNotFound, organizationID)
	return "", err
}

// getNodePoolIDFromWorkerType maps the node pool id in nodePools to a worker type.
// It returns an error if the node pool id does not exist in any node pool in nodePools.
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

// createEnvVars takes a deploymentFromFile, deploymentID and a client as its arguments.
// It updates the deployment identified by deploymentID with environment variables.
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

// hasAlertEmails returns true if alert emails exist in deploymentFromFile.
// it returns false if they don't.
func hasAlertEmails(deploymentFromFile *inspect.FormattedDeployment) bool {
	return len(deploymentFromFile.Deployment.AlertEmails) > 0
}

// createAlertEmails takes a deploymentFromFile and deploymentID and a client as its arguments.
// It creates alert emails for the deplyment identified by deploymentID.
// It returns an error if it fails to update the alert emails for a deployment.
func createAlertEmails(deploymentFromFile *inspect.FormattedDeployment, deploymentID string, client astro.Client) (astro.DeploymentAlerts, error) {
	var (
		input       astro.UpdateDeploymentAlertsInput
		alertEmails []string
		alerts      astro.DeploymentAlerts
		err         error
	)

	alertEmails = deploymentFromFile.Deployment.AlertEmails
	input = astro.UpdateDeploymentAlertsInput{
		DeploymentID: deploymentID,
		AlertEmails:  alertEmails,
	}
	alerts, err = client.UpdateAlertEmails(input)
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
