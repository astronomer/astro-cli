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

// ManifestDagDeployInput is the resolved input for a project's dags-only deploy.
// The deployment is already chosen; this reuses the 1.x path's dags transport (create
// deploy, upload the tarball, finalize) against the project's dags/ directory.
type ManifestDagDeployInput struct {
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

// ManifestDagDeployResult reports the outcome for cmd to render.
type ManifestDagDeployResult struct {
	WorkspaceID       string
	RuntimeVersion    string
	DagTarballVersion string
	URL               string
	Git               ManifestDeployGit
}

// ManifestDeployGit is what a manifest deploy recorded about the commit it shipped.
type ManifestDeployGit struct {
	// Commit is the git metadata sent with the deploy, nil when none was.
	Commit      *astrov1.CreateDeployGitRequest
	Uncommitted bool
}

// readManifestDeployGit reads the git metadata a manifest deploy records, under the 1.x path's
// rules: none when deploy.git_metadata is off, and none when the tree has
// uncommitted changes, since HEAD would not describe the files deployed. The
// commit message comes back as the description fallback.
func readManifestDeployGit(projectDir string) (info ManifestDeployGit, commitMessage string) {
	if !config.CFG.DeployGitMetadata.GetBool() {
		return ManifestDeployGit{}, ""
	}
	if git.HasUncommittedChanges(projectDir) {
		return ManifestDeployGit{Uncommitted: true}, ""
	}
	commit, message := readHeadGitMetadata(projectDir)
	return ManifestDeployGit{Commit: commit}, message
}

func descriptionOrCommitMessage(description, commitMessage string) string {
	if description != "" {
		return description
	}
	return commitMessage
}

// DeployManifestDags deploys only the dags/ directory of a project to an already
// resolved deployment. It reads the runtime version and type from the
// server-side deployment — a project ships no image, so a dags-only deploy
// must fit the image already running — then reuses the 1.x path's dags transport.
//
// Unlike the 1.x path's Deploy(), it neither prints nor exits: it returns a result for
// cmd to render, so the path stays cancellable and ready for --output json. It
// reuses createDeploy and deployDags as they are, and finalizes through a
// print-free helper rather than the 1.x path's finalizeDeploy, which prints.
func DeployManifestDags(in ManifestDagDeployInput, astroV1Client astrov1.APIClient) (ManifestDagDeployResult, error) {
	c, err := loginOrCurrent(in.Login)
	if err != nil {
		return ManifestDagDeployResult{}, err
	}

	// Read the deployment's server-side facts: runtime version and type drive
	// the dags transport, and the flags gate the deploy.
	dep, err := deployment.GetDeploymentByID(c.Organization, in.DeploymentID, astroV1Client)
	if err != nil {
		return ManifestDagDeployResult{}, err
	}

	if dep.IsCicdEnforced && !canCiCdDeploy(c.Token) {
		return ManifestDagDeployResult{}, fmt.Errorf(errCiCdEnforcementUpdate, dep.Name)
	}
	if !dep.IsDagDeployEnabled {
		return ManifestDagDeployResult{}, fmt.Errorf(enableDagDeployMsg, in.DeploymentID)
	}

	gitInfo, commitMessage := readManifestDeployGit(in.ProjectDir)
	description := descriptionOrCommitMessage(in.Description, commitMessage)
	created, err := createDeploy(dep.OrganizationId, dep.Id, astrov1.CreateDeployRequest{
		Description: &description,
		Type:        astrov1.CreateDeployRequestTypeDAGONLY,
		Git:         gitInfo.Commit,
	}, astroV1Client)
	if err != nil {
		return ManifestDagDeployResult{}, explainHibernating(err, &dep)
	}

	tarballVersion, err := uploadDeployDags(&c, in.ProjectDir, in.DeploymentID, &dep, created, in.NoDagsBaseDir)
	if err != nil {
		return ManifestDagDeployResult{}, err
	}

	if err := finalizeManifestDeploy(dep.OrganizationId, dep.Id, created.Id, tarballVersion, astroV1Client); err != nil {
		return ManifestDagDeployResult{}, err
	}

	if in.Wait {
		if err := deployment.HealthPollIn(dep.OrganizationId, c.Token, dep.Id, dagOnlyDeploySleepTime, tickNum, int(in.WaitTime.Seconds()), astroV1Client); err != nil {
			return ManifestDagDeployResult{}, err
		}
	}

	return ManifestDagDeployResult{
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
