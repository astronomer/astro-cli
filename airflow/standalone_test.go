package airflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *Suite) TestStandaloneInit() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)
	s.NotNil(handler)
	s.Equal("/tmp/test", handler.airflowHome)
	s.Equal(".env", handler.envFile)
	s.Equal("Dockerfile", handler.dockerfile)
}

func (s *Suite) TestStandaloneHandlerInit() {
	handler, err := StandaloneHandlerInit("/tmp/test", ".env", "Dockerfile", "project")
	s.NoError(err)
	s.NotNil(handler)
}

func (s *Suite) TestStandaloneStart_Airflow2Rejected() {
	// Mock parseFile to return an Airflow 2 runtime image
	origParseFile := standaloneParseFile
	defer func() { standaloneParseFile = origParseFile }()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"quay.io/astronomer/astro-runtime:12.0.0"}},
		}, nil
	}

	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Equal(errNotAirflow3, err)
}

func (s *Suite) TestStandaloneStart_MissingUV() {
	// Mock parseFile to return an Airflow 3 runtime image
	origParseFile := standaloneParseFile
	origLookPath := lookPath
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}

	lookPath = func(file string) (string, error) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}

	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Equal(errUVNotFound, err)
}

func (s *Suite) TestStandaloneStart_DockerfileParseError() {
	origParseFile := standaloneParseFile
	defer func() { standaloneParseFile = origParseFile }()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return nil, docker.IOError{Msg: "file not found"}
	}

	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Contains(err.Error(), "error parsing Dockerfile")
}

func (s *Suite) TestStandaloneStart_EmptyTag() {
	origParseFile := standaloneParseFile
	defer func() { standaloneParseFile = origParseFile }()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "run", Value: []string{"echo hello"}},
		}, nil
	}

	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Contains(err.Error(), "could not determine runtime version")
}

func (s *Suite) TestStandaloneStubMethods() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	s.Equal(errStandaloneNotSupported, handler.PS())
	s.Equal(errStandaloneNotSupported, handler.Logs(false))
	s.Equal(errStandaloneNotSupported, handler.Run(nil, ""))
	s.Equal(errStandaloneNotSupported, handler.Bash(""))
	s.Equal(errStandaloneNotSupported, handler.RunDAG("", "", "", "", false, false))
	s.Equal(errStandaloneNotSupported, handler.ImportSettings("", "", false, false, false))
	s.Equal(errStandaloneNotSupported, handler.ExportSettings("", "", false, false, false, false))
	s.Equal(errStandaloneNotSupported, handler.ComposeExport("", ""))

	_, pytestErr := handler.Pytest("", "", "", "", "")
	s.Equal(errStandaloneNotSupported, pytestErr)

	s.Equal(errStandaloneNotSupported, handler.Parse("", "", ""))
	s.Equal(errStandaloneNotSupported, handler.UpgradeTest("", "", "", "", false, false, false, false, false, "", nil))
}

func (s *Suite) TestStandaloneStop() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Stop(false)
	s.NoError(err)
}

