package airflow

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/pkg/fileutil"
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
