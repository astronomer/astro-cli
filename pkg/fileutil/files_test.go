package fileutil

import (
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
)

var errMock = errors.New("mock error")

type FileMode = f.FileMode

func openFileError(name string, flag int, perm FileMode) (*os.File, error) {
	return nil, errMock
}

func readFileError(name string) ([]byte, error) {
	return nil, errMock
}

func TestExists(t *testing.T) {
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
			assert.Contains(t, actualErr.Error(), tt.errResp)
		} else {
			assert.NoError(t, actualErr)
		}
		assert.Equal(t, tt.expectedResp, actualResp)
	}
}

func TestWriteStringToFile(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			if tt.errAssertion(t, WriteStringToFile(tt.args.path, tt.args.s)) {
				return
			}
			if _, err := os.Open(tt.args.path); err != nil {
				t.Errorf("Error opening file %s", tt.args.path)
			}
		})
	}
}

func TestTar(t *testing.T) {
	os.Mkdir("./test", os.ModePerm)

	path := "./test/test.txt"
	WriteStringToFile(path, "testing")
	os.Symlink(path, filepath.Join("test", "symlink"))
	type args struct {
		source string
		target string
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic case",
			args:         args{source: "./test", target: "/tmp"},
			errAssertion: assert.NoError,
		},
	}
	defer afero.NewOsFs().Remove(path)
	defer afero.NewOsFs().Remove("./test/symlink")
	defer afero.NewOsFs().Remove("./test")
	defer afero.NewOsFs().Remove("/tmp/test.tar")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errAssertion(t, Tar(tt.args.source, tt.args.target)) {
				return
			}
			filePath := "/tmp/test.tar"
			if _, err := os.Create(filePath); err != nil {
				t.Errorf("Error creating file %s", filePath)
			}
			if _, err := os.Stat(tt.args.source); err != nil {
				t.Errorf("Error getting file stats %s", tt.args.source)
			}
			if _, err := os.Open(tt.args.source); err != nil {
				t.Errorf("Error opening file %s", tt.args.source)
			}
		})
	}
}

func TestContains(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			exist, index := Contains(tt.args.elems, tt.args.param)
			assert.Equal(t, exist, tt.expectedResp)
			assert.Equal(t, index, tt.expectedPos)
		})
	}
}

func TestRead(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			actualResp, actualErr := Read(tt.args.path)
			if tt.errResp != "" && actualErr != nil {
				assert.Contains(t, actualErr.Error(), tt.errResp)
			} else {
				assert.NoError(t, actualErr)
			}
			assert.Equal(t, tt.expectedResp, actualResp)
		})
	}
}

func TestReadFileToString(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			actualResp, actualErr := ReadFileToString(tt.args.path)
			if tt.errResp != "" && actualErr != nil {
				assert.Contains(t, actualErr.Error(), tt.errResp)
			} else {
				assert.NoError(t, actualErr)
			}
			assert.Equal(t, tt.expectedResp, actualResp)
		})
	}
}

func TestGetFilesWithSpecificExtension(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			files := GetFilesWithSpecificExtension(tt.args.folderPath, tt.args.ext)
			assert.Equal(t, expectedFiles, files)
		})
	}
}

func TestAddLineToFile(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			openFile = os.OpenFile
			readFile = os.ReadFile
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.NoError(t, err)
		})

		t.Run(tt.name, func(t *testing.T) {
			openFile = openFileError
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.Contains(t, err.Error(), errMock.Error())
		})

		t.Run(tt.name, func(t *testing.T) {
			openFile = os.OpenFile
			readFile = readFileError
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.Contains(t, err.Error(), errMock.Error())
		})
	}
}

func TestRemoveLineFromFile(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			openFile = os.OpenFile
			readFile = os.ReadFile
			err := RemoveLineFromFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.NoError(t, err)
		})

		t.Run(tt.name, func(t *testing.T) {
			openFile = openFileError
			err := RemoveLineFromFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.ErrorIs(t, err, errMock)
		})

		t.Run(tt.name, func(t *testing.T) {
			openFile = os.OpenFile
			readFile = readFileError
			err := RemoveLineFromFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.ErrorIs(t, err, errMock)
		})
	}
}

