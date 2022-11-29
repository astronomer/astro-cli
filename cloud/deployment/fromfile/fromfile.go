package fromfile

import (
	"errors"
	"fmt"
	"os"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/ghodss/yaml"
)

var (
	errEmptyFile    = errors.New("has no content")
	errCreateFailed = errors.New("failed to create deployment with input")
)

// TODO we need an io.Writer to create happy path output
func Create(inputFile string, client astro.Client) error {
	var (
		err                 error
		dataBytes           []byte
		formattedDeployment inspect.FormattedDeployment
		createInput         astro.CreateDeploymentInput
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
	// transform formattedDeployment to DeploymentCreateInput
	createInput = getCreateInput(&formattedDeployment)

	// TODO validate required fields
	// TODO should we check if deployment exists before creating it?

	// create the deployment
	_, err = client.CreateDeployment(&createInput)
	if err != nil {
		return fmt.Errorf("%s: %w %+v", err.Error(), errCreateFailed, createInput)
	}
	// TODO add happy path output **should we call inspect?
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
