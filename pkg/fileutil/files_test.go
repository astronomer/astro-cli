package fileutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	http_context "context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestFileUtil(t *testing.T) {
	suite.Run(t, new(Suite))
}

var errMock = errors.New("mock error")

func (s *Suite) TestExists() {
	filePath := "test.yaml"
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, filePath, []byte(`test`), 0o777)
	tempFile, _ := os.CreateTemp("", "test.yaml")
	defer os.Remove(tempFile.Name())
	type args struct {
		path string
		fs   afero.Fs
	}
	tests := []struct {
		name         string
		args         args
		expectedResp bool
		errResp      string
	}{
		{
			name: "file exists in fs",
			args: args{
				path: filePath,
				fs:   fs,
			},
			expectedResp: true,
			errResp:      "",
		},
		{
			name: "file does not exists in fs",
			args: args{
				path: "test_not_exists.yaml",
				fs:   fs,
			},
			expectedResp: false,
			errResp:      "",
		},
		{
			name: "return with an error when fs is not nil",
			args: args{
				path: "\000x", // invalid file name
				fs:   fs,
			},
			expectedResp: false,
			errResp:      "cannot determine if path exists, error ambiguous:",
		},
		{
			name: "file exists in os",
			args: args{
				path: tempFile.Name(),
				fs:   nil,
			},
			expectedResp: true,
			errResp:      "",
		},

		{
			name: "file doesnot exists in os",
			args: args{
				path: "test_not_exists.yaml",
				fs:   nil,
			},
			expectedResp: false,
			errResp:      "",
		},
		{
			name: "return with an error when fs is nil",
			args: args{
				path: "\000x", // invalid file name
				fs:   nil,
			},
			expectedResp: false,
			errResp:      "cannot determine if path exists, error ambiguous:",
		},
	}

	for _, tt := range tests {
		actualResp, actualErr := Exists(tt.args.path, tt.args.fs)
		if tt.errResp != "" && actualErr != nil {
			s.Contains(actualErr.Error(), tt.errResp)
		} else {
			s.NoError(actualErr)
		}
		s.Equal(tt.expectedResp, actualResp)
	}
}

func (s *Suite) TestWriteStringToFile() {
	type args struct {
		path string
		s    string
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic case",
			args:         args{path: "./test.out", s: "testing"},
			errAssertion: assert.NoError,
		},
	}
	defer afero.NewOsFs().Remove("./test.out")
	for _, tt := range tests {
		s.Run(tt.name, func() {
			if tt.errAssertion(s.T(), WriteStringToFile(tt.args.path, tt.args.s)) {
				return
			}
			_, err := os.Open(tt.args.path)
			s.NoError(err, "Error opening file %s", tt.args.path)
		})
	}
}

