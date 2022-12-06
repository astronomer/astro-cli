package fromfile

import (
	"errors"
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/config"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
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
	// map cluster name to id
	clusterID, err = getClusterIDFromName(formattedDeployment.Deployment.Configuration.ClusterName, c.Organization, client)
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
	// transform formattedDeployment to DeploymentCreateInput
	createInput = getCreateInput(&formattedDeployment, clusterID, workspaceID)
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

// getCreateInput transforms an inspect.FormattedDeployment into astro.CreateDeploymentInput
func getCreateInput(deploymentFromFile *inspect.FormattedDeployment, clusterID, workspaceID string) astro.CreateDeploymentInput {
	// TODO add WorkerQs and Alert Emails using updateDeploymentAlerts mutation

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
	}
	return createInput
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

// getClusterIDFromName takes clusterName and organizationID as its arguments.
// It returns the clusterID if the cluster is found in the organization.
// It returns an errClusterNotFound if the cluster does not exist in the organization.
func getClusterIDFromName(clusterName, organizationID string, client astro.Client) (string, error) {
	var (
		existingClusters []astro.Cluster
		err              error
	)
	existingClusters, err = client.ListClusters(organizationID)
	if err != nil {
		return "", err
	}
	for _, cluster := range existingClusters {
		if cluster.Name == clusterName {
			return cluster.ID, nil
		}
	}
	err = fmt.Errorf("cluster_name: %s %w in organization: %s", clusterName, errNotFound, organizationID)
	return "", err
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

func createWorkerQueues(deploymentFromFile *inspect.FormattedDeployment) []astro.WorkerQueue {
	var (
		qList []astro.WorkerQueue
	)
	requestedQueues := deploymentFromFile.Deployment.WorkerQs
	if len(requestedQueues) > 0 {
		qList = make([]astro.WorkerQueue, len(requestedQueues))
		for i, queue := range requestedQueues {
			qList[i].Name = queue.Name
			qList[i].IsDefault = queue.IsDefault
			qList[i].MinWorkerCount = queue.MinWorkerCount
			qList[i].MaxWorkerCount = queue.MaxWorkerCount
			qList[i].WorkerConcurrency = queue.WorkerConcurrency
			qList[i].NodePoolID = queue.NodePoolID
		}
	}
	return qList
}

// hasEnvVars returns true if environment variables exist in deploymentFromFile.
// it returns false if they don't.
func hasEnvVars(deploymentFromFile *inspect.FormattedDeployment) bool {
	return len(deploymentFromFile.Deployment.EnvVars) > 0
}
