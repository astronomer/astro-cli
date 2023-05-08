package fileutil

import (
	f "io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"
)

var errMock = errors.New("mock error")

type FileMode = f.FileMode

func openFileError(name string, flag int, perm FileMode) (*os.File, error) {
	return nil, errMock
}

func readFileError(name string) ([]byte, error) {
	return nil, errMock
}

type Suite struct {
	suite.Suite
}

func TestPkgFileUtilSuite(t *testing.T) {
	suite.Run(t, new(Suite))
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
		s.Run(tt.name, func() {
			if err := WriteStringToFile(tt.args.path, tt.args.s); (err != nil) != tt.wantErr {
				s.Error(err)
			}
			_, err := os.Open(tt.args.path)
			s.NoError(err)
		})
	}
}

func (s *Suite) TestTar() {
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
		s.Run(tt.name, func() {
			if err := Tar(tt.args.source, tt.args.target); (err != nil) != tt.wantErr {
				s.Error(err)
			}
			filePath := "/tmp/test.tar"
			_, err := os.Create(filePath)
			s.NoError(err)
			_, err = os.Stat(tt.args.source)
			s.NoError(err)
			_, err = os.Open(tt.args.source)
			s.NoError(err)
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
