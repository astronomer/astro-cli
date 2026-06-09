package deploy

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/git"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type BundleSuite struct {
	suite.Suite
	mockV1Client *astrov1_mocks.ClientWithResponsesInterface
}

func (s *BundleSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
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
		BundlePath:    testBundlePath,
		MountPath:     "test-mount-path",
		DeploymentID:  "test-deployment-id",
		BundleType:    "test-bundle-type",
		Description:   "test-description",
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, true)

	expectedDeploy := &astrov1.CreateDeployRequest{
		Type:            astrov1.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
	}
	mockCreateDeploy(s.mockV1Client, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(s.mockV1Client, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_CiCdIncompatible() {
	canCiCdDeploy = func(token string) bool {
		return false
	}

	input := &DeployBundleInput{
		DeploymentID:  "test-deployment-id",
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, true)

	err := DeployBundle(input)
	assert.Error(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_DagDeployDisabled() {
	input := &DeployBundleInput{
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, false, false)

	err := DeployBundle(input)
	assert.Error(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitMetadataRetrieved() {
	gitPath := s.createTestGitRepository(false)
	defer os.RemoveAll(gitPath)

	input := &DeployBundleInput{
		BundlePath:    gitPath,
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, false)

	expectedDescription := strings.Repeat("a", git.MaxCommitMessageLineLength)
	mockCreateDeployMatch(s.mockV1Client, "http://bundle-upload-url", func(req astrov1.CreateDeployRequest) bool {
		if req.Type != astrov1.CreateDeployRequestTypeBUNDLE {
			return false
		}
		if req.Description == nil || *req.Description != expectedDescription {
			return false
		}
		if req.Git == nil {
			return false
		}
		g := req.Git
		return g.Provider == astrov1.CreateDeployGitRequestProviderGITHUB &&
			g.Account != nil && *g.Account == "account" &&
			g.Repo != nil && *g.Repo == "repo" &&
			g.Branch != nil && *g.Branch == "main" &&
			g.CommitSha != "" &&
			g.CommitUrl != nil && strings.HasPrefix(*g.CommitUrl, "https://github.com/account/repo/commit/") &&
			g.AuthorName != nil && *g.AuthorName == "Test" &&
			g.RemoteUrl == nil
	})
	mockUpdateDeploy(s.mockV1Client, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitMetadataGenericProvider() {
	gitPath := s.createTestGitRepositoryWithRemote(false, "git@gitlab.com:team/project.git")
	defer os.RemoveAll(gitPath)

	input := &DeployBundleInput{
		BundlePath:    gitPath,
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, false)

	mockCreateDeployMatch(s.mockV1Client, "http://bundle-upload-url", func(req astrov1.CreateDeployRequest) bool {
		if req.Git == nil {
			return false
		}
		g := req.Git
		return g.Provider == astrov1.CreateDeployGitRequestProviderGENERIC &&
			g.RemoteUrl != nil && *g.RemoteUrl == "git@gitlab.com:team/project.git" &&
			g.Account == nil &&
			g.Repo == nil &&
			g.CommitUrl == nil &&
			g.Branch != nil && *g.Branch == "main" &&
			g.CommitSha != ""
	})
	mockUpdateDeploy(s.mockV1Client, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitHasUncommittedChanges() {
	gitPath := s.createTestGitRepository(true)
	defer os.RemoveAll(gitPath)

	input := &DeployBundleInput{
		BundlePath:    gitPath,
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, false)

	expectedDeploy := &astrov1.CreateDeployRequest{
		Type:            astrov1.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
	}
	mockCreateDeploy(s.mockV1Client, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(s.mockV1Client, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_GitMetadataDisabledViaConfig() {
	// Create a git repo that would normally have metadata retrieved
	gitPath := s.createTestGitRepository(false)
	defer os.RemoveAll(gitPath)

	// Disable git metadata via config
	err := config.CFG.DeployGitMetadata.SetHomeString("false")
	require.NoError(s.T(), err)
	defer config.CFG.DeployGitMetadata.SetHomeString("true") // Reset after test

	input := &DeployBundleInput{
		BundlePath:    gitPath,
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, false)

	// Expect deploy request WITHOUT git metadata
	expectedDeploy := &astrov1.CreateDeployRequest{
		Type:            astrov1.CreateDeployRequestTypeBUNDLE,
		BundleType:      &input.BundleType,
		BundleMountPath: &input.MountPath,
		Description:     &input.Description,
	}
	mockCreateDeploy(s.mockV1Client, "http://bundle-upload-url", expectedDeploy)
	mockUpdateDeploy(s.mockV1Client, "version-id")

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err = DeployBundle(input)
	assert.NoError(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDeploy_BundleUploadUrlMissing() {
	input := &DeployBundleInput{
		AstroV1Client: s.mockV1Client,
	}

	mockGetDeployment(s.mockV1Client, true, false)

	mockCreateDeploy(s.mockV1Client, "", nil)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(input)
	assert.Error(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
}

func (s *BundleSuite) TestBundleDelete_Success() {
	input := &DeleteBundleInput{
		DeploymentID:  "test-deployment-id",
		MountPath:     "test-mount-path",
		AstroV1Client: s.mockV1Client,
	}

	mockCreateDeploy(s.mockV1Client, "", nil)
	mockUpdateDeploy(s.mockV1Client, "")

	err := DeleteBundle(input)
	assert.NoError(s.T(), err)

	s.mockV1Client.AssertExpectations(s.T())
	s.mockV1Client.AssertExpectations(s.T())
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

func mockCreateDeploy(client *astrov1_mocks.ClientWithResponsesInterface, bundleUploadURL string, expectedDeploy *astrov1.CreateDeployRequest) {
	var request any
	if expectedDeploy != nil {
		request = *expectedDeploy
	} else {
		request = mock.Anything
	}
	response := &astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
		JSON200: &astrov1.Deploy{
			Id: "test-deploy-id",
		},
	}
	if bundleUploadURL != "" {
		response.JSON200.BundleUploadUrl = &bundleUploadURL
	}
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, request).Return(response, nil)
}

func mockCreateDeployMatch(client *astrov1_mocks.ClientWithResponsesInterface, bundleUploadURL string, match func(astrov1.CreateDeployRequest) bool) {
	response := &astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
		JSON200: &astrov1.Deploy{
			Id: "test-deploy-id",
		},
	}
	if bundleUploadURL != "" {
		response.JSON200.BundleUploadUrl = &bundleUploadURL
	}
	matcher := mock.MatchedBy(func(req astrov1.CreateDeployRequest) bool { return match(req) })
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, matcher).Return(response, nil)
}

func mockUpdateDeploy(client *astrov1_mocks.ClientWithResponsesInterface, expectedBundleVersion string) {
	request := astrov1.FinalizeDeployRequest{
		BundleTarballVersion: &expectedBundleVersion,
	}
	client.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, request).Return(&astrov1.FinalizeDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
	}, nil)
}

func mockGetDeployment(client *astrov1_mocks.ClientWithResponsesInterface, isDagDeployEnabled, isCicdEnforced bool) {
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
		JSON200: &astrov1.Deployment{
			Id:                 "test-deployment-id",
			IsDagDeployEnabled: isDagDeployEnabled,
			IsCicdEnforced:     isCicdEnforced,
		},
	}, nil)
}

func (s *BundleSuite) createTestGitRepository(withUncommittedFile bool) string {
	return s.createTestGitRepositoryWithRemote(withUncommittedFile, "https://github.com/account/repo.git")
}

func (s *BundleSuite) createTestGitRepositoryWithRemote(withUncommittedFile bool, remoteURL string) string {
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

	err = exec.Command("git", "-C", dir, "remote", "add", "origin", remoteURL).Run()
	require.NoError(s.T(), err)

	if withUncommittedFile {
		err = os.WriteFile(filepath.Join(dir, "uncommitted-file"), []byte("uncommitted"), 0o644)
		require.NoError(s.T(), err)

		err = exec.Command("git", "-C", dir, "add", "uncommitted-file").Run()
		require.NoError(s.T(), err)
	}

	return dir
}
