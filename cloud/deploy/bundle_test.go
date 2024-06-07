package deploy

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/pkg/git"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBundleDeploy_Success(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	canCiCdDeploy = func(token string) bool {
		return true
	}

	input := &DeployBundleInput{
		BundlePath:   "test-bundle-path",
		MountPath:    "test-mount-path",
		DeploymentID: "test-deployment-id",
		BundleType:   "test-bundle-type",
		Description:  "test-description",
	}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockPlatformCoreClient, true, true)

	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	expectedDeploy := &astrocore.CreateDeployRequest{
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
	}
	mockCreateDeploy(mockCoreClient, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(mockCoreClient)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestBundleDeploy_CiCdIncompatible(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	canCiCdDeploy = func(token string) bool {
		return false
	}

	input := &DeployBundleInput{
		DeploymentID: "test-deployment-id",
	}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockPlatformCoreClient, true, true)

	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	err := DeployBundle(input, mockPlatformCoreClient, mockCoreClient)
	assert.Error(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestBundleDeploy_DagDeployDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	input := &DeployBundleInput{}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockPlatformCoreClient, false, false)

	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	err := DeployBundle(input, mockPlatformCoreClient, mockCoreClient)
	assert.Error(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestBundleDeploy_GitMetadataRetrieved(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	sha, cleanupGit := createTestGitRepository(t, false)
	defer cleanupGit()

	input := &DeployBundleInput{}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockPlatformCoreClient, true, false)

	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	expectedAuthorName := "Test"
	expectedDeploy := &astrocore.CreateDeployRequest{
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
		Git: &astrocore.CreateDeployGitRequest{
			Provider:   astrocore.CreateDeployGitRequestProviderGITHUB,
			Account:    "account",
			Repo:       "repo",
			Branch:     "main",
			CommitSha:  sha,
			CommitUrl:  "https://github.com/account/repo/commit/" + sha,
			AuthorName: &expectedAuthorName,
		},
	}
	mockCreateDeploy(mockCoreClient, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(mockCoreClient)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestBundleDeploy_GitHasUncommittedChanges(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	_, cleanupGit := createTestGitRepository(t, true)
	defer cleanupGit()

	input := &DeployBundleInput{}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockPlatformCoreClient, true, false)

	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	expectedDeploy := &astrocore.CreateDeployRequest{
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
		Git:             nil,
	}
	mockCreateDeploy(mockCoreClient, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(mockCoreClient)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestBundleDeploy_BundleUploadUrlMissing(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	input := &DeployBundleInput{}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockPlatformCoreClient, true, false)

	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockCreateDeploy(mockCoreClient, "", nil)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input, mockPlatformCoreClient, mockCoreClient)
	assert.Error(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func mockCreateDeploy(client *astrocore_mocks.ClientWithResponsesInterface, bundleUploadURL string, expectedDeploy *astrocore.CreateDeployRequest) {
	var request any
	if expectedDeploy != nil {
		request = *expectedDeploy
	} else {
		request = mock.Anything
	}
	response := &astrocore.CreateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astrocore.Deploy{
			Id: "test-deploy-id",
		},
	}
	if bundleUploadURL != "" {
		response.JSON200.BundleUploadUrl = &bundleUploadURL
	}
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, request).Return(response, nil)
}

func mockUpdateDeploy(client *astrocore_mocks.ClientWithResponsesInterface) {
	client.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrocore.UpdateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
	}, nil)
}

func mockGetDeployment(client *astroplatformcore_mocks.ClientWithResponsesInterface, isDagDeployEnabled, isCicdEnforced bool) {
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                 "test-deployment-id",
			IsDagDeployEnabled: isDagDeployEnabled,
			IsCicdEnforced:     isCicdEnforced,
		},
	}, nil)
}

func createTestGitRepository(t *testing.T, withUncommittedFile bool) (sha string, cleanup func()) {
	dir, err := os.MkdirTemp("", "test-git-repo")
	require.NoError(t, err)
	git.Path = dir
	logrus.Warnf("git.Path: %s", git.Path)

	err = exec.Command("git", "-C", dir, "init", "-b", "main").Run()
	require.NoError(t, err)

	err = exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	require.NoError(t, err)

	err = exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	require.NoError(t, err)

	err = exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "Initial commit").Run()
	require.NoError(t, err)

	err = exec.Command("git", "-C", dir, "remote", "add", "origin", "https://github.com/account/repo.git").Run()
	require.NoError(t, err)

	shaBytes, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)

	if withUncommittedFile {
		err = os.WriteFile(filepath.Join(dir, "uncommitted-file"), []byte("uncommitted"), 0o644)
		require.NoError(t, err)

		err = exec.Command("git", "-C", dir, "add", "uncommitted-file").Run()
		require.NoError(t, err)
	}

	return strings.TrimSpace(string(shaBytes)), func() {
		os.RemoveAll(dir)
		git.Path = ""
	}
}