func TestGzipFile(t *testing.T) {
	t.Run("source file not found", func(t *testing.T) {
		err := GzipFile("non-existent-file.txt", "./zipped.txt.gz")
		assert.EqualError(t, err, "open non-existent-file.txt: no such file or directory")
	})

	t.Run("destination file error", func(t *testing.T) {
		// Create a temporary source file
		srcContent := []byte("This is a test file.")
		srcFilePath := "./testFileForTestGzipFile.txt"
		err := os.WriteFile(srcFilePath, srcContent, os.ModePerm)
		assert.NoError(t, err)
		defer os.Remove(srcFilePath)

		err = GzipFile(srcFilePath, "/invalidPath/zipped.txt.gz")
		assert.EqualError(t, err, "open /invalidPath/zipped.txt.gz: no such file or directory")
	})

	t.Run("successful gzip", func(t *testing.T) {
		// Create a temporary source file
		srcContent := []byte("This is a test file.")
		srcFilePath := "./testFileForTestGzipFile.txt"
		err := os.WriteFile(srcFilePath, srcContent, os.ModePerm)
		assert.NoError(t, err)
		defer os.Remove(srcFilePath)

		destFilePath := "./zipped.txt.gz"
		err = GzipFile(srcFilePath, destFilePath)
		assert.NoError(t, err)
		defer os.Remove(destFilePath)

		// Create the expected content
		expectedContent := new(bytes.Buffer)
		gzipWriter := gzip.NewWriter(expectedContent)
		_, err = gzipWriter.Write(srcContent)
		assert.NoError(t, err, "Error writing to gzip buffer")
		gzipWriter.Close()

		// Check if the destination file has the expected content
		actualContent, err := os.ReadFile(destFilePath)
		assert.NoError(t, err, "Error reading gZipped file")
		assert.True(t, bytes.Equal(expectedContent.Bytes(), actualContent), "GZipped file content does not match expected")

		// Check if the gZipped file is a valid gzip file
		destFile, err := os.Open(destFilePath)
		assert.NoError(t, err, "Error opening gZipped file")
		defer destFile.Close()
		gzipReader, err := gzip.NewReader(destFile)
		assert.NoError(t, err, "Error creating gzip reader")
		defer gzipReader.Close()

		// Read the content from the gzip reader
		actualGZippedContent, err := io.ReadAll(gzipReader)
		assert.NoError(t, err, "Error reading gZipped file with gzip reader")
		assert.True(t, bytes.Equal(srcContent, actualGZippedContent), "Unzipped content does not match original")
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

func TestUploadFile(t *testing.T) {
	t.Run("attempt to upload a non-existent file", func(t *testing.T) {
		uploadFileArgs := UploadFileArguments{
			FilePath:            "non-existent-file.txt",
			TargetURL:           "http://localhost:8080/upload",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			MaxTries:            3,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err := UploadFile(&uploadFileArgs)
		assert.EqualError(t, err, "error opening file: open non-existent-file.txt: no such file or directory")
	})

	t.Run("io copy throws an error", func(t *testing.T) {
		ioCopyError := errors.New("mock error")

		ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
			return 0, ioCopyError
		}

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		assert.NoError(t, err, "Error creating test file")
		defer os.Remove(filePath)

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           "testURL",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			MaxTries:            3,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		assert.ErrorIs(t, err, ioCopyError)
		ioCopy = io.Copy
	})

	t.Run("newRequestWithContext throws an error", func(t *testing.T) {
		requestError := errors.New("mock error")
		newRequestWithContext = func(ctx http_context.Context, method, url string, body io.Reader) (*http.Request, error) {
			return nil, requestError
		}
		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		assert.NoError(t, err, "Error creating test file")
		defer os.Remove(filePath)

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           "testURL",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			MaxTries:            3,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		assert.ErrorIs(t, err, requestError)
		newRequestWithContext = http.NewRequestWithContext
	})

	t.Run("uploaded the file but got 500 response code. Uploading should be retried", func(t *testing.T) {
		mockServer := createMockServerWithHitCountReturning500()
		testServer := httptest.NewServer(mockServer)
		defer testServer.Close()

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		assert.NoError(t, err, "Error creating test file")
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
			MaxTries:            2,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		// Assert the error is as expected
		assert.EqualError(t, err, "file upload failed. Status code: 500 and Message: Internal Server Error")
		// Assert that the server is hit required number of times
		assert.Equal(t, 2, mockServer.hitCount)
	})

	t.Run("uploaded the file but got 400 response code. Uploading should not be retried", func(t *testing.T) {
		mockServer := createMockServerWithHitCountReturning400()
		testServer := httptest.NewServer(mockServer)
		defer testServer.Close()

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		assert.NoError(t, err, "Error creating test file")
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
			MaxTries:            2,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		// Assert the error is as expected
		assert.EqualError(t, err, "file upload failed. Status code: 400 and Message: Bad Request")
		// Assert that the server is hit required number of times
		assert.Equal(t, 1, mockServer.hitCount)
	})

	t.Run("error making POST request due to invalid URL.", func(t *testing.T) {
		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		assert.NoError(t, err, "Error creating test file")
		defer os.Remove(filePath)

		uploadFileArgs := UploadFileArguments{
			FilePath:            filePath,
			TargetURL:           "https://astro.unit.test",
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			MaxTries:            2,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		assert.ErrorContains(t, err, "astro.unit.test")
	})

	t.Run("successfully uploaded the file", func(t *testing.T) {
		// Prepare a test server to capture the request
		server := createMockServer(http.StatusOK, "OK", make(map[string][]string))
		defer server.Close()

		// Create a temporary file with some content for testing
		filePath := "./testFile.txt"
		fileContent := []byte("This is a test file.")
		err := os.WriteFile(filePath, fileContent, os.ModePerm)
		assert.NoError(t, err, "Error creating test file")
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
			MaxTries:            2,
			InitialDelayInMS:    1 * 1000,
			BackoffFactor:       2,
			RetryDisplayMessage: "please wait, attempting to upload the dags",
		}
		err = UploadFile(&uploadFileArgs)

		assert.NoError(t, err, "Expected no error")
		// assert the received headers
		request := getCapturedRequest(server)
		assert.Equal(t, "Bearer token", request.Header.Get("Authorization"))
		assert.Contains(t, request.Header.Get("Content-Type"), "multipart/form-data")
	})
}