func (s *Suite) TestTar() {
	// create a test directory with a sub-directory "source" for the tar contents
	testDirPath, _ := os.MkdirTemp("", "")
	testSourceDirName := "source"
	testSourceDirPath := filepath.Join(testDirPath, testSourceDirName)
	defer os.RemoveAll(testDirPath)

	// create a test file, and a symlink to it
	testFileName := "test.txt"
	testFilePath := filepath.Join(testSourceDirPath, testFileName)
	WriteStringToFile(testFilePath, "testing")
	symlinkFileName := "symlink"
	symlinkFilePath := filepath.Join(testSourceDirPath, symlinkFileName)
	os.Symlink(testFilePath, symlinkFilePath)

	// create test file in a sub-directory
	testSubDirFileName := "test_subdir.txt"
	testSubDirName := "subdir"
	testSubDirPath := filepath.Join(testSourceDirPath, testSubDirName)
	testSubDirFilePath := filepath.Join(testSubDirPath, testSubDirFileName)
	_ = os.Mkdir(testSubDirPath, os.ModePerm)
	WriteStringToFile(testSubDirFilePath, "testing")

	WriteStringToFile(filepath.Join(testSourceDirPath, "__pycache__", "dag.cpython-313.pyc"), "bytecode")
	WriteStringToFile(filepath.Join(testSubDirPath, "__pycache__", "helper.cpython-313.pyc"), "bytecode")
	airflowIgnoreName := ".airflowignore"
	WriteStringToFile(filepath.Join(testSourceDirPath, airflowIgnoreName), "scratch/")
	sourcelessPycName := "vendored.pyc"
	WriteStringToFile(filepath.Join(testSourceDirPath, sourcelessPycName), "bytecode")

	type args struct {
		source         string
		target         string
		prependBaseDir bool
		excludePaths   []string
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
		expectPaths  []string
	}{
		{
			name: "no prepend base dir",
			args: args{
				source:         testSourceDirPath,
				target:         filepath.Join(testDirPath, "test_no_prepend.tar"),
				prependBaseDir: false,
			},
			errAssertion: assert.NoError,
			expectPaths: []string{
				testFileName,
				symlinkFileName,
				airflowIgnoreName,
				sourcelessPycName,
				// Tar entries are slash-separated whatever the platform.
				testSubDirName + "/" + testSubDirFileName,
			},
		},
		{
			name: "prepend base dir",
			args: args{
				source:         testSourceDirPath,
				target:         filepath.Join(testDirPath, "test_prepend.tar"),
				prependBaseDir: true,
			},
			errAssertion: assert.NoError,
			expectPaths: []string{
				testSourceDirName + "/" + testFileName,
				testSourceDirName + "/" + symlinkFileName,
				testSourceDirName + "/" + airflowIgnoreName,
				testSourceDirName + "/" + sourcelessPycName,
				testSourceDirName + "/" + testSubDirName + "/" + testSubDirFileName,
			},
		},
		{
			name: "exclude paths",
			args: args{
				source:         testSourceDirPath,
				target:         filepath.Join(testDirPath, "test_exclude.tar"),
				prependBaseDir: false,
				excludePaths:   []string{testSubDirName},
			},
			errAssertion: assert.NoError,
			expectPaths: []string{
				testFileName,
				symlinkFileName,
				airflowIgnoreName,
				sourcelessPycName,
				// testSubDirFileName excluded
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// check that the tar operation was successful
			assert.True(s.T(), tt.errAssertion(s.T(), Tar(tt.args.source, tt.args.target, tt.args.prependBaseDir, tt.args.excludePaths)))

			// check that all the files are in the tar at the correct paths
			file, err := os.Open(tt.args.target)
			if err != nil {
				s.Fail("Error opening file %s", tt.args.target)
			}
			defer file.Close()
			tarReader := tar.NewReader(file)
			numIteratedFiles := 0
			for {
				header, err := tarReader.Next()
				if err == io.EOF {
					break
				}
				require.NoError(s.T(), err)
				require.True(s.T(), s.Contains(tt.expectPaths, header.Name))
				numIteratedFiles++
			}
			require.Equal(s.T(), len(tt.expectPaths), numIteratedFiles)
		})
	}
}

// TarDir names the base directory itself, whatever the source is called,
// and fails on a source that is not there, where Tar makes an empty tarball.
func (s *Suite) TestTarDir() {
	dir := s.T().TempDir()
	source := filepath.Join(dir, "shared_dags")
	s.Require().NoError(os.MkdirAll(filepath.Join(source, "sub"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(source, "a.py"), nil, 0o600))
	s.Require().NoError(os.WriteFile(filepath.Join(source, "sub", "b.py"), nil, 0o600))

	target := filepath.Join(dir, "dags.tar")
	s.Require().NoError(TarDir(source, target, "dags"))
	file, err := os.Open(target)
	s.Require().NoError(err)
	defer file.Close()
	names := []string{}
	tr := tar.NewReader(file)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		s.Require().NoError(err)
		names = append(names, header.Name)
	}
	s.ElementsMatch([]string{"dags/a.py", "dags/sub/b.py"}, names)

	missing := filepath.Join(dir, "missing")
	s.ErrorIs(TarDir(missing, filepath.Join(dir, "missing.tar"), "dags"), os.ErrNotExist)
	s.NoError(Tar(missing, filepath.Join(dir, "missing-tar.tar"), true, nil), "Tar is unchanged")
}

