package airflow

import (
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

func (s *Suite) TestInitDirs() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	dirs := []string{"dags"}

	err = initDirs(tmpDir, dirs)
	s.NoError(err)

	exist, err := fileutil.Exists(filepath.Join(tmpDir, "dags"), nil)

	s.NoError(err)
	s.True(exist)
}

func (s *Suite) TestInitDirsEmpty() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = initDirs(tmpDir, nil)
	s.NoError(err)
}

func (s *Suite) TestInitFiles() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	files := map[string]string{
		"requirements.txt": "",
	}

	err = initFiles(tmpDir, files)
	s.NoError(err)

	exist, err := fileutil.Exists(filepath.Join(tmpDir, "requirements.txt"), nil)

	s.NoError(err)
	s.True(exist)
}

func (s *Suite) TestInit() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = Init(tmpDir, "astro-runtime", "test", "")
	s.NoError(err)

	expectedFiles := []string{
		".dockerignore",
		"Dockerfile",
		".gitignore",
		"packages.txt",
		"requirements.txt",
		".env",
		"airflow_settings.yaml",
		"dags/exampledag.py",
		"dags/.airflowignore",
		"README.md",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist)
	}
}

func (s *Suite) TestTemplateInit() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = Init(tmpDir, "astro-runtime", "test", "etl")
	s.NoError(err)

	expectedFiles := []string{
		".dockerignore",
		"Dockerfile",
		".gitignore",
		"packages.txt",
		"requirements.txt",
		"dags/example_etl_galaxies.py",
		"dags/.airflowignore",
		"README.md",
		"include",
		"tests/dags",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist)
	}
}

func (s *Suite) TestInitConflictTest() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = initConflictTest(tmpDir, "astro-runtime", "test")
	s.NoError(err)

	expectedFiles := []string{
		"conflict-check.Dockerfile",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist)
	}
}
