package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/cosmosboost"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/logger"
)

type DeployBundleInput struct {
	BundlePath   string
	MountPath    string
	DeploymentID string
	// Deployment is the Deployment as the caller already read it (from the
	// picker, or by name); nil reads it by DeploymentID.
	Deployment    *astrov1.Deployment
	BundleType    string
	Description   string
	AstroV1Client astrov1.APIClient
}

// BundleGit is the commit a bundle deploy recorded, when the bundle sits in a
// git checkout with no uncommitted changes.
type BundleGit struct {
	CommitSHA string
	Branch    string
	CommitURL string
}

// BundleDeploy is what a bundle deploy did: the deploy it created on the
// Deployment and the version of the bundle it uploaded. Waiting for the
// Deployment to take it is the caller's (WaitForBundle), so a caller can
// report the upload before the wait begins.
type BundleDeploy struct {
	DeploymentID   string
	DeploymentName string
	WorkspaceID    string
	DeployID       string
	MountPath      string
	BundleVersion  string
	Git            *BundleGit
}

// DeployBundle uploads the bundle at input.BundlePath to the Deployment and
// finalizes the deploy. It does not report the result: its caller renders the
// BundleDeploy it returns.
func DeployBundle(input *DeployBundleInput) (BundleDeploy, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return BundleDeploy{}, err
	}

	// the deployment, to check the deploy is valid: as the caller read it, or
	// read now when the caller knew only its id
	currentDeployment, err := deploymentFor(c.Organization, input.DeploymentID, input.Deployment, input.AstroV1Client)
	if err != nil {
		return BundleDeploy{}, err
	}

	// if CI/CD is enforced, check the subject can deploy
	if currentDeployment.IsCicdEnforced && !canCiCdDeploy(c.Token) {
		return BundleDeploy{}, fmt.Errorf(errCiCdEnforcementUpdate, currentDeployment.Name)
	}

	// check the deployment is enabled for DAG deploys
	if !currentDeployment.IsDagDeployEnabled {
		return BundleDeploy{}, fmt.Errorf(enableDagDeployMsg, input.DeploymentID)
	}

	// Check if git metadata is enabled (default: true)
	var deployGit *astrov1.CreateDeployGitRequest
	var commitMessage string
	if config.CFG.DeployGitMetadata.GetBool() {
		deployGit, commitMessage = retrieveLocalGitMetadata(input.BundlePath)
	}

	// if no description was provided, use the commit message from the local Git checkout
	if input.Description == "" {
		input.Description = commitMessage
	}

	// initialize the deploy
	deploy, err := createBundleDeploy(c.Organization, input, deployGit, input.AstroV1Client)
	if err != nil {
		return BundleDeploy{}, explainHibernating(err, &currentDeployment)
	}

	// check we received an upload URL
	if deploy.BundleUploadUrl == nil {
		return BundleDeploy{}, errors.New("no bundle upload URL received from Astro")
	}

	// upload the bundle
	tarballVersion, err := UploadBundle(config.WorkingPath, input.BundlePath, *deploy.BundleUploadUrl, false, currentDeployment.AstroRuntimeVersion)
	if err != nil {
		return BundleDeploy{}, err
	}

	// finalize the deploy
	err = finalizeBundleDeploy(c.Organization, input.DeploymentID, deploy.Id, tarballVersion, input.AstroV1Client)
	if err != nil {
		return BundleDeploy{}, err
	}

	res := BundleDeploy{
		DeploymentID:   currentDeployment.Id,
		DeploymentName: currentDeployment.Name,
		WorkspaceID:    currentDeployment.WorkspaceId,
		DeployID:       deploy.Id,
		MountPath:      input.MountPath,
		BundleVersion:  tarballVersion,
	}
	if deployGit != nil {
		res.Git = &BundleGit{CommitSHA: deployGit.CommitSha}
		if deployGit.Branch != nil {
			res.Git.Branch = *deployGit.Branch
		}
		if deployGit.CommitUrl != nil {
			res.Git.CommitURL = *deployGit.CommitUrl
		}
	}
	return res, nil
}

// deploymentFor is read when the caller has it, and the Deployment with id
// read from Astro otherwise.
func deploymentFor(orgID, id string, read *astrov1.Deployment, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if read != nil {
		return *read, nil
	}
	return deployment.GetDeploymentByID(orgID, id, astroV1Client)
}

// WaitForBundle waits up to waitTime for the Deployment to become healthy
// after a bundle deploy or delete, writing its progress to progress (nil is
// stderr).
func WaitForBundle(progress io.Writer, deploymentID string, waitTime time.Duration, astroV1Client astrov1.APIClient) error {
	return deployment.HealthPoll(progress, deploymentID, dagOnlyDeploySleepTime, tickNum, int(waitTime.Seconds()), astroV1Client)
}

