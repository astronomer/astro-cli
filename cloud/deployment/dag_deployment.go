package deployment

import (
	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/pkg/errors"
)

func Initiate(deploymentID string, client astro.Client) (astro.InitiateDagDeployment, error) {
	// create initiate dag deployment input
	initiateDagDeploymentInput := astro.InitiateDagDeploymentInput{
		DeploymentID: deploymentID,
	}

	// initiate dag deployment
	dagDeployment, err := client.InitiateDagDeployment(initiateDagDeploymentInput)
	if err != nil {
		return astro.InitiateDagDeployment{}, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	return dagDeployment, nil
}

func ReportDagDeploymentStatus(initiatedDagDeploymentID, deploymentID, action, versionID, status, message string, client astro.Client) (astro.DagDeploymentStatus, error) {
	// create report dag deployment status input
	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		DeploymentID:             deploymentID,
		Action:                   action,
		VersionID:                versionID,
		Status:                   status,
		Message:                  message,
	}

	// report dag deployment status
	dagDeploymentStatus, err := client.ReportDagDeploymentStatus(reportDagDeploymentStatusInput)
	if err != nil {
		return astro.DagDeploymentStatus{}, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	return dagDeploymentStatus, nil
}
