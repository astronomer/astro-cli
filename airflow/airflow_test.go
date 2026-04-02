package airflow

import (
	"errors"
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

	err = Init(tmpDir, "astro-runtime", "12.0.0", "", "")
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
	ExtractTemplate = func(templateDir, destDir string) error {
		err := os.MkdirAll(destDir, os.ModePerm)
		s.NoError(err)
		mockFile := filepath.Join(destDir, "requirements.txt")
		file, err := os.Create(mockFile)
		s.NoError(err)
		defer file.Close()
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = Init(tmpDir, "astro-runtime", "test", "etl", "")
	s.NoError(err)

	expectedFiles := []string{
		"requirements.txt",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist)
	}
}

func (s *Suite) TestInitWithClientImageTag() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	// Test with Airflow 3 and clientImageTag
	err = Init(tmpDir, "astro-runtime", "3.0-1", "", "3.1-1-python-3.12-astro-agent-1.1.0")
	s.NoError(err)

	// Check that standard files are created
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
		"tests/dags/test_dag_example.py",
		".astro/test_dag_integrity_default.py",
		".astro/dag_integrity_exceptions.txt",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist, "Expected file %s to exist", file)
	}

	// Check that client-specific files are created when clientImageTag is provided
	clientFiles := []string{
		"Dockerfile.client",
		"requirements-client.txt",
		"packages-client.txt",
	}
	for _, file := range clientFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist, "Expected client file %s to exist", file)
	}
}

func (s *Suite) TestInitWithoutClientImageTag() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	// Test with Airflow 3 but no clientImageTag
	err = Init(tmpDir, "astro-runtime", "3.0-1", "", "")
	s.NoError(err)

	// Check that client-specific files are NOT created when clientImageTag is empty
	clientFiles := []string{
		"Dockerfile.client",
		"requirements-client.txt",
		"packages-client.txt",
	}
	for _, file := range clientFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.False(exist, "Expected client file %s to NOT exist", file)
	}
}

func (s *Suite) TestInitPyProject() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = InitPyProject(tmpDir, "my-project", "3.0.1", "3.0-7", "3.12")
	s.NoError(err)

	// Files that SHOULD exist
	expectedFiles := []string{
		"pyproject.toml",
		".gitignore",
		".env",
		"airflow_settings.yaml",
		"dags/exampledag.py",
		"dags/.airflowignore",
		"tests/dags/test_dag_example.py",
		".astro/test_dag_integrity_default.py",
		".astro/dag_integrity_exceptions.txt",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.True(exist, "Expected file %s to exist", file)
	}

	// Files that should NOT exist (Dockerfile-format-specific)
	dockerFiles := []string{
		"Dockerfile",
		"README.md",
	}
	for _, file := range dockerFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		s.NoError(err)
		s.False(exist, "Expected file %s to NOT exist", file)
	}

	// Verify pyproject.toml content
	content, err := os.ReadFile(filepath.Join(tmpDir, "pyproject.toml"))
	s.NoError(err)
	s.Contains(string(content), `name = "my-project"`)
	s.Contains(string(content), `requires-python = ">=3.12"`)
	s.Contains(string(content), `airflow-version = "3.0.1"`)
	s.Contains(string(content), `runtime-version = "3.0-7"`)
	s.NotContains(string(content), "Dockerfile")
}

func (s *Suite) TestInitPyProject_CanBeReadBack() {
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	err = InitPyProject(tmpDir, "roundtrip-test", "3.0.1", "3.0-7", "3.12")
	s.NoError(err)

	// Read it back with our parser from Phase 1
	proj, err := ReadProject(tmpDir)
	s.NoError(err)
	s.Equal("roundtrip-test", proj.Name)
	s.Equal(">=3.12", proj.RequiresPython)
	s.Equal("3.0.1", proj.AirflowVersion)
	s.Equal("3.0-7", proj.RuntimeVersion)
	s.Equal("docker", proj.Mode) // default when not specified in template
	s.Empty(proj.Dependencies)   // empty list in template
}

func (s *Suite) TestTemplateInitFail() {
	ExtractTemplate = func(templateDir, destDir string) error {
		err := errors.New("error extracting files")
		return err
	}
	tmpDir, err := os.MkdirTemp("", "temp")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)
	err = Init(tmpDir, "astro-runtime", "test", "etl", "")
	s.EqualError(err, "failed to set up template-based astro project: error extracting files")
}