// failingClose is a tarball file whose writes land and whose Close fails.
type failingClose struct{ bytes.Buffer }

func (*failingClose) Close() error { return errors.New("close failed") }

// A tarball that did not close is not one: TarDir and Tar both fail on it.
func (s *Suite) TestTarFailsOnAFailedClose() {
	prev := createTarFile
	defer func() { createTarFile = prev }()
	var written *failingClose
	createTarFile = func(string) (io.WriteCloser, error) {
		written = &failingClose{}
		return written, nil
	}
	source := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(source, "a.py"), []byte("a"), 0o600))

	s.EqualError(TarDir(source, "dags.tar", "dags"), "close failed")
	s.Positive(written.Len(), "the footer was flushed before the file was closed")
	s.EqualError(Tar(source, "dags.tar", true, nil), "close failed")
}

func (s *Suite) TestReadFileToString() {
	filePath := "./test.out"
	content := "testing"
	WriteStringToFile(filePath, content)
	defer afero.NewOsFs().Remove(filePath)
	type args struct {
		path string
	}
	tests := []struct {
		name         string
		args         args
		expectedResp string
		errResp      string
	}{
		{
			name:         "should read file contents successfully",
			args:         args{path: filePath},
			expectedResp: content,
			errResp:      "",
		},
		{
			name:         "error on read file content",
			args:         args{path: "incorrect-file"},
			expectedResp: "",
			errResp:      "open incorrect-file",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			actualResp, actualErr := ReadFileToString(tt.args.path)
			if tt.errResp != "" {
				s.Require().Error(actualErr)
				s.Contains(actualErr.Error(), tt.errResp)
				s.ErrorIs(actualErr, os.ErrNotExist)
			} else {
				s.NoError(actualErr)
			}
			s.Equal(tt.expectedResp, actualResp)
		})
	}
}

func (s *Suite) TestGetFilesWithSpecificExtension() {
	filePath := "./test.py"
	content := "testing"
	WriteStringToFile(filePath, content)
	defer afero.NewOsFs().Remove(filePath)

	expectedFiles := []string{"test.py"}
	type args struct {
		folderPath string
		ext        string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "basic case",
			args: args{folderPath: filePath, ext: ".py"},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			files := GetFilesWithSpecificExtension(tt.args.folderPath, tt.args.ext)
			s.Equal(expectedFiles, files)
		})
	}
}

func (s *Suite) TestGzipFile() {
	s.Run("source file not found", func() {
		err := GzipFile("non-existent-file.txt", "./zipped.txt.gz")
		s.ErrorContains(err, "open non-existent-file.txt: ")
		s.ErrorIs(err, os.ErrNotExist)
	})

	s.Run("destination file error", func() {
		// Create a temporary source file
		srcContent := []byte("This is a test file.")
		srcFilePath := "./testFileForTestGzipFile.txt"
		err := os.WriteFile(srcFilePath, srcContent, os.ModePerm)
		s.NoError(err)
		defer os.Remove(srcFilePath)

		err = GzipFile(srcFilePath, "/invalidPath/zipped.txt.gz")
		s.ErrorContains(err, "open /invalidPath/zipped.txt.gz: ")
		s.ErrorIs(err, os.ErrNotExist)
	})

	s.Run("successful gzip", func() {
		// Create a temporary source file
		srcContent := []byte("This is a test file.")
		srcFilePath := "./testFileForTestGzipFile.txt"
		err := os.WriteFile(srcFilePath, srcContent, os.ModePerm)
		s.NoError(err)
		defer os.Remove(srcFilePath)

		destFilePath := "./zipped.txt.gz"
		err = GzipFile(srcFilePath, destFilePath)
		s.NoError(err)
		defer os.Remove(destFilePath)

		// Create the expected content
		expectedContent := new(bytes.Buffer)
		gzipWriter := gzip.NewWriter(expectedContent)
		_, err = gzipWriter.Write(srcContent)
		s.NoError(err, "Error writing to gzip buffer")
		gzipWriter.Close()

		// Check if the destination file has the expected content
		actualContent, err := os.ReadFile(destFilePath)
		s.NoError(err, "Error reading gZipped file")
		s.True(bytes.Equal(expectedContent.Bytes(), actualContent), "GZipped file content does not match expected")

		// Check if the gZipped file is a valid gzip file
		destFile, err := os.Open(destFilePath)
		s.NoError(err, "Error opening gZipped file")
		defer destFile.Close()
		gzipReader, err := gzip.NewReader(destFile)
		s.NoError(err, "Error creating gzip reader")
		defer gzipReader.Close()

		// Read the content from the gzip reader
		actualGZippedContent, err := io.ReadAll(gzipReader)
		s.NoError(err, "Error reading gZipped file with gzip reader")
		s.True(bytes.Equal(srcContent, actualGZippedContent), "Unzipped content does not match original")
	})
}

