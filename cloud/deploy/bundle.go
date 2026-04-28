package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/logger"
)

type DeployBundleInput struct {
	BundlePath    string
	MountPath     string
	DeploymentID  string
	BundleType    string
	Description   string
	Wait          bool
	WaitTime      time.Duration
	AstroV1Client astrov1.APIClient
}

func DeployBundle(input *DeployBundleInput) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	// get the current deployment so we can check the deploy is valid
	currentDeployment, err := deployment.GetDeploymentByID(c.Organization, input.DeploymentID, input.AstroV1Client)
	if err != nil {
		return err
	}

	// if CI/CD is enforced, check the subject can deploy
	if currentDeployment.IsCicdEnforced && !canCiCdDeploy(c.Token) {
		return fmt.Errorf(errCiCdEnforcementUpdate, currentDeployment.Name)
	}

	// check the deployment is enabled for DAG deploys
	if !currentDeployment.IsDagDeployEnabled {
		return fmt.Errorf(enableDagDeployMsg, input.DeploymentID)
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
		return err
	}

	// check we received an upload URL
	if deploy.BundleUploadUrl == nil {
		return errors.New("no bundle upload URL received from Astro")
	}

	// upload the bundle
	tarballVersion, err := UploadBundle(config.WorkingPath, input.BundlePath, *deploy.BundleUploadUrl, false, currentDeployment.RuntimeVersion)
	if err != nil {
		return err
	}

	// finalize the deploy
	err = finalizeBundleDeploy(c.Organization, input.DeploymentID, deploy.Id, tarballVersion, input.AstroV1Client)
	if err != nil {
		return err
	}
	fmt.Println("Successfully uploaded bundle with version " + tarballVersion + " to Astro.")

	// if requested, wait for the deploy to finish by polling the deployment until it is healthy
	if input.Wait {
		err = deployment.HealthPoll(currentDeployment.Id, currentDeployment.WorkspaceId, dagOnlyDeploySleepTime, tickNum, int(input.WaitTime.Seconds()), input.AstroV1Client)
		if err != nil {
			return err
		}
	}

	return nil
}

type DeleteBundleInput struct {
	MountPath     string
	DeploymentID  string
	WorkspaceID   string
	BundleType    string
	Description   string
	Wait          bool
	WaitTime      time.Duration
	AstroV1Client astrov1.APIClient
}

func DeleteBundle(input *DeleteBundleInput) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
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
		return err
	}

	// immediately finalize with no version, which will delete the bundle from the deployment
	err = finalizeBundleDeploy(c.Organization, input.DeploymentID, deploy.Id, "", input.AstroV1Client)
	if err != nil {
		return err
	}
	fmt.Println("Successfully requested bundle delete for mount path " + input.MountPath + " from Astro.")

	// if requested, wait for the deploy to finish by polling the deployment until it is healthy
	if input.Wait {
		err = deployment.HealthPoll(input.DeploymentID, input.WorkspaceID, dagOnlyDeploySleepTime, tickNum, int(input.WaitTime.Seconds()), input.AstroV1Client)
		if err != nil {
			return err
		}
	}

	return nil
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

	// get the raw remote URL (needed for the GENERIC provider), assume the remote is named "origin"
	remoteURL, err := git.GetRemoteURL(bundlePath, "origin")
	if err != nil {
		logger.Debugf("Failed to retrieve remote repository details, skipping Git metadata retrieval: %s", err)
		return nil, ""
	}
	repoURL, err := git.ParseRemoteURL(remoteURL)
	if err != nil {
		logger.Debugf("Failed to parse remote repository URL, skipping Git metadata retrieval: %s", err)
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
