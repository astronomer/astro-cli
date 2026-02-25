package cloud

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type BundleCmdSuite struct {
	suite.Suite
	mockPlatformCoreClient *astroplatformcore_mocks.ClientWithResponsesInterface
	mockCoreClient         *astrocore_mocks.ClientWithResponsesInterface
	mockStreamClient       *astrocore_mocks.BundleFilesStreamClient
	origPlatformCoreClient astroplatformcore.CoreClient
	origCoreClient         astrocore.CoreClient
	origStreamClient       astrocore.BundleFilesStreamClient
	origBundleListFiles    func(out io.Writer, coreClient astrocore.CoreClient, orgID, deploymentID, path string) error
	origBundleUploadFile   func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, remotePath, localPath string) error
	origBundleDownloadFile func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, remotePath, outputPath string, out io.Writer) error
	origBundleDeleteFile   func(coreClient astrocore.CoreClient, orgID, deploymentID, path string) error
	origBundleMoveFile     func(coreClient astrocore.CoreClient, orgID, deploymentID, sourcePath, destination string) error
	origBundleDupFile      func(coreClient astrocore.CoreClient, orgID, deploymentID, sourcePath, destination string) error
	origBundleSync         func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, localDir, targetPath string, overwrite bool) error
	origBundleUploadArch   func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, archivePath, targetPath string, overwrite bool) error
	origBundleDownloadArch func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, outputPath string, out io.Writer) error
}

func (s *BundleCmdSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.origPlatformCoreClient = platformCoreClient
	s.origCoreClient = astroCoreClient
	s.origStreamClient = bundleStreamClient
	s.origBundleListFiles = BundleListFiles
	s.origBundleUploadFile = BundleUploadFile
	s.origBundleDownloadFile = BundleDownloadFile
	s.origBundleDeleteFile = BundleDeleteFile
	s.origBundleMoveFile = BundleMoveFile
	s.origBundleDupFile = BundleDuplicateFile
	s.origBundleSync = BundleSync
	s.origBundleUploadArch = BundleUploadArchive
	s.origBundleDownloadArch = BundleDownloadArch

	s.mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	s.mockCoreClient = new(astrocore_mocks.ClientWithResponsesInterface)
	s.mockStreamClient = new(astrocore_mocks.BundleFilesStreamClient)
	platformCoreClient = s.mockPlatformCoreClient
	astroCoreClient = s.mockCoreClient
	bundleStreamClient = s.mockStreamClient
}

func (s *BundleCmdSuite) TearDownTest() {
	s.mockPlatformCoreClient.AssertExpectations(s.T())
	s.mockCoreClient.AssertExpectations(s.T())
	s.mockStreamClient.AssertExpectations(s.T())

	platformCoreClient = s.origPlatformCoreClient
	astroCoreClient = s.origCoreClient
	bundleStreamClient = s.origStreamClient
	BundleListFiles = s.origBundleListFiles
	BundleUploadFile = s.origBundleUploadFile
	BundleDownloadFile = s.origBundleDownloadFile
	BundleDeleteFile = s.origBundleDeleteFile
	BundleMoveFile = s.origBundleMoveFile
	BundleDuplicateFile = s.origBundleDupFile
	BundleSync = s.origBundleSync
	BundleUploadArchive = s.origBundleUploadArch
	BundleDownloadArch = s.origBundleDownloadArch
}

func TestBundleCmd(t *testing.T) {
	suite.Run(t, new(BundleCmdSuite))
}

func execBundleCmd(cmd *cobra.Command, args ...string) error {
	if args == nil {
		args = []string{}
	}
	testUtil.SetupOSArgsForGinkgo()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

// --- bundle file list ---

func (s *BundleCmdSuite) TestBundleFileList_WithDeploymentID() {
	BundleListFiles = func(out io.Writer, coreClient astrocore.CoreClient, orgID, deploymentID, path string) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		return nil
	}

	err := execBundleCmd(newBundleFileListCmd(os.Stdout), "test-deploy-id")
	assert.NoError(s.T(), err)
}

func (s *BundleCmdSuite) TestBundleFileList_PickDeployment() {
	BundleListFiles = func(out io.Writer, coreClient astrocore.CoreClient, orgID, deploymentID, path string) error {
		assert.Equal(s.T(), "test-deployment-id", deploymentID)
		return nil
	}

	s.mockListTestDeployments()
	s.mockGetTestDeployment()

	defer testUtil.MockUserInput(s.T(), "1")()
	err := execBundleCmd(newBundleFileListCmd(os.Stdout))
	assert.NoError(s.T(), err)
}

// --- bundle file delete ---

func (s *BundleCmdSuite) TestBundleFileDelete_WithDeploymentID() {
	BundleDeleteFile = func(coreClient astrocore.CoreClient, orgID, deploymentID, path string) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), "dags/test.py", path)
		return nil
	}

	err := execBundleCmd(newBundleFileDeleteCmd(os.Stdout), "test-deploy-id", "dags/test.py")
	assert.NoError(s.T(), err)
}