func createMockServer(statusCode int, responseBody string, headers map[string][]string) *httptest.Server {
	handler := &testHandler{
		StatusCode:   statusCode,
		ResponseBody: responseBody,
		Headers:      headers,
	}
	return httptest.NewServer(handler)
}

func getCapturedRequest(server *httptest.Server) ([]byte, http.Header) {
	handler, ok := server.Config.Handler.(*testHandler)
	if !ok {
		panic("Unexpected server handler type")
	}
	return handler.RequestBody, handler.RequestHeader
}

type testHandler struct {
	StatusCode    int
	ResponseBody  string
	Headers       map[string][]string
	RequestBody   []byte
	RequestHeader http.Header
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, _ := io.ReadAll(r.Body)
	h.RequestBody = req
	h.RequestHeader = r.Header
	w.WriteHeader(h.StatusCode)
	for key, values := range h.Headers {
		w.Header()[key] = values
	}
	w.Write([]byte(h.ResponseBody))
}

type MockServerWithHitCountReturning500 struct {
	hitCount int
}

func (m *MockServerWithHitCountReturning500) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.hitCount++
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, "Internal Server Error")
}

func createMockServerWithHitCountReturning500() *MockServerWithHitCountReturning500 {
	return &MockServerWithHitCountReturning500{}
}

type MockServerWithHitCountReturning400 struct {
	hitCount int
}

func (m *MockServerWithHitCountReturning400) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.hitCount++
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprint(w, "Bad Request")
}

func createMockServerWithHitCountReturning400() *MockServerWithHitCountReturning400 {
	return &MockServerWithHitCountReturning400{}
}

