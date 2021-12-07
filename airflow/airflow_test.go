package airflow

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"

	"github.com/stretchr/testify/assert"
)

func TestInitDirs(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dirs := []string{"dags"}

	err = initDirs(tmpDir, dirs)
	assert.NoError(t, err)

	exist, err := fileutil.Exists(filepath.Join(tmpDir, "dags"))

	assert.NoError(t, err)
	assert.True(t, exist)
}

func TestInitDirsEmpty(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = initDirs(tmpDir, nil)
	assert.NoError(t, err)
}

func TestInitFiles(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	files := map[string]string{
		"requirements.txt": "",
	}

	err = initFiles(tmpDir, files)
	assert.NoError(t, err)

	exist, err := fileutil.Exists(filepath.Join(tmpDir, "requirements.txt"))

	assert.NoError(t, err)
	assert.True(t, exist)
}

func TestInit(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0777)
	config.InitConfig(fs)
	tmpDir, err := ioutil.TempDir("", "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = Init(tmpDir, "test")
	assert.NoError(t, err)

	expectedFiles := []string{
		".dockerignore",
		"Dockerfile",
		".gitignore",
		"packages.txt",
		"requirements.txt",
		".env",
		"airflow_settings.yaml",
		"dags/example-dag.py",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file))
		assert.NoError(t, err)
		assert.True(t, exist)
	}
}

func Test_airflowVersionFromDockerFile(t *testing.T) {
	airflowHome := config.WorkingPath + "/testfiles"
	type args struct {
		dockerFile string
	}
	tests := []struct {
		name           string
		args           args
		wantErr        bool
		expectedResult interface{}
	}{
		{
			name: "airflow version 1 ok",
			args: args{
				dockerFile: "Dockerfile.Airflow1.ok",
			},
			wantErr:        false,
			expectedResult: uint64(0x1),
		},
		{
			name: "airflow version 2 ok",
			args: args{
				dockerFile: "Dockerfile.Airflow2.ok",
			},
			wantErr:        false,
			expectedResult: uint64(0x2),
		},
		{
			name: "invalid airflow tag ok",
			args: args{
				dockerFile: "Dockerfile.tag.invalid",
			},
			wantErr:        false,
			expectedResult: uint64(0x1),
		},
		{
			name: "invalid dockerfile",
			args: args{
				dockerFile: "Dockerfile.not.real",
			},
			wantErr:        true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := ParseVersionFromDockerFile(airflowHome, tt.args.dockerFile)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.expectedResult != nil {
				assert.Equal(t, tt.expectedResult, version)
			}
		})
	}
}
