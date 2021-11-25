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
	configYaml := testUtils.NewTestConfig()
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

	// Version 1
	expected := uint64(0x1)
	dockerfile := "Dockerfile.Airflow1.ok"
	version, err := ParseVersionFromDockerFile(airflowHome, dockerfile)

	assert.NoError(t, err)
	assert.Equal(t, expected, version)

	// Version 2
	expected = uint64(0x2)
	dockerfile = "Dockerfile.Airflow2.ok"
	version, err = ParseVersionFromDockerFile(airflowHome, dockerfile)

	assert.NoError(t, err)
	assert.Equal(t, expected, version)

	// Default to Airflow 1 when there is an invalid Tag
	expected = uint64(0x1)
	dockerfile = "Dockerfile.tag.invalid"
	version, err = ParseVersionFromDockerFile(airflowHome, dockerfile)

	assert.NoError(t, err)
	assert.Equal(t, expected, version)

	// Invalid Dockerfile
	dockerfile = "Dockerfile.not.real"
	_, err = ParseVersionFromDockerFile(airflowHome, dockerfile)

	assert.Error(t, err)
}