func (s *BundleCmdSuite) TestBundleFileDelete_NoDeploymentID() {
	BundleDeleteFile = func(coreClient astrocore.CoreClient, orgID, deploymentID, path string) error {
		assert.Equal(s.T(), "test-deployment-id", deploymentID)
		assert.Equal(s.T(), "dags/test.py", path)
		return nil
	}

	s.mockListTestDeployments()
	s.mockGetTestDeployment()

	defer testUtil.MockUserInput(s.T(), "1")()
	err := execBundleCmd(newBundleFileDeleteCmd(os.Stdout), "dags/test.py")
	assert.NoError(s.T(), err)
}

// --- bundle file move ---

func (s *BundleCmdSuite) TestBundleFileMove_WithDeploymentID() {
	BundleMoveFile = func(coreClient astrocore.CoreClient, orgID, deploymentID, sourcePath, destination string) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), "old.py", sourcePath)
		assert.Equal(s.T(), "new.py", destination)
		return nil
	}

	cmd := newBundleFileMoveCmd(os.Stdout)
	err := execBundleCmd(cmd, "test-deploy-id", "old.py", "--destination", "new.py")
	assert.NoError(s.T(), err)
}

// --- bundle file duplicate ---

func (s *BundleCmdSuite) TestBundleFileDuplicate_WithDeploymentID() {
	BundleDuplicateFile = func(coreClient astrocore.CoreClient, orgID, deploymentID, sourcePath, destination string) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), "src.py", sourcePath)
		assert.Equal(s.T(), "dst.py", destination)
		return nil
	}

	cmd := newBundleFileDuplicateCmd(os.Stdout)
	err := execBundleCmd(cmd, "test-deploy-id", "src.py", "--destination", "dst.py")
	assert.NoError(s.T(), err)
}

// --- bundle file upload ---

func (s *BundleCmdSuite) TestBundleFileUpload_WithDeploymentID() {
	tmpFile := filepath.Join(s.T().TempDir(), "test.py")
	_ = os.WriteFile(tmpFile, []byte("print('hello')"), 0o644)

	BundleUploadFile = func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, remotePath, localPath string) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), "dags/test.py", remotePath)
		assert.Equal(s.T(), tmpFile, localPath)
		return nil
	}

	cmd := newBundleFileUploadCmd(os.Stdout)
	err := execBundleCmd(cmd, "test-deploy-id", tmpFile, "--remote-path", "dags/test.py")
	assert.NoError(s.T(), err)
}

// --- bundle file download ---

func (s *BundleCmdSuite) TestBundleFileDownload_WithDeploymentID() {
	outputPath := filepath.Join(s.T().TempDir(), "output.py")

	BundleDownloadFile = func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, remotePath, outPath string, out io.Writer) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), "dags/test.py", remotePath)
		assert.Equal(s.T(), outputPath, outPath)
		return nil
	}

	bundleFileOutputPath = "" // reset shared state
	cmd := newBundleFileDownloadCmd(os.Stdout)
	err := execBundleCmd(cmd, "test-deploy-id", "dags/test.py", "--output", outputPath)
	assert.NoError(s.T(), err)
}

// --- bundle sync ---

func (s *BundleCmdSuite) TestBundleSync_WithDeploymentID() {
	BundleSync = func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, localDir, targetPath string, overwrite bool) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), ".", localDir)
		assert.True(s.T(), overwrite)
		return nil
	}

	err := execBundleCmd(newBundleSyncCmd(os.Stdout), "test-deploy-id")
	assert.NoError(s.T(), err)
}

func (s *BundleCmdSuite) TestBundleSync_NoOverwrite() {
	BundleSync = func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, localDir, targetPath string, overwrite bool) error {
		assert.False(s.T(), overwrite)
		return nil
	}

	err := execBundleCmd(newBundleSyncCmd(os.Stdout), "test-deploy-id", "--no-overwrite")
	assert.NoError(s.T(), err)
}

// --- bundle archive upload ---

func (s *BundleCmdSuite) TestBundleArchiveUpload_WithDeploymentID() {
	tmpFile := filepath.Join(s.T().TempDir(), "bundle.tar.gz")
	_ = os.WriteFile(tmpFile, []byte("fake"), 0o644)

	BundleUploadArchive = func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, archivePath, targetPath string, overwrite bool) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), tmpFile, archivePath)
		assert.True(s.T(), overwrite)
		return nil
	}

	cmd := newBundleArchiveUploadCmd(os.Stdout)
	err := execBundleCmd(cmd, "test-deploy-id", tmpFile)
	assert.NoError(s.T(), err)
}

// --- bundle archive download ---

func (s *BundleCmdSuite) TestBundleArchiveDownload_WithDeploymentID() {
	outputPath := filepath.Join(s.T().TempDir(), "bundle.tar.gz")

	BundleDownloadArch = func(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, outPath string, out io.Writer) error {
		assert.Equal(s.T(), "test-deploy-id", deploymentID)
		assert.Equal(s.T(), outputPath, outPath)
		return nil
	}

	bundleArchiveOutputPath = "" // reset shared state
	cmd := newBundleArchiveDownloadCmd(os.Stdout)
	err := execBundleCmd(cmd, "test-deploy-id", "--output", outputPath)
	assert.NoError(s.T(), err)
}

// --- helpers ---

func (s *BundleCmdSuite) mockListTestDeployments() {
	s.mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{
				{
					Id: "test-deployment-id",
				},
			},
		},
	}, nil)
}

func (s *BundleCmdSuite) mockGetTestDeployment() {
	s.mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id: "test-deployment-id",
		},
	}, nil)
}
