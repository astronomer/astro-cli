package deployment

import (
	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/pkg/errors"
)



func Initiate(deploymentID, organizationID string, client astro.Client) (astro.InitiateDagDeployment, error) {
	// create initiate dag deployment input
	initiateDagDeploymentInput := astro.InitiateDagDeploymentInput{
		DeploymentID: deploymentID,
		OrganizationID: organizationID,
	}

	// update deployment
	dagDeployment, err := client.InitiateDagDeployment(initiateDagDeploymentInput)
	if err != nil {
		return astro.InitiateDagDeployment{} , errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	return dagDeployment, nil
}