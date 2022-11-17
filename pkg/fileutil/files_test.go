package fileutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("mock error")

type FileMode = fs.FileMode

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
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "basic case",
			args:    args{path: "./test.out", s: "testing"},
			wantErr: false,
		},
	}
	defer afero.NewOsFs().Remove("./test.out")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WriteStringToFile(tt.args.path, tt.args.s); (err != nil) != tt.wantErr {
				t.Errorf("WriteStringToFile() error = %v, wantErr %v", err, tt.wantErr)
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
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "basic case",
			args:    args{source: "./test", target: "/tmp"},
			wantErr: false,
		},
	}
	defer afero.NewOsFs().Remove(path)
	defer afero.NewOsFs().Remove("./test/symlink")
	defer afero.NewOsFs().Remove("./test")
	defer afero.NewOsFs().Remove("/tmp/test.tar")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Tar(tt.args.source, tt.args.target); (err != nil) != tt.wantErr {
				t.Errorf("Tar() error = %v, wantErr %v", err, tt.wantErr)
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
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.NoError(t, err)
		})

		t.Run(tt.name, func(t *testing.T) {
			openFile = openFileError
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.ErrorIs(t, err, errMock)
		})

		t.Run(tt.name, func(t *testing.T) {
			openFile = os.OpenFile
			readFile = readFileError
			err := AddLineToFile(tt.args.filePath, tt.args.lineText, tt.args.commentText)
			assert.ErrorIs(t, err, errMock)
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