type DeleteBundleInput struct {
	MountPath    string
	DeploymentID string
	// Deployment is the Deployment as the caller already read it (from the
	// picker, or by name), nil when it knew only DeploymentID. A delete reads
	// nothing to fill it in: it names the Workspace only from this, and
	// explains a hibernating refusal with what it has.
	Deployment    *astrov1.Deployment
	BundleType    string
	Description   string
	AstroV1Client astrov1.APIClient
}

// BundleDelete is what a bundle delete did: the deploy that removes the
// bundle at MountPath from the Deployment. Astro removes it as the
// Deployment takes that deploy, which WaitForBundle waits for. WorkspaceID
// is empty when the caller did not know it.
type BundleDelete struct {
	DeploymentID string
	WorkspaceID  string
	DeployID     string
	MountPath    string
}

// DeleteBundle requests the removal of the bundle mounted at input.MountPath.
// It does not report the result: its caller renders the BundleDelete it
// returns.
func DeleteBundle(input *DeleteBundleInput) (BundleDelete, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return BundleDelete{}, err
	}

	// initialize the deploy
	createInput := &DeployBundleInput{
		MountPath:    input.MountPath,
		DeploymentID: input.DeploymentID,
		BundleType:   input.BundleType,
		Description:  input.Description,
	}
	deploy, err := createBundleDeploy(c.Organization, createInput, nil, input.AstroV1Client)
	if err != nil {
		known := input.Deployment
		if known == nil {
			known = &astrov1.Deployment{Id: input.DeploymentID}
		}
		return BundleDelete{}, explainHibernating(err, known)
	}

	// immediately finalize with no version, which will delete the bundle from the deployment
	err = finalizeBundleDeploy(c.Organization, input.DeploymentID, deploy.Id, "", input.AstroV1Client)
	if err != nil {
		return BundleDelete{}, err
	}
	res := BundleDelete{
		DeploymentID: input.DeploymentID,
		DeployID:     deploy.Id,
		MountPath:    input.MountPath,
	}
	if input.Deployment != nil {
		res.WorkspaceID = input.Deployment.WorkspaceId
	}
	return res, nil
}

// ValidateBundleSymlinks checks if any symlinks within the bundlePath point outside of it
func ValidateBundleSymlinks(bundlePath string) error {
	absBundlePath, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for bundle directory: %w", err)
	}

	err = filepath.WalkDir(bundlePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err // Propagate errors from WalkDir itself
		}

		// Check only for symlinks
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				logger.Debugf("Could not read symlink %s: %v", path, err)
				return nil
			}

			// If the target is not absolute, join it with the directory containing the link
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}

			// Get the absolute path of the target
			absTarget, err := filepath.Abs(target)
			if err != nil {
				logger.Debugf("Could not get absolute path for symlink target %s -> %s: %v", path, target, err)
				return nil
			}

			// Check if the absolute target path is outside the absolute bundle path directory
			if !strings.HasPrefix(absTarget, absBundlePath) {
				return fmt.Errorf("symlink %s points to %s which is outside the bundle directory %s", path, target, absBundlePath)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("bundle validation failed: %w", err)
	}

	return nil
}

func UploadBundle(tarDirPath, bundlePath, uploadURL string, prependBaseDir bool, currentRuntimeVersion string) (string, error) {
	// If Airflow 3.x, check for symlinks pointing outside the bundle directory
	if airflowversions.AirflowMajorVersionForRuntimeVersion(currentRuntimeVersion) == "3" {
		err := ValidateBundleSymlinks(bundlePath)
		if err != nil {
			return "", err
		}
	}

	tarFilePath := filepath.Join(tarDirPath, "bundle.tar")
	tarGzFilePath := tarFilePath + ".gz"
	defer func() {
		tarFiles := []string{tarFilePath, tarGzFilePath}
		for _, file := range tarFiles {
			err := os.Remove(file)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				fmt.Println("\nFailed to delete archived file: ", err.Error())
				fmt.Println("\nPlease delete the archived file manually from path: " + file)
			}
		}
	}()

	// Cosmos Boost pre-deploy step, opt-in via cosmos_boost.pre_deploy; with
	// the setting off the deploy does not touch the tree at all. Cleanup runs
	// first and is fatal on failure (a stale artifact must not ship inside the
	// bundle), while stamping is best-effort (a missing artifact is safe).
	// Artifacts left by earlier enabled deploys are removed with
	// `astro dbt cleanup`.
	if config.CFG.CosmosBoostPreDeploy.GetBool() {
		if err := cosmosboost.EnsureClean(bundlePath); err != nil {
			return "", err
		}
		cosmosboost.BestEffortPreDeploy(bundlePath)
	}

	// Generate the bundle tar
	err := fileutil.Tar(bundlePath, tarFilePath, prependBaseDir, []string{".git/"})
	if err != nil {
		return "", err
	}

	// Gzip the tar
	err = fileutil.GzipFile(tarFilePath, tarGzFilePath)
	if err != nil {
		return "", err
	}

	tarGzFile, err := os.Open(tarGzFilePath)
	if err != nil {
		return "", err
	}
	defer tarGzFile.Close()

	versionID, err := azureUploader(uploadURL, tarGzFile)
	if err != nil {
		return "", err
	}

	return versionID, nil
}

