package airflow

import (
	"os"
	"path/filepath"
)

const validPyProject = `[project]
name = "my-airflow-project"
requires-python = ">=3.12"
dependencies = [
    "apache-airflow-providers-snowflake>=5.0",
    "pandas>=2.0",
]

[tool.astro]
airflow-version = "3.0.1"
runtime-version = "13.0.0"
mode = "standalone"

[tool.astro.docker]
system-packages = ["gcc", "libpq-dev"]
`

const minimalPyProject = `[project]
name = "minimal"

[tool.astro]
airflow-version = "3.0.1"
`

const noAstroSection = `[project]
name = "just-ruff"

[tool.ruff]
line-length = 120
`

const invalidMode = `[project]
name = "bad-mode"

[tool.astro]
airflow-version = "3.0.1"
mode = "kubernetes"
`

const missingAirflowVersion = `[project]
name = "no-version"

[tool.astro]
runtime-version = "13.0.0"
`

// --- ReadProject tests ---

func (s *Suite) TestReadProject_Valid() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(validPyProject), 0o644))

	proj, err := ReadProject(tmpDir)
	s.NoError(err)
	s.Equal("my-airflow-project", proj.Name)
	s.Equal(">=3.12", proj.RequiresPython)
	s.Equal([]string{"apache-airflow-providers-snowflake>=5.0", "pandas>=2.0"}, proj.Dependencies)
	s.Equal("3.0.1", proj.AirflowVersion)
	s.Equal("13.0.0", proj.RuntimeVersion)
	s.Equal("standalone", proj.Mode)
	s.Equal([]string{"gcc", "libpq-dev"}, proj.SystemPackages)
}

func (s *Suite) TestReadProject_Minimal() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(minimalPyProject), 0o644))

	proj, err := ReadProject(tmpDir)
	s.NoError(err)
	s.Equal("minimal", proj.Name)
	s.Equal("", proj.RequiresPython)
	s.Nil(proj.Dependencies)
	s.Equal("3.0.1", proj.AirflowVersion)
	s.Equal("", proj.RuntimeVersion)
	s.Equal("docker", proj.Mode) // default
	s.Nil(proj.SystemPackages)
}

func (s *Suite) TestReadProject_NoAstroSection() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(noAstroSection), 0o644))

	_, err = ReadProject(tmpDir)
	s.Error(err)
	s.Contains(err.Error(), "[tool.astro]")
}

func (s *Suite) TestReadProject_InvalidMode() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(invalidMode), 0o644))

	_, err = ReadProject(tmpDir)
	s.Error(err)
	s.Contains(err.Error(), "kubernetes")
}

func (s *Suite) TestReadProject_MissingAirflowVersion() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(missingAirflowVersion), 0o644))

	_, err = ReadProject(tmpDir)
	s.Error(err)
	s.Contains(err.Error(), "airflow-version")
}

func (s *Suite) TestReadProject_FileNotFound() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	proj, err := ReadProject(tmpDir)
	s.NoError(err) // missing file is not an error — just means not a pyproject project
	s.Nil(proj)
}

func (s *Suite) TestTryReadProject_FileNotFound() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	proj, found, err := TryReadProject(tmpDir)
	s.NoError(err)
	s.False(found)
	s.Nil(proj)
}

func (s *Suite) TestTryReadProject_BrokenTOML() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte("{{broken"), 0o644))

	proj, found, err := TryReadProject(tmpDir)
	s.True(found) // file exists
	s.Error(err)  // but is broken
	s.Nil(proj)
}

func (s *Suite) TestReadProject_InvalidTOML() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte("{{invalid toml"), 0o644))

	_, err = ReadProject(tmpDir)
	s.Error(err)
	s.Contains(err.Error(), "failed to parse")
}

// --- IsPyProject tests ---

func (s *Suite) TestIsPyProject_WithAstroSection() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(validPyProject), 0o644))

	s.True(IsPyProject(tmpDir))
}

func (s *Suite) TestIsPyProject_WithoutAstroSection() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(noAstroSection), 0o644))

	s.False(IsPyProject(tmpDir))
}

func (s *Suite) TestIsPyProject_NoFile() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.False(IsPyProject(tmpDir))
}

// --- DetectProjectFormat tests ---

func (s *Suite) TestDetectProjectFormat_PyProject() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(validPyProject), 0o644))

	s.Equal(ProjectFormatPy, DetectProjectFormat(tmpDir))
}

func (s *Suite) TestDetectProjectFormat_Dockerfile() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM astro-runtime:12.0.0"), 0o644))

	s.Equal(ProjectFormatDocker, DetectProjectFormat(tmpDir))
}

func (s *Suite) TestDetectProjectFormat_BothPrefersPyProject() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(validPyProject), 0o644))
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM astro-runtime:12.0.0"), 0o644))

	s.Equal(ProjectFormatPy, DetectProjectFormat(tmpDir))
}

func (s *Suite) TestDetectProjectFormat_Unknown() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	s.Equal(ProjectFormatUnknown, DetectProjectFormat(tmpDir))
}

func (s *Suite) TestDetectProjectFormat_PyProjectWithoutAstro() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	// pyproject.toml exists but without [tool.astro] — should NOT be detected as pyproject format
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(noAstroSection), 0o644))
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM astro-runtime:12.0.0"), 0o644))

	s.Equal(ProjectFormatDocker, DetectProjectFormat(tmpDir))
}

// --- PinRuntimeVersion tests ---

func (s *Suite) TestPinRuntimeVersion_AddsWhenMissing() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	initial := "[project]\nname = \"test\"\n\n[tool.astro]\nairflow-version = \"3.0.1\"\n"
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(initial), 0o644))

	err = PinRuntimeVersion(tmpDir, "3.0-2")
	s.NoError(err)

	// Verify via re-parsing (library may use single or double quotes)
	proj, readErr := ReadProject(tmpDir)
	s.NoError(readErr)
	s.Equal("3.0-2", proj.RuntimeVersion)
	s.Equal("3.0.1", proj.AirflowVersion)
}

func (s *Suite) TestPinRuntimeVersion_UpdatesExisting() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	initial := "[project]\nname = \"test\"\n\n[tool.astro]\nairflow-version = \"3.0.1\"\nruntime-version = \"3.0-1\"\n"
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(initial), 0o644))

	err = PinRuntimeVersion(tmpDir, "3.0-2")
	s.NoError(err)

	proj, readErr := ReadProject(tmpDir)
	s.NoError(readErr)
	s.Equal("3.0-2", proj.RuntimeVersion)
}

func (s *Suite) TestPinRuntimeVersion_PreservesOtherSections() {
	tmpDir, err := os.MkdirTemp("", "pyproject")
	s.Require().NoError(err)
	defer os.RemoveAll(tmpDir)

	initial := `[project]
name = "test"
requires-python = ">=3.12"
dependencies = ["pandas>=2.0"]

[tool.astro]
airflow-version = "3.0.1"

[tool.ruff]
line-length = 120

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, PyProjectFile), []byte(initial), 0o644))

	err = PinRuntimeVersion(tmpDir, "3.0-2")
	s.NoError(err)

	content, err := os.ReadFile(filepath.Join(tmpDir, PyProjectFile))
	s.NoError(err)

	// runtime-version was added
	s.Contains(string(content), `runtime-version = "3.0-2"`)
	// Other sections preserved
	s.Contains(string(content), "[tool.ruff]")
	s.Contains(string(content), "line-length = 120")
	s.Contains(string(content), "[build-system]")
	s.Contains(string(content), "hatchling")
	s.Contains(string(content), `dependencies = ["pandas>=2.0"]`)
}
