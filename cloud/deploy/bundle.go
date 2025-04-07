package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/logger"
)

type DeployBundleInput struct {
	BundlePath         string
	MountPath          string
	DeploymentID       string
	BundleType         string
	Description        string
	Wait               bool
	PlatformCoreClient astroplatformcore.CoreClient
	CoreClient         astrocore.CoreClient
}

func DeployBundle(input *DeployBundleInput) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	// get the current deployment so we can check the deploy is valid
	currentDeployment, err := deployment.CoreGetDeployment(c.Organization, input.DeploymentID, input.PlatformCoreClient)
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

	// retrieve metadata about the local Git checkout. returns nil if not available
	deployGit, commitMessage := retrieveLocalGitMetadata(input.BundlePath)

	// if no description was provided, use the commit message from the local Git checkout
	if input.Description == "" {
		input.Description = commitMessage
	}

	// initialize the deploy
	deploy, err := createBundleDeploy(c.Organization, input, deployGit, input.CoreClient)
	if err != nil {
		return err
	}

	// check we received an upload URL
	if deploy.BundleUploadUrl == nil {
		return errors.New("no bundle upload URL received from Astro")
	}

	// upload the bundle
	tarballVersion, err := UploadBundle(config.WorkingPath, input.BundlePath, *deploy.BundleUploadUrl, false)
	if err != nil {
		return err
	}

	// finalize the deploy
	err = finalizeBundleDeploy(c.Organization, input.DeploymentID, deploy.Id, tarballVersion, input.CoreClient)
	if err != nil {
		return err
	}
	fmt.Println("Successfully uploaded bundle with version " + tarballVersion + " to Astro.")

	// if requested, wait for the deploy to finish by polling the deployment until it is healthy
	if input.Wait {
		err = deployment.HealthPoll(currentDeployment.Id, currentDeployment.WorkspaceId, dagOnlyDeploySleepTime, tickNum, timeoutNum, input.PlatformCoreClient)
		if err != nil {
			return err
		}
	}

	return nil
}

type DeleteBundleInput struct {
	MountPath          string
	DeploymentID       string
	WorkspaceID        string
	BundleType         string
	Description        string
	Wait               bool
	CoreClient         astrocore.CoreClient
	PlatformCoreClient astroplatformcore.CoreClient
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
	deploy, err := createBundleDeploy(c.Organization, createInput, nil, input.CoreClient)
	if err != nil {
		return err
	}

	// immediately finalize with no version, which will delete the bundle from the deployment
	err = finalizeBundleDeploy(c.Organization, input.DeploymentID, deploy.Id, "", input.CoreClient)
	if err != nil {
		return err
	}
	fmt.Println("Successfully requested bundle delete for mount path " + input.MountPath + " from Astro.")

	// if requested, wait for the deploy to finish by polling the deployment until it is healthy
	if input.Wait {
		err = deployment.HealthPoll(input.DeploymentID, input.WorkspaceID, dagOnlyDeploySleepTime, tickNum, timeoutNum, input.PlatformCoreClient)
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

func UploadBundle(tarDirPath, bundlePath, uploadURL string, prependBaseDir bool) (string, error) {
	// Check for symlinks pointing outside the bundle directory
	err := ValidateBundleSymlinks(bundlePath)
	if err != nil {
		return "", err
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
	err = fileutil.Tar(bundlePath, tarFilePath, prependBaseDir, []string{".git/"})
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

func createBundleDeploy(organizationID string, input *DeployBundleInput, deployGit *astrocore.DeployGit, coreClient astrocore.CoreClient) (*astrocore.Deploy, error) {
	request := astrocore.CreateDeployRequest{
		Description:     &input.Description,
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleMountPath: &input.MountPath,
		BundleType:      &input.BundleType,
	}
	if deployGit != nil {
		request.Git = &astrocore.CreateDeployGitRequest{
			Provider:   astrocore.CreateDeployGitRequestProvider(deployGit.Provider),
			Repo:       deployGit.Repo,
			Account:    deployGit.Account,
			Path:       deployGit.Path,
			Branch:     deployGit.Branch,
			CommitSha:  deployGit.CommitSha,
			CommitUrl:  fmt.Sprintf("https://github.com/%s/%s/commit/%s", deployGit.Account, deployGit.Repo, deployGit.CommitSha),
			AuthorName: deployGit.AuthorName,
		}
	}
	resp, err := coreClient.CreateDeployWithResponse(context.Background(), organizationID, input.DeploymentID, request)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func finalizeBundleDeploy(organizationID, deploymentID, deployID, tarballVersion string, coreClient astrocore.CoreClient) error {
	request := astrocore.UpdateDeployRequest{
		BundleTarballVersion: &tarballVersion,
	}
	resp, err := coreClient.UpdateDeployWithResponse(context.Background(), organizationID, deploymentID, deployID, request)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

func retrieveLocalGitMetadata(bundlePath string) (deployGit *astrocore.DeployGit, commitMessage string) {
	if git.HasUncommittedChanges(bundlePath) {
		fmt.Println("Local repository has uncommitted changes, skipping Git metadata retrieval")
		return nil, ""
	}

	deployGit = &astrocore.DeployGit{}

	// get the remote repository details, assume the remote is named "origin"
	repoURL, err := git.GetRemoteRepository(bundlePath, "origin")
	if err != nil {
		logger.Debugf("Failed to retrieve remote repository details, skipping Git metadata retrieval: %s", err)
		return nil, ""
	}
	switch repoURL.Host {
	case "github.com":
		deployGit.Provider = astrocore.DeployGitProviderGITHUB
	default:
		logger.Debugf("Unsupported Git provider, skipping Git metadata retrieval: %s", repoURL.Host)
		return nil, ""
	}
	urlPath := strings.TrimPrefix(repoURL.Path, "/")
	firstSlashIndex := strings.Index(urlPath, "/")
	if firstSlashIndex == -1 {
		logger.Debugf("Failed to parse remote repository path, skipping Git metadata retrieval: %s", repoURL.Path)
		return nil, ""
	}
	deployGit.Account = urlPath[:firstSlashIndex]
	deployGit.Repo = urlPath[firstSlashIndex+1:]

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
	deployGit.Branch = branch

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

	// derive the remote URL of the local commit
	switch repoURL.Host {
	case "github.com":
		deployGit.CommitUrl = fmt.Sprintf("https://%s/%s/%s/commit/%s", repoURL.Host, deployGit.Account, deployGit.Repo, sha)
	default:
		logger.Debugf("Unsupported Git provider, skipping Git metadata retrieval: %s", repoURL.Host)
		return nil, ""
	}

	logger.Debugf("Retrieved Git metadata: %+v", deployGit)

	return deployGit, message
}