func (s *Suite) TestUploadFile() {
	s.Run("attempt to upload a non-existent file", func() {
		uploadFileArgs := UploadFileArguments{
			FilePath:            "non-existent-file.txt",
			TargetURL:           "http://localhost:8080/upload",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            3,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err := UploadFile(&uploadFileArgs)
		s.ErrorContains(err, "error opening file: open non-existent-file.txt: ")
		s.ErrorIs(err, os.ErrNotExist)
	})

	s.Run("io copy throws an error", func() {
		ioCopyError := errors.New("mock error")

		ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
			return 0, ioCopyError
		}

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           "testURL",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            3,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		s.ErrorIs(err, ioCopyError)
		ioCopy = io.Copy
	})

	s.Run("newRequestWithContext throws an error", func() {
		requestError := errors.New("mock error")
		newRequestWithContext = func(ctx http_context.Context, method, url string, body io.Reader) (*http.Request, error) {
			return nil, requestError
		}
		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           "testURL",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            3,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		s.ErrorIs(err, requestError)
		newRequestWithContext = http.NewRequestWithContext
	})

	s.Run("uploaded the file but got 500 response code. Uploading should be retried", func() {
		mockServer := createMockServerWithHitCountReturning500()
		testServer := httptest.NewServer(mockServer)
		defer testServer.Close()

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		headers := map[string]string{
			"Authorization": "Bearer token",
			"Content-Type":  "application/json",
		}

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           testServer.URL,
			FormFileFieldName:   "file",
			Headers:             headers,
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            2,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
			Out:                 new(bytes.Buffer),
		}
		err = UploadFile(&uploadFileArgs)

		// Assert the error is as expected
		s.EqualError(err, "file upload failed. Status code: 500 and Message: Internal Server Error")
		// Assert that the server is hit required number of times
		s.Equal(2, mockServer.hitCount)
		// Each try's line goes to Out, not stdout.
		s.Equal(strings.Repeat("please wait, attempting to upload the dags\n", 2), uploadFileArgs.Out.(*bytes.Buffer).String())
	})

	s.Run("uploaded the file but got 400 response code. Uploading should not be retried", func() {
		mockServer := createMockServerWithHitCountReturning400()
		testServer := httptest.NewServer(mockServer)
		defer testServer.Close()

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		headers := map[string]string{
			"Authorization": "Bearer token",
			"Content-Type":  "application/json",
		}

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           testServer.URL,
			FormFileFieldName:   "file",
			Headers:             headers,
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            2,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		// Assert the error is as expected
		s.EqualError(err, "file upload failed. Status code: 400 and Message: Bad Request")
		// Assert that the server is hit required number of times
		s.Equal(1, mockServer.hitCount)
	})

	s.Run("error making POST request due to invalid URL.", func() {
		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           "https://astro.unit.test",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            2,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		s.ErrorContains(err, "astro.unit.test")
	})

	s.Run("successfully uploaded the file", func() {
		// Prepare a test server to capture the request
		server := createMockServer(http.StatusOK, "OK", make(map[string][]string))
		defer server.Close()

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		headers := map[string]string{
			"Authorization": "Bearer token",
			"Content-Type":  "application/json",
		}

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           server.URL,
			FormFileFieldName:   "file",
			Headers:             headers,
			Description:         "Deployed via <astro deploy --dags>",
			MaxTries:            2,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		s.NoError(err, "Expected no error")
		// assert the received headers
		reqBody, header := getCapturedRequest(server)
		s.Equal("Bearer token", header.Get("Authorization"))
		s.Contains(header.Get("Content-Type"), "multipart/form-data")
		s.Contains(string(reqBody), "description")
		s.Contains(string(reqBody), "Deployed via <astro deploy --dags>")
	})

	s.Run("successfully uploaded with an empty description", func() {
		server := createMockServer(http.StatusOK, "OK", make(map[string][]string))
		defer server.Close()

		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		s.NoError(err, "Error creating test file")
		defer os.Remove(filePath)

		headers := map[string]string{
			"Authorization": "Bearer token",
			"Content-Type":  "application/json",
		}

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           server.URL,
			FormFileFieldName:   "file",
			Headers:             headers,
			Description:         "",
			MaxTries:            2,
			InitialDelayInMS:    1,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		s.NoError(err, "Expected no error")
		reqBody, header := getCapturedRequest(server)
		s.Equal("Bearer token", header.Get("Authorization"))
		s.Contains(header.Get("Content-Type"), "multipart/form-data")
		s.NotContains(string(reqBody), "description")
	})
}

func (s *Suite) TestCopyFile() {
	tempDir := s.T().TempDir()

	s.Run("copies file successfully", func() {
		srcContent := "test file content"
		srcFile := filepath.Join(tempDir, "source.txt")
		dstFile := filepath.Join(tempDir, "destination.txt")

		err := os.WriteFile(srcFile, []byte(srcContent), 0o644)
		s.NoError(err)

		err = CopyFile(srcFile, dstFile)
		s.NoError(err)

		dstContent, err := os.ReadFile(dstFile)
		s.NoError(err)
		s.Equal(srcContent, string(dstContent))

		// Verify permissions are preserved
		srcInfo, err := os.Stat(srcFile)
		s.NoError(err)
		dstInfo, err := os.Stat(dstFile)
		s.NoError(err)
		s.Equal(srcInfo.Mode(), dstInfo.Mode())
	})

	s.Run("returns error when source doesn't exist", func() {
		srcFile := filepath.Join(tempDir, "nonexistent.txt")
		dstFile := filepath.Join(tempDir, "destination.txt")

		err := CopyFile(srcFile, dstFile)
		s.Error(err)
	})
}

