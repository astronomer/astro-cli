package deploy

import (
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/git"
)

// DagDeployV2Input is the resolved input for a project's dags-only deploy.
// The deployment is already chosen; this reuses the v1 dags transport (create
// deploy, upload the tarball, finalize) against the project's dags/ directory.
type DagDeployV2Input struct {
	// Login is the Astro login the deploy runs under, whose host the
	// Deployment lives on; nil is the current context.
	Login *config.Context
	// ProjectDir is the project root; dags/ sits under it.
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
	Git               DeployGitV2
}

// DeployGitV2 is what a manifest deploy recorded about the commit it shipped.
type DeployGitV2 struct {
	// Commit is the git metadata sent with the deploy, nil when none was.
	Commit      *astrov1.CreateDeployGitRequest
	Uncommitted bool
}

// readDeployGitV2 reads the git metadata a manifest deploy records, under v1's
// rules: none when deploy.git_metadata is off, and none when the tree has
// uncommitted changes, since HEAD would not describe the files deployed. The
// commit message comes back as the description fallback.
func readDeployGitV2(projectDir string) (info DeployGitV2, commitMessage string) {
	if !config.CFG.DeployGitMetadata.GetBool() {
		return DeployGitV2{}, ""
	}
	if git.HasUncommittedChanges(projectDir) {
		return DeployGitV2{Uncommitted: true}, ""
	}
	commit, message := readHeadGitMetadata(projectDir)
	return DeployGitV2{Commit: commit}, message
}

func descriptionOrCommitMessage(description, commitMessage string) string {
	if description != "" {
		return description
	}
	return commitMessage
}

// DeployDagsV2 deploys only the dags/ directory of a project to an already
// resolved deployment. It reads the runtime version and type from the
// server-side deployment — a project ships no image, so a dags-only deploy
// must fit the image already running — then reuses the v1 dags transport.
//
// Unlike the v1 Deploy(), it neither prints nor exits: it returns a result for
// cmd to render, so the path stays cancellable and ready for --output json. It
// reuses createDeploy and deployDags as they are, and finalizes through a
// print-free helper rather than the v1 finalizeDeploy, which prints.
func DeployDagsV2(in DagDeployV2Input, astroV1Client astrov1.APIClient) (DagDeployV2Result, error) {
	c, err := loginOrCurrent(in.Login)
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

	gitInfo, commitMessage := readDeployGitV2(in.ProjectDir)
	description := descriptionOrCommitMessage(in.Description, commitMessage)
	created, err := createDeploy(dep.OrganizationId, dep.Id, astrov1.CreateDeployRequest{
		Description: &description,
		Type:        astrov1.CreateDeployRequestTypeDAGONLY,
		Git:         gitInfo.Commit,
	}, astroV1Client)
	if err != nil {
		return DagDeployV2Result{}, explainHibernating(err, &dep)
	}

	tarballVersion, err := uploadDeployDags(&c, in.ProjectDir, in.DeploymentID, &dep, created, in.NoDagsBaseDir)
	if err != nil {
		return DagDeployV2Result{}, err
	}

	if err := finalizeDeployV2(dep.OrganizationId, dep.Id, created.Id, tarballVersion, astroV1Client); err != nil {
		return DagDeployV2Result{}, err
	}

	if in.Wait {
		if err := deployment.HealthPollIn(dep.OrganizationId, c.Token, dep.Id, dagOnlyDeploySleepTime, tickNum, int(in.WaitTime.Seconds()), astroV1Client); err != nil {
			return DagDeployV2Result{}, err
		}
	}

	return DagDeployV2Result{
		WorkspaceID:       dep.WorkspaceId,
		RuntimeVersion:    dep.AstroRuntimeVersion,
		DagTarballVersion: tarballVersion,
		URL:               dashboardURL(c.Domain, dep.Id, dep.WorkspaceId),
		Git:               gitInfo,
	}, nil
}

// loginOrCurrent is the login a manifest deploy runs under: the one cmd picked for
// the project's host, else the current context.
func loginOrCurrent(login *config.Context) (config.Context, error) {
	if login != nil {
		return *login, nil
	}
	return config.GetCurrentContext()
}

func dashboardURL(domain, deploymentID, workspaceID string) string {
	url := deployment.DeploymentURLOn(domain, deploymentID, workspaceID)
	if domain == domainutil.LocalDomain {
		return "http://" + url
	}
	return "https://" + url
}