func createBundleDeploy(organizationID string, input *DeployBundleInput, deployGit *astrov1.CreateDeployGitRequest, astroV1Client astrov1.APIClient) (*astrov1.Deploy, error) {
	request := astrov1.CreateDeployRequest{
		Description:     &input.Description,
		Type:            astrov1.CreateDeployRequestTypeBUNDLE,
		BundleMountPath: &input.MountPath,
		BundleType:      &input.BundleType,
		Git:             deployGit,
	}
	resp, err := astroV1Client.CreateDeployWithResponse(context.Background(), organizationID, input.DeploymentID, request)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func finalizeBundleDeploy(organizationID, deploymentID, deployID, tarballVersion string, astroV1Client astrov1.APIClient) error {
	request := astrov1.FinalizeDeployRequest{
		BundleTarballVersion: &tarballVersion,
	}
	resp, err := astroV1Client.FinalizeDeployWithResponse(context.Background(), organizationID, deploymentID, deployID, request)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

// retrieveLocalGitMetadata retrieves git metadata from the local repository for deploy tracking.
// Returns nil and empty string if the repository has uncommitted changes or if git metadata cannot be retrieved.
func retrieveLocalGitMetadata(bundlePath string) (deployGit *astrov1.CreateDeployGitRequest, commitMessage string) {
	if git.HasUncommittedChanges(bundlePath) {
		fmt.Println("Local repository has uncommitted changes, skipping Git metadata retrieval")
		return nil, ""
	}
	return readHeadGitMetadata(bundlePath)
}

// readHeadGitMetadata describes the HEAD commit of the repository holding
// bundlePath, without checking for uncommitted changes. Returns nil and empty
// string if git metadata cannot be retrieved.
func readHeadGitMetadata(bundlePath string) (deployGit *astrov1.CreateDeployGitRequest, commitMessage string) {
	// get the raw remote URL (needed for the GENERIC provider), assume the remote is named "origin"
	remoteURL, err := git.GetRemoteURL(bundlePath, "origin")
	if err != nil {
		logger.Debugf("Failed to retrieve remote repository details, skipping Git metadata retrieval: %s", err)
		return nil, ""
	}
	repoURL, err := git.ParseRemoteURL(remoteURL)
	if err != nil {
		logger.Debug("Failed to parse remote repository URL, skipping Git metadata retrieval")
		return nil, ""
	}

	deployGit = &astrov1.CreateDeployGitRequest{}

	// get the path of the bundle within the repository
	path, err := git.GetLocalRepositoryPathPrefix(bundlePath, bundlePath)
	if err != nil {
		logger.Debugf("Failed to retrieve local repository path prefix, skipping Git metadata retrieval: %s", err)
		return nil, ""
	}
	if path != "" {
		deployGit.Path = &path
	}

	// get the branch of the local commit
	branch, err := git.GetBranch(bundlePath)
	if err != nil {
		logger.Debugf("Failed to retrieve branch name, skipping Git metadata retrieval: %s", err)
		return nil, ""
	}
	deployGit.Branch = &branch

	// get the local commit
	sha, message, authorName, _, err := git.GetHeadCommit(bundlePath)
	if err != nil {
		logger.Debugf("Failed to retrieve commit, skipping Git metadata retrieval: %s", err)
		return nil, ""
	}
	deployGit.CommitSha = sha
	if authorName != "" {
		deployGit.AuthorName = &authorName
	}

	// populate provider-specific fields. GitHub gets first-class treatment; everything else is GENERIC.
	if repoURL.Host == "github.com" {
		account, repo, ok := splitGithubPath(repoURL.Path)
		if !ok {
			logger.Debugf("Failed to parse GitHub repository path, skipping Git metadata retrieval: %s", repoURL.Path)
			return nil, ""
		}
		deployGit.Provider = astrov1.CreateDeployGitRequestProviderGITHUB
		deployGit.Account = &account
		deployGit.Repo = &repo
		commitURL := fmt.Sprintf("https://%s/%s/%s/commit/%s", repoURL.Host, account, repo, sha)
		deployGit.CommitUrl = &commitURL
	} else {
		deployGit.Provider = astrov1.CreateDeployGitRequestProviderGENERIC
		deployGit.RemoteUrl = &remoteURL
	}

	logger.Debugf("Retrieved Git metadata: %+v", deployGit)

	return deployGit, message
}

func splitGithubPath(path string) (account, repo string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/")
	slash := strings.Index(trimmed, "/")
	if slash == -1 {
		return "", "", false
	}
	return trimmed[:slash], trimmed[slash+1:], true
}
