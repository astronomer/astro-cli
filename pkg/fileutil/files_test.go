package fileutil

import (
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

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
	defer func() { os.Remove("./test.out") }()
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
