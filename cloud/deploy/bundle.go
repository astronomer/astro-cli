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
	"github.com/sirupsen/logrus"
)

type DeployBundleInput struct {
	BundlePath   string
	MountPath    string
	DeploymentID string
	BundleType   string
	Description  string
	Wait         bool
}

func DeployBundle(input *DeployBundleInput, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	// get the current deployment so we can check the deploy is valid
	currentDeployment, err := deployment.CoreGetDeployment(c.Organization, input.DeploymentID, platformCoreClient)
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
	gitMetadata := retrieveLocalGitMetadata(input.BundlePath)

	// initialize the deploy
	deploy, err := createBundleDeploy(c.Organization, input, gitMetadata, coreClient)
	if err != nil {
		return err
	}

	// check we received an upload URL
	if deploy.BundleUploadUrl == nil {
		return errors.New("no bundle upload URL received from Astro")
	}

	// upload the bundle
	tarballVersion, err := uploadBundle(config.WorkingPath, input.BundlePath, *deploy.BundleUploadUrl, false)
	if err != nil {
		return err
	}

	// finalize the deploy
	_, err = finalizeBundleDeploy(c.Organization, deploy.Id, tarballVersion, input, coreClient)
	if err != nil {
		return err
	}
	fmt.Println("Successfully uploaded bundle with version " + tarballVersion + " to Astro.")

	// if requested, wait for the deploy to finish by polling the deployment until it is healthy
	if input.Wait {
		err = deployment.HealthPoll(currentDeployment.Id, currentDeployment.WorkspaceId, dagOnlyDeploySleepTime, tickNum, timeoutNum, platformCoreClient)
		if err != nil {
			return err
		}
	}

	return nil
}

func uploadBundle(tarDirPath, bundlePath, uploadURL string, prependBaseDir bool) (string, error) {
	tarFilePath := filepath.Join(tarDirPath, "bundle.tar")
	defer func() {
		err := os.Remove(tarFilePath)
		if err != nil {
			fmt.Println("\nFailed to delete tar file: ", err.Error())
			fmt.Println("\nPlease delete the tar file manually from path: " + tarFilePath)
		}
	}()

	// Generate the bundle tar
	err := fileutil.Tar(bundlePath, tarFilePath, prependBaseDir)
	if err != nil {
		return "", err
	}

	tarFile, err := os.Open(tarFilePath)
	if err != nil {
		return "", err
	}
	defer tarFile.Close()

	versionID, err := azureUploader(uploadURL, tarFile)
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

func finalizeBundleDeploy(organizationID, deployID, tarballVersion string, input *DeployBundleInput, coreClient astrocore.CoreClient) (*astrocore.UpdateDeployResponse, error) {
	request := astrocore.UpdateDeployRequest{
		BundleTarballVersion: &tarballVersion,
	}
	resp, err := coreClient.UpdateDeployWithResponse(context.Background(), organizationID, input.DeploymentID, deployID, request)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func retrieveLocalGitMetadata(bundlePath string) *astrocore.DeployGit {
	if git.HasUncommittedChanges(bundlePath) {
		logrus.Warn("Local repository has uncommitted changes, skipping Git metadata retrieval")
		return nil
	}

	gitMetadata := &astrocore.DeployGit{}

	// get the remote repository details, assume the remote is named "origin"
	repoURL, err := git.GetRemoteRepository(bundlePath, "origin")
	if err != nil {
		logrus.Debugf("Failed to retrieve remote repository details, skipping Git metadata retrieval: %s", err)
		return nil
	}
	switch repoURL.Host {
	case "github.com":
		gitMetadata.Provider = astrocore.DeployGitProviderGITHUB
	default:
		logrus.Debugf("Unsupported Git provider, skipping Git metadata retrieval: %s", repoURL.Host)
		return nil
	}
	urlPath := strings.TrimPrefix(repoURL.Path, "/")
	firstSlashIndex := strings.Index(urlPath, "/")
	if firstSlashIndex == -1 {
		logrus.Debugf("Failed to parse remote repository path, skipping Git metadata retrieval: %s", repoURL.Path)
		return nil
	}
	gitMetadata.Account = urlPath[:firstSlashIndex]
	gitMetadata.Repo = urlPath[firstSlashIndex+1:]

	// get the path of the bundle within the repository
	path, err := git.GetLocalRepositoryPathPrefix(bundlePath, bundlePath)
	if err != nil {
		logrus.Debugf("Failed to retrieve local repository path prefix, skipping Git metadata retrieval: %s", err)
		return nil
	}
	if path != "" {
		gitMetadata.Path = &path
	}

	// get the branch of the local commit
	branch, err := git.GetBranch(bundlePath)
	if err != nil {
		logrus.Debugf("Failed to retrieve branch name, skipping Git metadata retrieval: %s", err)
		return nil
	}
	gitMetadata.Branch = branch

	// get the SHA of the local commit
	sha, err := git.GetHeadCommitSHA(bundlePath)
	if err != nil {
		logrus.Debugf("Failed to retrieve commit SHA, skipping Git metadata retrieval: %s", err)
		return nil
	}
	gitMetadata.CommitSha = sha

	// derive the remote URL of the local commit
	switch repoURL.Host {
	case "github.com":
		gitMetadata.CommitUrl = fmt.Sprintf("https://%s/%s/%s/commit/%s", repoURL.Host, gitMetadata.Account, gitMetadata.Repo, sha)
	default:
		logrus.Debugf("Unsupported Git provider, skipping Git metadata retrieval: %s", repoURL.Host)
		return nil
	}

	// get the author name of the local commit
	authorName, _, err := git.GetHeadCommitAuthor(bundlePath)
	if err != nil {
		logrus.Debugf("Failed to retrieve commit author, skipping Git metadata retrieval: %s", err)
		return nil
	}
	if authorName != "" {
		gitMetadata.AuthorName = &authorName
	}

	logrus.Debugf("Retrieved Git metadata: %+v", gitMetadata)

	return gitMetadata
}
