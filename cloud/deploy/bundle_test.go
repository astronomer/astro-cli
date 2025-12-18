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
	"github.com/astronomer/astro-cli/config"
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

	// Create a temporary directory for the bundle path
	tmpDir := s.T().TempDir()
	testBundlePath := filepath.Join(tmpDir, "test-bundle-path")
	require.NoError(s.T(), os.Mkdir(testBundlePath, 0o755))

	input := &DeployBundleInput{
		BundlePath:         testBundlePath,
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

func (s *BundleSuite) TestBundleDeploy_GitMetadataDisabledViaConfig() {
	// Create a git repo that would normally have metadata retrieved
	_, gitPath := s.createTestGitRepository(false)
	defer os.RemoveAll(gitPath)

	// Disable git metadata via config
	err := config.CFG.DeployGitMetadata.SetHomeString("false")
	require.NoError(s.T(), err)
	defer config.CFG.DeployGitMetadata.SetHomeString("true") // Reset after test

	input := &DeployBundleInput{
		BundlePath:         gitPath,
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, false)

	// Expect deploy request WITHOUT git metadata (Git: nil)
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

	err = DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockCoreClient.AssertExpectations(s.T())
	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitMetadataDisabledViaEnvVar() {
	// Create a git repo that would normally have metadata retrieved
	_, gitPath := s.createTestGitRepository(false)
	defer os.RemoveAll(gitPath)

	// Disable git metadata via environment variable
	s.T().Setenv("ASTRO_DEPLOY_GIT_METADATA", "false")

	input := &DeployBundleInput{
		BundlePath:         gitPath,
		PlatformCoreClient: s.mockPlatformCoreClient,
		CoreClient:         s.mockCoreClient,
	}

	mockGetDeployment(s.mockPlatformCoreClient, true, false)

	// Expect deploy request WITHOUT git metadata (Git: nil)
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

func (s *BundleSuite) TestValidateBundleSymlinks() {
	t := s.T()

	// Helper function to create a structure
	setup := func(t *testing.T, files map[string]string, links map[string]string) string {
		tmpDir := t.TempDir()
		for name, content := range files {
			filePath := filepath.Join(tmpDir, name)
			require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))
		}
		for linkPath, target := range links {
			fullLinkPath := filepath.Join(tmpDir, linkPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(fullLinkPath), 0o755))
			// Make target absolute if it starts with / for the test setup
			linkTarget := target
			require.NoError(t, os.Symlink(linkTarget, fullLinkPath))
		}
		return tmpDir
	}

	// Case 1: Valid - No symlinks
	t.Run("Valid - No Symlinks", func(t *testing.T) {
		bundlePath := setup(t, map[string]string{"file1.txt": "hello"}, nil)
		err := ValidateBundleSymlinks(bundlePath)
		assert.NoError(t, err)
	})

	// Case 2: Valid - Symlink inside directory
	t.Run("Valid - Symlink Inside", func(t *testing.T) {
		bundlePath := setup(t,
			map[string]string{"file1.txt": "hello"},
			map[string]string{"link1": "file1.txt"},
		)
		err := ValidateBundleSymlinks(bundlePath)
		assert.NoError(t, err)
	})

	// Case 3: Valid - Symlink to subdir
	t.Run("Valid - Symlink To Subdir", func(t *testing.T) {
		bundlePath := setup(t,
			map[string]string{"subdir/file2.txt": "world"},
			map[string]string{"link2": "subdir/file2.txt"},
		)
		err := ValidateBundleSymlinks(bundlePath)
		assert.NoError(t, err)
	})

	// Case 4: Valid - Symlink from subdir back to parent (inside)
	t.Run("Valid - Symlink From Subdir To Parent Inside", func(t *testing.T) {
		bundlePath := setup(t,
			map[string]string{"file1.txt": "hello"},
			map[string]string{"subdir/link3": "../file1.txt"},
		)
		err := ValidateBundleSymlinks(bundlePath)
		assert.NoError(t, err)
	})

	// Case 5: Valid - Broken symlink (relative path would be inside)
	t.Run("Valid - Broken Symlink Inside", func(t *testing.T) {
		bundlePath := setup(t,
			nil,
			map[string]string{"broken_link": "non_existent_file.txt"},
		)
		err := ValidateBundleSymlinks(bundlePath)
		assert.NoError(t, err)
	})

	// Case 6: Invalid - Symlink points relatively outside
	t.Run("Invalid - Relative Symlink Outside", func(t *testing.T) {
		bundlePath := setup(t, nil, map[string]string{"link_outside": "../outside_file"})
		// Create the target file outside for resolution
		outsideFile := filepath.Join(filepath.Dir(bundlePath), "outside_file")
		require.NoError(t, os.WriteFile(outsideFile, []byte("outside"), 0o644))
		defer os.Remove(outsideFile)

		err := ValidateBundleSymlinks(bundlePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "points to")
		assert.Contains(t, err.Error(), "which is outside the bundle directory")
	})

	// Case 7: Invalid - Symlink points absolutely outside
	t.Run("Invalid - Absolute Symlink Outside", func(t *testing.T) {
		// Create a temporary file outside the bundle dir to link to
		outsideDir := t.TempDir()
		outsideFilePath := filepath.Join(outsideDir, "real_outside_file")
		require.NoError(t, os.WriteFile(outsideFilePath, []byte("absolute"), 0o644))

		bundlePath := setup(t, nil, map[string]string{"link_abs_outside": outsideFilePath})
		err := ValidateBundleSymlinks(bundlePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "points to")
		assert.Contains(t, err.Error(), "which is outside the bundle directory")
	})

	// Case 8: Invalid - Symlink from subdir points relatively outside
	t.Run("Invalid - Relative Symlink From Subdir Outside", func(t *testing.T) {
		bundlePath := setup(t, nil, map[string]string{"subdir/link_outside": "../../outside_file"})
		// Create the target file outside for resolution
		outsideFile := filepath.Join(filepath.Dir(bundlePath), "outside_file")
		require.NoError(t, os.WriteFile(outsideFile, []byte("outside"), 0o644))
		defer os.Remove(outsideFile)

		err := ValidateBundleSymlinks(bundlePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "points to")
		assert.Contains(t, err.Error(), "which is outside the bundle directory")
	})
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
