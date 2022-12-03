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
)

// TODO we need an io.Writer to create happy path output
func Create(inputFile string, client astro.Client) error {
	var (
		err                 error
		errHelp             string
		dataBytes           []byte
		formattedDeployment inspect.FormattedDeployment
		createInput         astro.CreateDeploymentInput
		existingDeployments []astro.Deployment
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
	// transform formattedDeployment to DeploymentCreateInput
	createInput = getCreateInput(&formattedDeployment)
	// TODO should we check if deployment exists before creating it?
	// map names to id
	// yes we should check for existence
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	existingDeployments, err = client.ListDeployments(c.Organization, c.Workspace)
	if err != nil {
		return err
	}
	// check if deployment exists
	if deploymentExists(existingDeployments, &createInput) {
		// create does not allow updating existing deployments
		errHelp = fmt.Sprintf("use deployment update --from-file %s instead", inputFile)
		return fmt.Errorf("deployment: %s %w: %s", createInput.Label, errCannotUpdateExistingDeployment, errHelp)
	}
	// create the deployment
	_, err = client.CreateDeployment(&createInput)
	if err != nil {
		return fmt.Errorf("%s: %w %+v", err.Error(), errCreateFailed, createInput)
	}
	// TODO add happy path output by calling inspect
	return nil
}

// getCreateInput transforms an inspect.FormattedDeployment into a astro.CreateDeploymentInput
func getCreateInput(deploymentFromFile *inspect.FormattedDeployment) astro.CreateDeploymentInput {
	// TODO add env Vars and **Alert Emails
	createInput := astro.CreateDeploymentInput{
		WorkspaceID:           "",
		ClusterID:             deploymentFromFile.Deployment.Configuration.ClusterID,
		Label:                 deploymentFromFile.Deployment.Configuration.Name,
		Description:           deploymentFromFile.Deployment.Configuration.Description,
		RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
		DagDeployEnabled:      false, // TODO should come from configuration
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

// checkRequiredFields ensures all required fields are present in a inspect.FormattedDeployment.
// It returns errRequiredField if required fields are missing and nil if not.
func checkRequiredFields(deploymentFromFile *inspect.FormattedDeployment) error {
	if deploymentFromFile.Deployment.Configuration.Name == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.name")
	}
	if deploymentFromFile.Deployment.Configuration.ClusterID == "" {
		return fmt.Errorf("%w: %s", errRequiredField, "deployment.configuration.cluster_id")
	}
	return nil
}

// DeploymentExists deploymentToCreate as its argument.
// It returns true if deploymentToCreate exists.
// It returns false if deploymentToCreate does not exist.
func deploymentExists(existingDeployments []astro.Deployment, deploymentToCreate *astro.CreateDeploymentInput) bool {
	// TODO use pointers to make it more efficient
	for _, deployment := range existingDeployments {
		if deployment.Label == deploymentToCreate.Label {
			// deployment exists
			return true
		}
	}
	return false
}
