package deploy

import (
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// DagDeployV2Input is the resolved input for a v2 project's dags-only deploy.
// The deployment is already chosen; this reuses the v1 dags transport (create
// deploy, upload the tarball, finalize) against the project's dags/ directory.
type DagDeployV2Input struct {
	// ProjectDir is the v2 project root; dags/ sits under it.
	ProjectDir    string
	DeploymentID  string
	Description   string
	NoDagsBaseDir bool
	Wait          bool
	WaitTime      time.Duration
}

// DagDeployV2Result reports the outcome for cmd to render.
type DagDeployV2Result struct {
	WorkspaceID       string
	RuntimeVersion    string
	DagTarballVersion string
	URL               string
}

// DeployDagsV2 deploys only the dags/ directory of a v2 project to an already
// resolved deployment. It reads the runtime version and type from the
// server-side deployment — a v2 project ships no image, so a dags-only deploy
// must fit the image already running — then reuses the v1 dags transport.
//
// Unlike the v1 Deploy(), it neither prints nor exits: it returns a result for
// cmd to render, so the path stays cancellable and ready for --output json. It
// reuses createDeploy and deployDags as they are, and finalizes through a
// print-free helper rather than the v1 finalizeDeploy, which prints.
func DeployDagsV2(in DagDeployV2Input, astroV1Client astrov1.APIClient) (DagDeployV2Result, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return DagDeployV2Result{}, err
	}

	// Read the deployment's server-side facts: runtime version and type drive
	// the dags transport, and the flags gate the deploy.
	dep, err := deployment.GetDeploymentByID(c.Organization, in.DeploymentID, astroV1Client)
	if err != nil {
		return DagDeployV2Result{}, err
	}

	if dep.IsCicdEnforced && !canCiCdDeploy(c.Token) {
		return DagDeployV2Result{}, fmt.Errorf(errCiCdEnforcementUpdate, dep.Name)
	}
	if !dep.IsDagDeployEnabled {
		return DagDeployV2Result{}, fmt.Errorf(enableDagDeployMsg, in.DeploymentID)
	}

	created, err := createDeploy(dep.OrganizationId, dep.Id, astrov1.CreateDeployRequest{
		Description: &in.Description,
		Type:        astrov1.CreateDeployRequestTypeDAGONLY,
	}, astroV1Client)
	if err != nil {
		return DagDeployV2Result{}, err
	}

	tarballVersion, err := uploadDeployDags(in.ProjectDir, in.DeploymentID, &dep, created, in.NoDagsBaseDir)
	if err != nil {
		return DagDeployV2Result{}, err
	}

	if err := finalizeDeployV2(dep.OrganizationId, dep.Id, created.Id, tarballVersion, astroV1Client); err != nil {
		return DagDeployV2Result{}, err
	}

	if in.Wait {
		if err := deployment.HealthPoll(dep.Id, dep.WorkspaceId, dagOnlyDeploySleepTime, tickNum, int(in.WaitTime.Seconds()), astroV1Client); err != nil {
			return DagDeployV2Result{}, err
		}
	}

	url, err := deployment.GetDeploymentURL(dep.Id, dep.WorkspaceId)
	if err != nil {
		return DagDeployV2Result{}, err
	}

	return DagDeployV2Result{
		WorkspaceID:       dep.WorkspaceId,
		RuntimeVersion:    dep.AstroRuntimeVersion,
		DagTarballVersion: tarballVersion,
		URL:               url,
	}, nil
}
