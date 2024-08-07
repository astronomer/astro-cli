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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BundleSuite struct {
	suite.Suite
	mockPlatformCoreClient *astroplatformcore_mocks.ClientWithResponsesInterface
	mockCoreClient         *astrocore_mocks.ClientWithResponsesInterface
}

func (s *BundleSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	s.mockCoreClient = new(astrocore_mocks.ClientWithResponsesInterface)
}

func TestBundles(t *testing.T) {
	suite.Run(t, new(BundleSuite))
}

func (s *BundleSuite) TestBundleDeploy_Success() {
	canCiCdDeploy = func(token string) bool {
		return true
	}

	input := &DeployBundleInput{
		BundlePath:         "test-bundle-path",
		MountPath:          "test-mount-path",
		DeploymentID:       "test-deployment-id",
		BundleType:         "test-bundle-type",
		Description:        "test-description",
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, true)

	expectedDeploy := &astrocore.CreateDeployRequest{
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
	}
	mockCreateDeploy(s.mockCoreClient, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(s.mockCoreClient, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_CiCdIncompatible() {
	canCiCdDeploy = func(token string) bool {
		return false
	}

	input := &DeployBundleInput{
		DeploymentID:       "test-deployment-id",
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, true)

	err := DeployBundle(input)
	assert.Error(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_DagDeployDisabled() {
	input := &DeployBundleInput{
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, false, false)

	err := DeployBundle(input)
	assert.Error(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitMetadataRetrieved() {
	sha, gitPath := s.createTestGitRepository(false)
	defer os.RemoveAll(gitPath)

	input := &DeployBundleInput{
		BundlePath:         gitPath,
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, false)

	expectedAuthorName := "Test"
	expectedDescription := strings.Repeat("a", git.MaxCommitMessageLineLength)
	expectedDeploy := &astrocore.CreateDeployRequest{
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &expectedDescription,
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
	mockCreateDeploy(s.mockCoreClient, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(s.mockCoreClient, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitHasUncommittedChanges() {
	_, gitPath := s.createTestGitRepository(true)
	defer os.RemoveAll(gitPath)

	input := &DeployBundleInput{
		BundlePath:         gitPath,
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, false)

	expectedDeploy := &astrocore.CreateDeployRequest{
		Type:            astrocore.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
		Git:             nil,
	}
	mockCreateDeploy(s.mockCoreClient, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(s.mockCoreClient, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_BundleUploadUrlMissing() {
	input := &DeployBundleInput{
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, false)

	mockCreateDeploy(s.mockCoreClient, "", nil)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.Error(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDelete_Success() {
	input := &DeleteBundleInput{
		DeploymentID:       "test-deployment-id",
		MountPath:          "test-mount-path",
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockCreateDeploy(s.mockCoreClient, "", nil)
	mockUpdateDeploy(s.mockCoreClient, "")

	err := DeleteBundle(input)
	assert.NoError(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
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

func mockUpdateDeploy(client *astrocore_mocks.ClientWithResponsesInterface, expectedBundleVersion string) {
	request := astrocore.UpdateDeployRequest{
		BundleTarballVersion: &expectedBundleVersion,
	}
	client.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, request).Return(&astrocore.UpdateDeployResponse{
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

func (s *BundleSuite) createTestGitRepository(withUncommittedFile bool) (sha, path string) {
	dir, err := os.MkdirTemp("", "test-git-repo")
	require.NoError(s.T(), err)

	err = exec.Command("git", "-C", dir, "init", "-b", "main").Run()
	require.NoError(s.T(), err)

	err = exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	require.NoError(s.T(), err)

	err = exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	require.NoError(s.T(), err)

	commitMessage := strings.Repeat("a", git.MaxCommitMessageLineLength+1) + "\n" + strings.Repeat("b", git.MaxCommitMessageLineLength+1)
	err = exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", commitMessage).Run()
	require.NoError(s.T(), err)

	err = exec.Command("git", "-C", dir, "remote", "add", "origin", "https://github.com/account/repo.git").Run()
	require.NoError(s.T(), err)

	shaBytes, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	require.NoError(s.T(), err)

	if withUncommittedFile {
		err = os.WriteFile(filepath.Join(dir, "uncommitted-file"), []byte("uncommitted"), 0o644)
		require.NoError(s.T(), err)

		err = exec.Command("git", "-C", dir, "add", "uncommitted-file").Run()
		require.NoError(s.T(), err)
	}

	return strings.TrimSpace(string(shaBytes)), dir
}
