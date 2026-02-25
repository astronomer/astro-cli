package bundle

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type BundleSuite struct {
	suite.Suite
	mockCoreClient   *astrocore_mocks.ClientWithResponsesInterface
	mockStreamClient *astrocore_mocks.BundleFilesStreamClient
}

func (s *BundleSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.mockCoreClient = new(astrocore_mocks.ClientWithResponsesInterface)
	s.mockStreamClient = new(astrocore_mocks.BundleFilesStreamClient)
}

func (s *BundleSuite) TearDownTest() {
	s.mockCoreClient.AssertExpectations(s.T())
	s.mockStreamClient.AssertExpectations(s.T())
}

func TestBundle(t *testing.T) {
	suite.Run(t, new(BundleSuite))
}

func (s *BundleSuite) TestListFiles_Success() {
	out := new(bytes.Buffer)
	s.mockCoreClient.On("ListBundleFilesWithResponse", mock.Anything, "org-id", "deploy-id", mock.Anything).Return(&astrocore.ListBundleFilesResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		Body:         []byte(`[{"name":"dags/example.py","size":1234,"type":"file"},{"name":"dags/","size":0,"type":"directory"}]`),
	}, nil)

	err := ListFiles(out, s.mockCoreClient, "org-id", "deploy-id", "")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), out.String(), "example.py")
	assert.Contains(s.T(), out.String(), "directory")
}

func (s *BundleSuite) TestListFiles_EmptyResult() {
	out := new(bytes.Buffer)
	s.mockCoreClient.On("ListBundleFilesWithResponse", mock.Anything, "org-id", "deploy-id", mock.Anything).Return(&astrocore.ListBundleFilesResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		Body:         []byte(`[]`),
	}, nil)

	err := ListFiles(out, s.mockCoreClient, "org-id", "deploy-id", "")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), out.String(), "No files found")
}

func (s *BundleSuite) TestDeleteFile_Success() {
	s.mockCoreClient.On("DeleteBundleFileWithResponse", mock.Anything, "org-id", "deploy-id", "dags/test.py").Return(&astrocore.DeleteBundleFileResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
		Body:         []byte{},
	}, nil)

	err := DeleteFile(s.mockCoreClient, "org-id", "deploy-id", "dags/test.py")
	assert.NoError(s.T(), err)
}

func (s *BundleSuite) TestMoveFile_Success() {
	s.mockCoreClient.On("MoveBundleFileWithResponse", mock.Anything, "org-id", "deploy-id", "old.py", astrocore.MoveBundleFileJSONRequestBody{
		Destination: "new.py",
	}).Return(&astrocore.MoveBundleFileResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
		Body:         []byte{},
	}, nil)

	err := MoveFile(s.mockCoreClient, "org-id", "deploy-id", "old.py", "new.py")
	assert.NoError(s.T(), err)
}

func (s *BundleSuite) TestDuplicateFile_Success() {
	s.mockCoreClient.On("DuplicateBundleFileWithResponse", mock.Anything, "org-id", "deploy-id", "src.py", astrocore.DuplicateBundleFileJSONRequestBody{
		Destination: "dst.py",
	}).Return(&astrocore.DuplicateBundleFileResponse{
		HTTPResponse: &http.Response{StatusCode: 204},
		Body:         []byte{},
	}, nil)

	err := DuplicateFile(s.mockCoreClient, "org-id", "deploy-id", "src.py", "dst.py")
	assert.NoError(s.T(), err)
}

func (s *BundleSuite) TestUploadFile_Success() {
	tmpFile := filepath.Join(s.T().TempDir(), "test.py")
	err := os.WriteFile(tmpFile, []byte("print('hello')"), 0o644)
	assert.NoError(s.T(), err)

	s.mockStreamClient.On("UploadBundleFile", mock.Anything, "org-id", "deploy-id", "dags/test.py", mock.Anything, mock.AnythingOfType("int64")).Return(nil)

	err = UploadFile(s.mockStreamClient, "org-id", "deploy-id", "dags/test.py", tmpFile)
	assert.NoError(s.T(), err)
}

func (s *BundleSuite) TestDownloadFile_ToFile() {
	out := new(bytes.Buffer)
	outputPath := filepath.Join(s.T().TempDir(), "output.py")

	s.mockStreamClient.On("DownloadBundleFile", mock.Anything, "org-id", "deploy-id", "dags/test.py").Return(io.NopCloser(bytes.NewReader([]byte("print('hello')"))), nil)

	err := DownloadFile(s.mockStreamClient, "org-id", "deploy-id", "dags/test.py", outputPath, out)
	assert.NoError(s.T(), err)

	content, err := os.ReadFile(outputPath)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "print('hello')", string(content))
}

func (s *BundleSuite) TestSync_Success() {
	// Create a temp dir with a file
	tmpDir := s.T().TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "dag.py"), []byte("print('dag')"), 0o644)
	assert.NoError(s.T(), err)

	s.mockStreamClient.On("UploadBundleArchive", mock.Anything, "org-id", "deploy-id", mock.Anything, mock.AnythingOfType("int64"), true, "").Return(nil)

	err = Sync(s.mockStreamClient, "org-id", "deploy-id", tmpDir, "", true)
	assert.NoError(s.T(), err)
}

func (s *BundleSuite) TestUploadArchive_Success() {
	tmpFile := filepath.Join(s.T().TempDir(), "bundle.tar.gz")
	err := os.WriteFile(tmpFile, []byte("fake-archive"), 0o644)
	assert.NoError(s.T(), err)

	s.mockStreamClient.On("UploadBundleArchive", mock.Anything, "org-id", "deploy-id", mock.Anything, mock.AnythingOfType("int64"), true, "dags/").Return(nil)

	err = UploadArchive(s.mockStreamClient, "org-id", "deploy-id", tmpFile, "dags/", true)
	assert.NoError(s.T(), err)
}

func (s *BundleSuite) TestDownloadArchive_Success() {
	out := new(bytes.Buffer)
	outputPath := filepath.Join(s.T().TempDir(), "bundle.tar.gz")

	s.mockStreamClient.On("DownloadBundleArchive", mock.Anything, "org-id", "deploy-id").Return(io.NopCloser(bytes.NewReader([]byte("fake-archive"))), nil)

	err := DownloadArchive(s.mockStreamClient, "org-id", "deploy-id", outputPath, out)
	assert.NoError(s.T(), err)

	content, err := os.ReadFile(outputPath)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "fake-archive", string(content))
}