func (s *Suite) TestCopyDirectory() {
	tempDir := s.T().TempDir()

	s.Run("copies directory successfully", func() {
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create source directory structure
		err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755)
		s.NoError(err)

		file1Content := "file 1 content"
		file2Content := "file 2 content"

		err = os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte(file1Content), 0o644)
		s.NoError(err)
		err = os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte(file2Content), 0o644)
		s.NoError(err)

		// Copy directory
		err = CopyDirectoryFiltered(srcDir, dstDir, nil)
		s.NoError(err)

		// Verify files were copied correctly
		copiedFile1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		s.NoError(err)
		s.Equal(file1Content, string(copiedFile1))

		copiedFile2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
		s.NoError(err)
		s.Equal(file2Content, string(copiedFile2))

		// Verify directory structure
		_, err = os.Stat(filepath.Join(dstDir, "subdir"))
		s.NoError(err)
	})

	s.Run("returns error when source doesn't exist", func() {
		srcDir := filepath.Join(tempDir, "nonexistent")
		dstDir := filepath.Join(tempDir, "dst")

		err := CopyDirectoryFiltered(srcDir, dstDir, nil)
		s.Error(err)
	})

	s.Run("copies symlink to directory without dereferencing it", func() {
		root := s.T().TempDir()
		srcDir := filepath.Join(root, "src")
		dstDir := filepath.Join(root, "dst")

		// A real directory the symlink points at, plus the symlink itself.
		// This reproduces terragrunt provider-cache symlinks that previously
		// triggered "read ...: is a directory".
		target := filepath.Join(root, "target-dir")
		s.NoError(os.MkdirAll(target, 0o755))
		s.NoError(os.WriteFile(filepath.Join(target, "inside.txt"), []byte("hi"), 0o644))

		s.NoError(os.MkdirAll(srcDir, 0o755))
		s.NoError(os.WriteFile(filepath.Join(srcDir, "regular.txt"), []byte("ok"), 0o644))
		s.NoError(os.Symlink(target, filepath.Join(srcDir, "link-to-dir")))

		err := CopyDirectoryFiltered(srcDir, dstDir, nil)
		s.NoError(err)

		// The regular file is copied.
		content, err := os.ReadFile(filepath.Join(dstDir, "regular.txt"))
		s.NoError(err)
		s.Equal("ok", string(content))

		// The symlink is recreated as a symlink, not dereferenced.
		info, err := os.Lstat(filepath.Join(dstDir, "link-to-dir"))
		s.NoError(err)
		s.NotZero(info.Mode()&os.ModeSymlink, "expected link-to-dir to remain a symlink")
		linkTarget, err := os.Readlink(filepath.Join(dstDir, "link-to-dir"))
		s.NoError(err)
		s.Equal(target, linkTarget)
	})

	s.Run("skip predicate excludes matching files and prunes directories", func() {
		root := s.T().TempDir()
		srcDir := filepath.Join(root, "src")
		dstDir := filepath.Join(root, "dst")

		s.NoError(os.MkdirAll(filepath.Join(srcDir, "infra", "agent"), 0o755))
		s.NoError(os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0o644))
		s.NoError(os.WriteFile(filepath.Join(srcDir, "infra", "agent", "secret.tf"), []byte("x"), 0o644))

		skip := func(relPath string, isDir bool) bool {
			return relPath == "infra"
		}

		err := CopyDirectoryFiltered(srcDir, dstDir, skip)
		s.NoError(err)

		_, err = os.Stat(filepath.Join(dstDir, "keep.txt"))
		s.NoError(err)
		_, err = os.Stat(filepath.Join(dstDir, "infra"))
		s.True(os.IsNotExist(err), "expected infra/ to be pruned")
	})
}

func (s *Suite) TestDefaultPermissions() {
	// Directories default to 0o755 and files to 0o644 so the CLI stops writing
	// world-writable paths.
	s.Equal(os.FileMode(0o755), perm)
	s.Equal(0o755, openPermissions)
}