func (s *Suite) TestStandaloneKill() {
	// Create a temp directory with some files to clean up
	tmpDir, err := os.MkdirTemp("", "standalone-kill-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Create files that Kill should remove
	venvDir := filepath.Join(tmpDir, ".venv")
	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	dbFile := filepath.Join(tmpDir, "airflow.db")
	logsDir := filepath.Join(tmpDir, "logs")

	err = os.MkdirAll(venvDir, 0o755)
	s.NoError(err)
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(dbFile, []byte("test"), 0o644)
	s.NoError(err)
	err = os.MkdirAll(logsDir, 0o755)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Kill()
	s.NoError(err)

	// Verify files were removed
	_, err = os.Stat(venvDir)
	s.True(os.IsNotExist(err))
	_, err = os.Stat(standaloneStateDir)
	s.True(os.IsNotExist(err))
	_, err = os.Stat(dbFile)
	s.True(os.IsNotExist(err))
	_, err = os.Stat(logsDir)
	s.True(os.IsNotExist(err))
}

func TestParseAirflowVersionFromConstraints(t *testing.T) {
	// Create a temp file with constraints
	tmpDir, err := os.MkdirTemp("", "constraints-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	constraintsFile := filepath.Join(tmpDir, "constraints.txt")
	content := `something-else==1.0.0
apache-airflow==3.0.0
another-package==2.0.0`
	err = os.WriteFile(constraintsFile, []byte(content), 0o644)
	require.NoError(t, err)

	version, err := parseAirflowVersionFromConstraints(constraintsFile)
	assert.NoError(t, err)
	assert.Equal(t, "3.0.0", version)
}

func TestParseAirflowVersionFromConstraints_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "constraints-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	constraintsFile := filepath.Join(tmpDir, "constraints.txt")
	content := `something-else==1.0.0
another-package==2.0.0`
	err = os.WriteFile(constraintsFile, []byte(content), 0o644)
	require.NoError(t, err)

	_, err = parseAirflowVersionFromConstraints(constraintsFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find apache-airflow version")
}

func TestLoadEnvFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "envfile-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	envFilePath := filepath.Join(tmpDir, ".env")
	content := `# Comment
FOO=bar
BAZ=qux

# Another comment
EMPTY=`
	err = os.WriteFile(envFilePath, []byte(content), 0o644)
	require.NoError(t, err)

	envVars, err := loadEnvFile(envFilePath)
	assert.NoError(t, err)
	assert.Contains(t, envVars, "FOO=bar")
	assert.Contains(t, envVars, "BAZ=qux")
	assert.Contains(t, envVars, "EMPTY=")
	assert.Len(t, envVars, 3)
}

func TestLoadEnvFile_NotFound(t *testing.T) {
	_, err := loadEnvFile("/nonexistent/.env")
	assert.Error(t, err)
}

func TestLoadEnvFile_ValueWithEquals(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "envfile-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	envFilePath := filepath.Join(tmpDir, ".env")
	content := `DB_URL=postgres://user:pass@host:5432/db?sslmode=require`
	err = os.WriteFile(envFilePath, []byte(content), 0o644)
	require.NoError(t, err)

	envVars, err := loadEnvFile(envFilePath)
	assert.NoError(t, err)
	assert.Len(t, envVars, 1)
	assert.Equal(t, "DB_URL=postgres://user:pass@host:5432/db?sslmode=require", envVars[0])
}

func (s *Suite) TestStandaloneBuildEnv() {
	handler, err := StandaloneInit("/tmp/test-project", "", "Dockerfile")
	s.NoError(err)

	env := handler.buildEnv()

	// Check that key env vars are present
	envMap := make(map[string]string)
	for _, e := range env {
		parts := splitEnvVar(e)
		if parts != nil {
			envMap[parts[0]] = parts[1]
		}
	}

	s.Equal("/tmp/test-project", envMap["AIRFLOW_HOME"])
	s.Equal("local", envMap["ASTRONOMER_ENVIRONMENT"])
	s.Equal("False", envMap["AIRFLOW__CORE__LOAD_EXAMPLES"])
	s.Equal("/tmp/test-project/dags", envMap["AIRFLOW__CORE__DAGS_FOLDER"])
	s.Contains(envMap["PATH"], "/tmp/test-project/.venv/bin")
}

func (s *Suite) TestStandaloneBuildEnv_NoDuplicateKeys() {
	handler, err := StandaloneInit("/tmp/test-project", "", "Dockerfile")
	s.NoError(err)

	env := handler.buildEnv()

	// Count occurrences of each key — should all be exactly 1
	keyCounts := make(map[string]int)
	for _, e := range env {
		parts := splitEnvVar(e)
		if parts != nil {
			keyCounts[parts[0]]++
		}
	}

	for key, count := range keyCounts {
		s.Equalf(1, count, "env var %q appears %d times, expected exactly 1", key, count)
	}
}

func (s *Suite) TestStandaloneBuildEnv_WithEnvFile() {
	tmpDir, err := os.MkdirTemp("", "standalone-buildenv-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Write a .env file with an override and a custom var
	envContent := "ASTRONOMER_ENVIRONMENT=custom\nMY_CUSTOM_VAR=hello\n"
	err = os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, "", "Dockerfile")
	s.NoError(err)

	env := handler.buildEnv()

	envMap := make(map[string]string)
	for _, e := range env {
		parts := splitEnvVar(e)
		if parts != nil {
			envMap[parts[0]] = parts[1]
		}
	}

	// .env should override our defaults
	s.Equal("custom", envMap["ASTRONOMER_ENVIRONMENT"])
	s.Equal("hello", envMap["MY_CUSTOM_VAR"])
	// Other defaults should still be present
	s.Equal(tmpDir, envMap["AIRFLOW_HOME"])
}

func splitEnvVar(s string) []string {
	idx := indexOf(s, '=')
	if idx < 0 {
		return nil
	}
	return []string{s[:idx], s[idx+1:]}
}

func indexOf(s string, c byte) int {
	for i := range len(s) {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func (s *Suite) TestStandaloneGetConstraints_Cached() {
	tmpDir, err := os.MkdirTemp("", "standalone-constraints-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create cached constraints
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)

	constraintsFile := filepath.Join(constraintsDir, "constraints-3.1-12.txt")
	content := "apache-airflow==3.0.1\nother-package==1.0.0\n"
	err = os.WriteFile(constraintsFile, []byte(content), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	path, version, err := handler.getConstraints("3.1-12")
	s.NoError(err)
	s.Equal(constraintsFile, path)
	s.Equal("3.0.1", version)
}

func (s *Suite) TestStandaloneGetConstraints_FetchesFromDocker() {
	tmpDir, err := os.MkdirTemp("", "standalone-constraints-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Mock execDockerRun
	origExecDockerRun := execDockerRun
	defer func() { execDockerRun = origExecDockerRun }()

	execDockerRun = func(imageName, filePath string) (string, error) {
		return "apache-airflow==3.0.2\nother-package==2.0.0\n", nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	path, version, err := handler.getConstraints("3.1-13")
	s.NoError(err)
	s.Contains(path, "constraints-3.1-13.txt")
	s.Equal("3.0.2", version)

	// Verify file was cached
	_, err = os.Stat(path)
	s.NoError(err)
}

func (s *Suite) TestStandaloneGetConstraints_DockerRunFails() {
	tmpDir, err := os.MkdirTemp("", "standalone-constraints-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	origExecDockerRun := execDockerRun
	defer func() { execDockerRun = origExecDockerRun }()

	execDockerRun = func(imageName, filePath string) (string, error) {
		return "", fmt.Errorf("docker not running")
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	_, _, err = handler.getConstraints("3.1-99")
	s.Error(err)
	s.Contains(err.Error(), "error extracting constraints")
	s.Contains(err.Error(), "docker not running")
}

func (s *Suite) TestStandaloneStart_VenvCreationFails() {
	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origRunCommand := runCommand
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/uv", nil
	}

	callCount := 0
	runCommand = func(dir, name string, args ...string) error {
		callCount++
		if callCount == 1 {
			// First call is "uv venv" — fail it
			return fmt.Errorf("uv venv failed: python 3.12 not found")
		}
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "standalone-venv-fail")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create constraints to skip Docker
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Contains(err.Error(), "error creating virtual environment")
}

func (s *Suite) TestStandaloneStart_InstallFails() {
	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origRunCommand := runCommand
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/uv", nil
	}

	callCount := 0
	runCommand = func(dir, name string, args ...string) error {
		callCount++
		if callCount == 2 {
			// Second call is "uv pip install" — fail it
			return fmt.Errorf("pip install failed: network error")
		}
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "standalone-install-fail")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Contains(err.Error(), "error installing dependencies")
}

func (s *Suite) TestStandaloneImplementsContainerHandler() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	// Verify that Standalone implements ContainerHandler
	var _ ContainerHandler = handler
}

func (s *Suite) TestStandaloneStart_HappyPath() {
	tmpDir, err := os.MkdirTemp("", "standalone-happy-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create cached constraints
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	constraintsFile := filepath.Join(constraintsDir, "constraints-3.1-12.txt")
	err = os.WriteFile(constraintsFile, []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	// Create a fake airflow binary that exits immediately
	venvBin := filepath.Join(tmpDir, ".venv", "bin")
	err = os.MkdirAll(venvBin, 0o755)
	s.NoError(err)
	airflowScript := filepath.Join(venvBin, "airflow")
	err = os.WriteFile(airflowScript, []byte("#!/bin/sh\necho 'standalone started'\nexit 0\n"), 0o755)
	s.NoError(err)

	// Mock all function variables
	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origRunCommand := runCommand
	origCheckHealth := checkWebserverHealth
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
		checkWebserverHealth = origCheckHealth
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/uv", nil
	}

	runCommand = func(dir, name string, args ...string) error {
		return nil
	}

	checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection(nil))
	s.NoError(err)
}
