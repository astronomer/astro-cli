package fileutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	http_context "context"
	"fmt"
	"io"
	f "io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

type FileMode = f.FileMode

func openFileError(name string, flag int, perm FileMode) (*os.File, error) {
	return nil, errMock
}

func readFileError(name string) ([]byte, error) {
	return nil, errMock
}

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
	defer afero.NewOsFs().Remove(testSourceDirPath)

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
				filepath.Join(testSubDirName, testSubDirFileName),
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
				filepath.Join(testSourceDirName, testFileName),
				filepath.Join(testSourceDirName, symlinkFileName),
				filepath.Join(testSourceDirName, testSubDirName, testSubDirFileName),
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

func (s *Suite) TestContains() {
	type args struct {
		elems []string
		param string
	}
	tests := []struct {
		name         string
		args         args
		expectedResp bool
		expectedPos  int
	}{
		{
			name:         "should contain element case",
			args:         args{elems: []string{"sample.yaml", "test.yaml"}, param: "test.yaml"},
			expectedResp: true,
			expectedPos:  1,
		},
		{
			name:         "should not contain element case",
			args:         args{elems: []string{"sample.yaml", "test.yaml"}, param: "random.yaml"},
			expectedResp: false,
			expectedPos:  0,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			exist, index := Contains(tt.args.elems, tt.args.param)
			s.Equal(exist, tt.expectedResp)
			s.Equal(index, tt.expectedPos)
		})
	}
}

func (s *Suite) TestRead() {
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
		expectedResp []string
		errResp      string
	}{
		{
			name:         "should read file contents successfully",
			args:         args{path: filePath},
			expectedResp: []string{content},
			errResp:      "",
		},
		{
			name:         "error on read file content",
			args:         args{path: "incorrect-file"},
			expectedResp: nil,
			errResp:      "no such file or directory",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			actualResp, actualErr := Read(tt.args.path)
			if tt.errResp != "" && actualErr != nil {
				s.Contains(actualErr.Error(), tt.errResp)
			} else {
				s.NoError(actualErr)
			}
			s.Equal(tt.expectedResp, actualResp)
		})
	}
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
			errResp:      "no such file or directory",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			actualResp, actualErr := ReadFileToString(tt.args.path)
			if tt.errResp != "" && actualErr != nil {
				s.Contains(actualErr.Error(), tt.errResp)
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

func (s *Suite) TestAddLineToFile() {
	filePath := "./test.py"
	content := "testing"

	WriteStringToFile(filePath, content)
	defer afero.NewOsFs().Remove(filePath)

	type args struct {
		filePath    string
		lineText    string
		commentText string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "basic case",
			args: args{filePath: filePath, lineText: "test line!", commentText: ""},
		},
		{
			name: "fail open file",
			args: args{filePath: filePath, lineText: "test line!", commentText: ""},
		},
		{
			name: "fail read file",
			args: args{filePath: filePath, lineText: "test line!", commentText: ""},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			openFile = os.OpenFile
			readFile = os.ReadFile
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			s.NoError(err)
		})

		s.Run(tt.name, func() {
			openFile = openFileError
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			s.Contains(err.Error(), errMock.Error())
		})

		s.Run(tt.name, func() {
			openFile = os.OpenFile
			readFile = readFileError
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			s.Contains(err.Error(), errMock.Error())
		})
	}
}

func (s *Suite) TestRemoveLineFromFile() {
	filePath := "./test.py"
	content := "testing\nremove this"

	WriteStringToFile(filePath, content)
	defer afero.NewOsFs().Remove(filePath)

	type args struct {
		filePath    string
		lineText    string
		commentText string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "basic case",
			args: args{filePath: filePath, lineText: "remove this", commentText: ""},
		},
		{
			name: "fail open file",
			args: args{filePath: filePath, lineText: "remove this", commentText: ""},
		},
		{
			name: "fail read file",
			args: args{filePath: filePath, lineText: "remove this", commentText: ""},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			openFile = os.OpenFile
			readFile = os.ReadFile
			err := RemoveLineFromFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			s.NoError(err)
		})

		s.Run(tt.name, func() {
			openFile = openFileError
			err := RemoveLineFromFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			s.ErrorIs(err, errMock)
		})

		s.Run(tt.name, func() {
			openFile = os.OpenFile
			readFile = readFileError
			err := RemoveLineFromFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			s.ErrorIs(err, errMock)
		})
	}
}

func (s *Suite) TestGzipFile() {
	s.Run("source file not found", func() {
		err := GzipFile("non-existent-file.txt", "./zipped.txt.gz")
		s.EqualError(err, "open non-existent-file.txt: no such file or directory")
	})

	s.Run("destination file error", func() {
		// Create a temporary source file
		srcContent := []byte("This is a test file.")
		srcFilePath := "./testFileForTestGzipFile.txt"
		err := os.WriteFile(srcFilePath, srcContent, os.ModePerm)
		s.NoError(err)
		defer os.Remove(srcFilePath)

		err = GzipFile(srcFilePath, "/invalidPath/zipped.txt.gz")
		s.EqualError(err, "open /invalidPath/zipped.txt.gz: no such file or directory")
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

func getCapturedRequest(server *httptest.Server) *http.Request {
	handler, ok := server.Config.Handler.(*testHandler)
	if !ok {
		panic("Unexpected server handler type")
	}
	return handler.Request
}

type testHandler struct {
	StatusCode   int
	ResponseBody string
	Headers      map[string][]string
	Request      *http.Request
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Request = r
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
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err := UploadFile(&uploadFileArgs)
		s.EqualError(err, "error opening file: open non-existent-file.txt: no such file or directory")
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
			InitialDelayInMS:    1 * 1000,
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
			InitialDelayInMS:    1 * 1000,
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
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		// Assert the error is as expected
		s.EqualError(err, "file upload failed. Status code: 500 and Message: Internal Server Error")
		// Assert that the server is hit required number of times
		s.Equal(2, mockServer.hitCount)
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
			InitialDelayInMS:    1 * 1000,
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
			InitialDelayInMS:    1 * 1000,
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
			Description:         "Deployed via <astro deploy --dags>n",
			MaxTries:            2,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		s.NoError(err, "Expected no error")
		// assert the received headers
		request := getCapturedRequest(server)
		s.Equal("Bearer token", request.Header.Get("Authorization"))
		s.Contains(request.Header.Get("Content-Type"), "multipart/form-data")
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
            InitialDelayInMS:    1 * 1000,
            BackoffFactor:       2,
            RetryDisplayMessage: "please wait, attempting to upload the dags",
        }
        err = UploadFile(&uploadFileArgs)

        s.NoError(err, "Expected no error")
        request := getCapturedRequest(server)
        s.Equal("Bearer token", request.Header.Get("Authorization"))
        s.Contains(request.Header.Get("Content-Type"), "multipart/form-data")

        body, _ := io.ReadAll(request.Body)
        s.NotContains(string(body), "description")
    })
}
