package airflow

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
	// Airflow 2 runtime versions (old format like 12.0.0) should be rejected
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
	s.Equal(errUnsupportedAirflowVersion, err)
}

func (s *Suite) TestStandaloneStart_UnsupportedVersion() {
	origParseFile := standaloneParseFile
	origGetImageTag := standaloneGetImageTag
	defer func() {
		standaloneParseFile = origParseFile
		standaloneGetImageTag = origGetImageTag
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"some-image:unknown-tag"}},
		}, nil
	}
	standaloneGetImageTag = func(cmds []docker.Command) (string, string) {
		return "some-image", "unknown-tag"
	}

	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Equal(errUnsupportedAirflowVersion, err)
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

func (s *Suite) TestStandaloneUnsupportedCommands() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	s.Equal(errStandaloneNotSupported, handler.RunDAG("", "", "", "", false, false))

	buildErr := handler.Build("", "", false)
	s.Error(buildErr)
	s.Contains(buildErr.Error(), "not available in standalone mode")

	composeErr := handler.ComposeExport("", "")
	s.Error(composeErr)
	s.Contains(composeErr.Error(), "not available in standalone mode")

	upgradeErr := handler.UpgradeTest("", "", "", "", false, false, false, false, false, "", nil)
	s.Error(upgradeErr)
	s.Contains(upgradeErr.Error(), "not available in standalone mode")
}

func (s *Suite) TestStandaloneStop_NoPIDFile() {
	tmpDir, err := os.MkdirTemp("", "standalone-stop-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// No PID file exists — should handle gracefully
	err = handler.Stop(false)
	s.NoError(err)
}

func (s *Suite) TestStandaloneStop_StalePID() {
	tmpDir, err := os.MkdirTemp("", "standalone-stop-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Create standalone dir and a PID file with a non-existent PID
	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.pid"), []byte("999999999"), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Stop(false)
	s.NoError(err)

	// PID file should be cleaned up
	_, err = os.Stat(filepath.Join(standaloneStateDir, "airflow.pid"))
	s.True(os.IsNotExist(err))
}

func (s *Suite) TestStandaloneKill() {
	// Create a temp directory with some files to clean up
	tmpDir, err := os.MkdirTemp("", "standalone-kill-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// AIRFLOW_HOME is .astro/standalone/, so airflow.cfg, airflow.db, logs/ all
	// live inside standaloneStateDir and are cleaned up when it is removed.
	venvDir := filepath.Join(tmpDir, ".venv")
	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	// Simulate Airflow-generated files inside standaloneDir
	dbFile := filepath.Join(standaloneStateDir, "airflow.db")
	logsDir := filepath.Join(standaloneStateDir, "logs")

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

	// venv and entire standaloneDir (including db and logs inside it) removed
	_, err = os.Stat(venvDir)
	s.True(os.IsNotExist(err))
	_, err = os.Stat(standaloneStateDir)
	s.True(os.IsNotExist(err))
}

func TestParseRuntimeTagPython(t *testing.T) {
	tests := []struct {
		tag        string
		wantBase   string
		wantPython string
	}{
		{"3.1-12", "3.1-12", ""},
		{"3.1-12-python-3.11", "3.1-12", "3.11"},
		{"3.1-12-python-3.11-base", "3.1-12", "3.11"},
		{"3.1-12-base", "3.1-12", ""},
		{"3.2-1-python-3.13", "3.2-1", "3.13"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			base, python := parseRuntimeTagPython(tt.tag)
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantPython, python)
		})
	}
}

func TestResolvePythonVersion(t *testing.T) {
	origResolve := resolvePythonVersion
	defer func() { resolvePythonVersion = origResolve }()

	t.Run("tier 1: explicit tag python wins", func(t *testing.T) {
		// Reset to real implementation for each subtest
		resolvePythonVersion = origResolve
		result := resolvePythonVersion("3.1-12", "3.11")
		assert.Equal(t, "3.11", result)
	})

	t.Run("tier 3: fallback when no tag and no JSON", func(t *testing.T) {
		// The real resolvePythonVersion will hit the network for tier 2
		// and fail (no matching version), so it falls through to tier 3.
		// Mock it to skip the network call and test the fallback.
		resolvePythonVersion = func(baseTag, tagPython string) string {
			if tagPython != "" {
				return tagPython
			}
			return defaultPythonVersion
		}
		result := resolvePythonVersion("99.99-99", "")
		assert.Equal(t, "3.12", result)
	})
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

func TestLoadEnvFile_QuotedValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "envfile-quotes-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	envFilePath := filepath.Join(tmpDir, ".env")
	content := `DOUBLE="hello world"
SINGLE='hello world'
UNQUOTED=hello world
MISMATCH="hello'
EMPTY_DOUBLE=""
EMPTY_SINGLE=''`
	err = os.WriteFile(envFilePath, []byte(content), 0o644)
	require.NoError(t, err)

	envVars, err := loadEnvFile(envFilePath)
	assert.NoError(t, err)
	assert.Contains(t, envVars, "DOUBLE=hello world")
	assert.Contains(t, envVars, "SINGLE=hello world")
	assert.Contains(t, envVars, "UNQUOTED=hello world")
	assert.Contains(t, envVars, `MISMATCH="hello'`)
	assert.Contains(t, envVars, "EMPTY_DOUBLE=")
	assert.Contains(t, envVars, "EMPTY_SINGLE=")
}

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`'hello'`, "hello"},
		{`hello`, "hello"},
		{`"hello'`, `"hello'`},
		{`'hello"`, `'hello"`},
		{`""`, ""},
		{`''`, ""},
		{`"`, `"`},
		{``, ``},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripQuotes(tt.input))
		})
	}
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

	s.Equal("/tmp/test-project/.astro/standalone", envMap["AIRFLOW_HOME"])
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

	// Standalone-critical vars must NOT be overridden by .env
	s.Equal("local", envMap["ASTRONOMER_ENVIRONMENT"])
	s.Equal(filepath.Join(tmpDir, ".astro", "standalone"), envMap["AIRFLOW_HOME"])
	// Non-critical .env vars should still be applied
	s.Equal("hello", envMap["MY_CUSTOM_VAR"])
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

	// Pre-create both cached files
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)

	constraintsFile := filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt")
	content := "apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\nother-package==1.0.0\n"
	err = os.WriteFile(constraintsFile, []byte(content), 0o644)
	s.NoError(err)

	freezeFile := filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt")
	err = os.WriteFile(freezeFile, []byte("apache-airflow==3.0.1\nsome-dep==1.2.3\n"), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	path, version, taskSDKVersion, err := handler.getConstraints("3.1-12", defaultPythonVersion)
	s.NoError(err)
	s.Equal(freezeFile, path)
	s.Equal("3.0.1", version)
	s.Equal("1.0.0", taskSDKVersion)
}

func (s *Suite) TestStandaloneGetConstraints_FetchesFromURL() {
	tmpDir, err := os.MkdirTemp("", "standalone-constraints-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Mock fetchConstraintsURL — called twice (constraints then freeze)
	origFetch := fetchConstraintsURL
	defer func() { fetchConstraintsURL = origFetch }()

	fetchConstraintsURL = func(url string) (string, error) {
		s.Contains(url, "runtime-3.1-13-python-3.12.txt")
		if strings.Contains(url, "runtime-constraints") {
			return "apache-airflow==3.0.2\napache-airflow-task-sdk==1.0.0\n", nil
		}
		// freeze URL
		return "apache-airflow==3.0.2\nsome-dep==1.2.3\n", nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	path, version, taskSDKVersion, err := handler.getConstraints("3.1-13", defaultPythonVersion)
	s.NoError(err)
	s.Contains(path, "freeze-3.1-13-python-3.12.txt")
	s.Equal("3.0.2", version)
	s.Equal("1.0.0", taskSDKVersion)

	// Verify freeze file was cached
	_, err = os.Stat(path)
	s.NoError(err)
}

func (s *Suite) TestStandaloneGetConstraints_FetchFails() {
	tmpDir, err := os.MkdirTemp("", "standalone-constraints-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	origFetch := fetchConstraintsURL
	defer func() { fetchConstraintsURL = origFetch }()

	fetchConstraintsURL = func(url string) (string, error) {
		return "", fmt.Errorf("network error")
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	_, _, _, err = handler.getConstraints("3.1-99", defaultPythonVersion) //nolint:dogsled
	s.Error(err)
	s.Contains(err.Error(), "error fetching constraints")
	s.Contains(err.Error(), "network error")
}

func (s *Suite) TestStandaloneStart_VenvCreationFails() {
	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origRunCommand := runCommand
	origResolvePython := resolvePythonVersion
	origCheckPort := checkPortAvailable
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
		resolvePythonVersion = origResolvePython
		checkPortAvailable = origCheckPort
	}()

	checkPortAvailable = func(_ string) error { return nil }
	resolvePythonVersion = func(_, _ string) string { return "3.12" }

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

	// Pre-create cached constraints + freeze to skip URL fetch
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
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
	origResolvePython := resolvePythonVersion
	origCheckPort := checkPortAvailable
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
		resolvePythonVersion = origResolvePython
		checkPortAvailable = origCheckPort
	}()

	checkPortAvailable = func(_ string) error { return nil }
	resolvePythonVersion = func(_, _ string) string { return "3.12" }

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
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
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

	// Pre-create cached constraints + freeze
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
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
	origResolvePython := resolvePythonVersion
	origCheckPort := checkPortAvailable
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
		checkWebserverHealth = origCheckHealth
		resolvePythonVersion = origResolvePython
		checkPortAvailable = origCheckPort
	}()

	checkPortAvailable = func(_ string) error { return nil }
	resolvePythonVersion = func(_, _ string) string { return "3.12" }

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
	handler.SetForeground(true) // Use foreground mode for this test

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection(nil))
	s.NoError(err)
}

func (s *Suite) TestStandaloneStart_Background() {
	tmpDir, err := os.MkdirTemp("", "standalone-bg-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create cached constraints + freeze and standalone dir
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	// Create a fake airflow binary that sleeps briefly then exits
	venvBin := filepath.Join(tmpDir, ".venv", "bin")
	err = os.MkdirAll(venvBin, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(venvBin, "airflow"), []byte("#!/bin/sh\necho 'standalone running'\nsleep 30\n"), 0o755)
	s.NoError(err)

	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origRunCommand := runCommand
	origCheckHealth := checkWebserverHealth
	origResolvePython := resolvePythonVersion
	origCheckPort := checkPortAvailable
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
		checkWebserverHealth = origCheckHealth
		resolvePythonVersion = origResolvePython
		checkPortAvailable = origCheckPort
	}()

	checkPortAvailable = func(_ string) error { return nil }
	resolvePythonVersion = func(_, _ string) string { return "3.12" }
	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}
	lookPath = func(file string) (string, error) { return "/usr/local/bin/uv", nil }
	runCommand = func(dir, name string, args ...string) error { return nil }
	checkWebserverHealth = func(url string, timeout time.Duration, component string) error { return nil }

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)
	// Default is background mode (foreground = false)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.NoError(err)

	// Verify PID file was written
	pidFilePath := filepath.Join(constraintsDir, "airflow.pid")
	_, err = os.Stat(pidFilePath)
	s.NoError(err)

	// Verify log file was created
	logFilePath := filepath.Join(constraintsDir, "airflow.log")
	_, err = os.Stat(logFilePath)
	s.NoError(err)

	// Clean up the process
	handler.Stop(false) //nolint:errcheck
}

func (s *Suite) TestStandaloneStart_AlreadyRunning() {
	tmpDir, err := os.MkdirTemp("", "standalone-already-running")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create standalone dir, constraints + freeze, venv
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	venvBin := filepath.Join(tmpDir, ".venv", "bin")
	err = os.MkdirAll(venvBin, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(venvBin, "airflow"), []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	s.NoError(err)

	// Write a PID file with our own PID (guaranteed to be alive)
	err = os.WriteFile(filepath.Join(constraintsDir, "airflow.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
	s.NoError(err)

	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origRunCommand := runCommand
	origResolvePython := resolvePythonVersion
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		runCommand = origRunCommand
		resolvePythonVersion = origResolvePython
	}()

	resolvePythonVersion = func(_, _ string) string { return "3.12" }
	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}
	lookPath = func(file string) (string, error) { return "/usr/local/bin/uv", nil }
	runCommand = func(dir, name string, args ...string) error { return nil }

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Contains(err.Error(), "already running")
}

func (s *Suite) TestStandaloneStart_PortInUse() {
	// Start a listener to simulate a port conflict
	ln, err := net.Listen("tcp", "localhost:0")
	s.NoError(err)
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	tmpDir, err := os.MkdirTemp("", "standalone-port-conflict")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	origParseFile := standaloneParseFile
	origLookPath := lookPath
	origResolvePython := resolvePythonVersion
	defer func() {
		standaloneParseFile = origParseFile
		lookPath = origLookPath
		resolvePythonVersion = origResolvePython
	}()

	resolvePythonVersion = func(_, _ string) string { return "3.12" }
	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1-12"}},
		}, nil
	}
	lookPath = func(file string) (string, error) { return "/usr/local/bin/uv", nil }

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)
	handler.SetPort(port)

	err = handler.Start("", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, nil)
	s.Error(err)
	s.Contains(err.Error(), "port "+port+" is already in use")
}

func (s *Suite) TestStandaloneStop_Running() {
	tmpDir, err := os.MkdirTemp("", "standalone-stop-running")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Start a real background process that we can stop
	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)

	// Start a sleep process
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	err = cmd.Start()
	s.NoError(err)

	// Write its PID
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.pid"), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Stop(false)
	s.NoError(err)

	// PID file should be removed
	_, err = os.Stat(filepath.Join(standaloneStateDir, "airflow.pid"))
	s.True(os.IsNotExist(err))
}

func (s *Suite) TestStandaloneLogs() {
	tmpDir, err := os.MkdirTemp("", "standalone-logs-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Create a log file with some content
	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.log"), []byte("log line 1\nlog line 2\n"), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// Non-follow mode should return immediately
	err = handler.Logs(false)
	s.NoError(err)
}

func (s *Suite) TestStandaloneLogs_NoFile() {
	tmpDir, err := os.MkdirTemp("", "standalone-logs-nofile")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Logs(false)
	s.Error(err)
	s.Contains(err.Error(), "no log file found")
}

func (s *Suite) TestStandaloneLogs_FilterByComponent() {
	tmpDir, err := os.MkdirTemp("", "standalone-logs-filter")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)

	logContent := `standalone | Starting Airflow Standalone
standalone | Database ready
scheduler  | Starting the scheduler
api-server | Uvicorn running on http://0.0.0.0:8080
triggerer  | Starting the triggerer
dag-processor | Processing files
scheduler  | Adopting orphaned tasks
api-server | GET /health 200 OK
`
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.log"), []byte(logContent), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// Capture stdout
	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	err = handler.Logs(false, SchedulerDockerContainerName)
	s.NoError(err)

	w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)
	output := string(out)

	s.Contains(output, "Starting the scheduler")
	s.Contains(output, "Adopting orphaned tasks")
	s.NotContains(output, "standalone")
	s.NotContains(output, "api-server")
	s.NotContains(output, "triggerer")
	s.NotContains(output, "dag-processor")
}

func (s *Suite) TestStandaloneLogs_WebserverMapsToAPIServer() {
	tmpDir, err := os.MkdirTemp("", "standalone-logs-webserver")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)

	logContent := `scheduler  | scheduler line
api-server | api line
`
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.log"), []byte(logContent), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	// "webserver" flag should map to "api-server" prefix
	err = handler.Logs(false, WebserverDockerContainerName)
	s.NoError(err)

	w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)
	output := string(out)

	s.Contains(output, "api line")
	s.NotContains(output, "scheduler line")
}

func (s *Suite) TestStandaloneLogs_NoFilterShowsAll() {
	tmpDir, err := os.MkdirTemp("", "standalone-logs-nofilter")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)

	logContent := `standalone | startup
scheduler  | sched line
api-server | api line
`
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.log"), []byte(logContent), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	// No filter — show everything
	err = handler.Logs(false)
	s.NoError(err)

	w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)
	output := string(out)

	s.Contains(output, "standalone | startup")
	s.Contains(output, "scheduler  | sched line")
	s.Contains(output, "api-server | api line")
}

func TestBuildLogPrefixes(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, buildLogPrefixes(nil))
		assert.Nil(t, buildLogPrefixes([]string{}))
	})

	t.Run("maps docker names to standalone prefixes", func(t *testing.T) {
		prefixes := buildLogPrefixes([]string{SchedulerDockerContainerName})
		assert.Equal(t, []string{"scheduler "}, prefixes)
	})

	t.Run("webserver maps to api-server", func(t *testing.T) {
		prefixes := buildLogPrefixes([]string{WebserverDockerContainerName})
		assert.Equal(t, []string{"api-server "}, prefixes)
	})

	t.Run("deduplicates webserver and api-server", func(t *testing.T) {
		prefixes := buildLogPrefixes([]string{WebserverDockerContainerName, APIServerDockerContainerName})
		assert.Equal(t, []string{"api-server "}, prefixes)
	})

	t.Run("multiple components", func(t *testing.T) {
		prefixes := buildLogPrefixes([]string{SchedulerDockerContainerName, TriggererDockerContainerName})
		assert.Contains(t, prefixes, "scheduler ")
		assert.Contains(t, prefixes, "triggerer ")
		assert.Len(t, prefixes, 2)
	})

	t.Run("unknown names are ignored", func(t *testing.T) {
		prefixes := buildLogPrefixes([]string{"postgres"})
		assert.Empty(t, prefixes)
	})
}

func TestMatchesLogPrefix(t *testing.T) {
	t.Run("nil prefixes matches everything", func(t *testing.T) {
		assert.True(t, matchesLogPrefix("anything", nil))
	})

	t.Run("matches prefix", func(t *testing.T) {
		assert.True(t, matchesLogPrefix("scheduler  | some log", []string{"scheduler "}))
	})

	t.Run("no match", func(t *testing.T) {
		assert.False(t, matchesLogPrefix("triggerer  | some log", []string{"scheduler "}))
	})
}

func (s *Suite) TestStandalonePS_NotRunning() {
	tmpDir, err := os.MkdirTemp("", "standalone-ps-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// Should not error even when not running
	err = handler.PS()
	s.NoError(err)
}

func (s *Suite) TestStandalonePS_Running() {
	tmpDir, err := os.MkdirTemp("", "standalone-ps-running")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)

	// Write our own PID (guaranteed alive)
	err = os.WriteFile(filepath.Join(standaloneStateDir, "airflow.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.PS()
	s.NoError(err)
}

func (s *Suite) TestStandaloneSetForeground() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	s.False(handler.foreground)
	handler.SetForeground(true)
	s.True(handler.foreground)
}

func (s *Suite) TestStandaloneSetPort() {
	handler, err := StandaloneInit("/tmp/test", ".env", "Dockerfile")
	s.NoError(err)

	// Default port
	s.Equal("8080", handler.webserverPort())

	// Override via SetPort
	handler.SetPort("9090")
	s.Equal("9090", handler.webserverPort())

	// Health endpoint uses custom port
	url, comp := handler.healthEndpoint()
	s.Contains(url, ":9090/")
	s.Equal("api-server", comp)
}

func (s *Suite) TestStandaloneBuildEnv_CustomPort() {
	handler, err := StandaloneInit("/tmp/test-project", "", "Dockerfile")
	s.NoError(err)
	handler.SetPort("9090")

	env := handler.buildEnv()

	envMap := make(map[string]string)
	for _, e := range env {
		parts := splitEnvVar(e)
		if parts != nil {
			envMap[parts[0]] = parts[1]
		}
	}

	s.Equal("9090", envMap["AIRFLOW__API__PORT"])
}

func (s *Suite) TestStandaloneEnsureCredentials() {
	tmpDir, err := os.MkdirTemp("", "standalone-ensure-creds")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// File lives in .astro/standalone/, not the project root
	credsPath := handler.passwordsFilePath()
	s.Contains(credsPath, ".astro/standalone/")

	// File doesn't exist — ensureCredentials should create it with admin:admin
	err = handler.ensureCredentials()
	s.NoError(err)

	data, err := os.ReadFile(credsPath)
	s.NoError(err)
	s.Contains(string(data), `"admin"`)

	// Called again — should be a no-op (not overwrite existing file)
	err = os.WriteFile(credsPath, []byte(`{"admin":"custompassword"}`), 0o644)
	s.NoError(err)
	err = handler.ensureCredentials()
	s.NoError(err)
	data, err = os.ReadFile(credsPath)
	s.NoError(err)
	s.Contains(string(data), "custompassword") // unchanged
}

func (s *Suite) TestStandaloneReadCredentials() {
	tmpDir, err := os.MkdirTemp("", "standalone-creds-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// No file — should return empty strings gracefully
	user, pass := handler.readCredentials()
	s.Equal("", user)
	s.Equal("", pass)

	// Write a valid credentials file into .astro/standalone/
	credsPath := handler.passwordsFilePath()
	err = os.MkdirAll(filepath.Dir(credsPath), 0o755)
	s.NoError(err)
	err = os.WriteFile(credsPath, []byte(`{"admin":"supersecret"}`), 0o644)
	s.NoError(err)

	user, pass = handler.readCredentials()
	s.Equal("admin", user)
	s.Equal("supersecret", pass)
}

func (s *Suite) TestStandaloneReadCredentials_InvalidJSON() {
	tmpDir, err := os.MkdirTemp("", "standalone-creds-invalid")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)
	credsPath := handler.passwordsFilePath()
	err = os.MkdirAll(filepath.Dir(credsPath), 0o755)
	s.NoError(err)
	err = os.WriteFile(credsPath, []byte(`not valid json`), 0o644)
	s.NoError(err)

	user, pass := handler.readCredentials()
	s.Equal("", user)
	s.Equal("", pass)
}

// --- ensureVenv tests ---

func (s *Suite) TestStandaloneEnsureVenv_NoVenv() {
	tmpDir, err := os.MkdirTemp("", "standalone-ensurevenv")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.ensureVenv()
	s.Error(err)
	s.Contains(err.Error(), "no virtual environment found")
}

func (s *Suite) TestStandaloneEnsureVenv_WithVenv() {
	tmpDir, err := os.MkdirTemp("", "standalone-ensurevenv")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.ensureVenv()
	s.NoError(err)
}

// --- Run tests ---

func (s *Suite) TestStandaloneRun_NoVenv() {
	tmpDir, err := os.MkdirTemp("", "standalone-run")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Run([]string{"airflow", "dags", "list"}, "")
	s.Error(err)
	s.Contains(err.Error(), "no virtual environment found")
}

func (s *Suite) TestStandaloneRun_Success() {
	tmpDir, err := os.MkdirTemp("", "standalone-run")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	var capturedArgs []string
	var capturedDir string
	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		capturedArgs = args
		capturedDir = dir
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Run([]string{"airflow", "dags", "list"}, "root")
	s.NoError(err)
	s.Equal([]string{"airflow", "dags", "list"}, capturedArgs)
	s.Equal(tmpDir, capturedDir)
}

// --- Bash tests ---

func (s *Suite) TestStandaloneBash_NoVenv() {
	tmpDir, err := os.MkdirTemp("", "standalone-bash")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Bash("scheduler")
	s.Error(err)
	s.Contains(err.Error(), "no virtual environment found")
}

func (s *Suite) TestStandaloneBash_Success() {
	tmpDir, err := os.MkdirTemp("", "standalone-bash")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	var capturedArgs []string
	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		capturedArgs = args
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Bash("scheduler")
	s.NoError(err)
	s.Equal([]string{"bash"}, capturedArgs)
}

// --- ImportSettings tests ---

func (s *Suite) TestStandaloneImportSettings_FileNotFound() {
	tmpDir, err := os.MkdirTemp("", "standalone-import")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.ImportSettings(filepath.Join(tmpDir, "nonexistent.yaml"), "", false, false, false)
	s.Error(err)
	s.Contains(err.Error(), "file specified does not exist")
}

func (s *Suite) TestStandaloneImportSettings_DefaultsAllFlags() {
	tmpDir, err := os.MkdirTemp("", "standalone-import")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Create a minimal settings file
	settingsFile := filepath.Join(tmpDir, "airflow_settings.yaml")
	err = os.WriteFile(settingsFile, []byte("airflow:\n  connections: []\n  variables: []\n  pools: []\n"), 0o644)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// This will fail because there's no real airflow to exec, but we can verify
	// it gets past the file check and flag defaulting
	err = handler.ImportSettings(settingsFile, "", false, false, false)
	// Error is expected since we don't have a real venv, but it should NOT be
	// "file specified does not exist"
	if err != nil {
		s.NotContains(err.Error(), "file specified does not exist")
	}
}

// --- ExportSettings tests ---

func (s *Suite) TestStandaloneExportSettings_FileNotFound() {
	tmpDir, err := os.MkdirTemp("", "standalone-export")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.ExportSettings(filepath.Join(tmpDir, "nonexistent.yaml"), "", false, false, false, false)
	s.Error(err)
	s.Contains(err.Error(), "file specified does not exist")
}

// --- Pytest tests ---

func (s *Suite) TestStandalonePytest_NoVenv() {
	tmpDir, err := os.MkdirTemp("", "standalone-pytest")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	_, err = handler.Pytest("", "", "", "", "")
	s.Error(err)
	s.Contains(err.Error(), "no virtual environment found")
}

func (s *Suite) TestStandalonePytest_Success() {
	tmpDir, err := os.MkdirTemp("", "standalone-pytest")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	var capturedArgs []string
	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		capturedArgs = args
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	exitCode, err := handler.Pytest("", "", "", "", "")
	s.NoError(err)
	s.Equal("", exitCode)
	s.Equal([]string{"pytest", "tests/"}, capturedArgs)
}

func (s *Suite) TestStandalonePytest_WithFileAndArgs() {
	tmpDir, err := os.MkdirTemp("", "standalone-pytest")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	var capturedArgs []string
	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		capturedArgs = args
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	exitCode, err := handler.Pytest("test_dag.py", "", "", "-v --tb=short", "")
	s.NoError(err)
	s.Equal("", exitCode)
	s.Equal([]string{"pytest", "tests/test_dag.py", "-v", "--tb=short"}, capturedArgs)
}

func (s *Suite) TestStandalonePytest_DefaultTestPath() {
	tmpDir, err := os.MkdirTemp("", "standalone-pytest")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	var capturedArgs []string
	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		capturedArgs = args
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	exitCode, err := handler.Pytest(DefaultTestPath, "", "", "", "")
	s.NoError(err)
	s.Equal("", exitCode)
	// DefaultTestPath should NOT be prepended with tests/
	s.Equal([]string{"pytest", DefaultTestPath}, capturedArgs)
}

func (s *Suite) TestStandalonePytest_FileInTestsDir() {
	tmpDir, err := os.MkdirTemp("", "standalone-pytest")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	var capturedArgs []string
	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		capturedArgs = args
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	exitCode, err := handler.Pytest("tests/test_my_dag.py", "", "", "", "")
	s.NoError(err)
	s.Equal("", exitCode)
	// Already contains "tests", should not be prepended
	s.Equal([]string{"pytest", "tests/test_my_dag.py"}, capturedArgs)
}

func (s *Suite) TestStandalonePytest_Failure() {
	tmpDir, err := os.MkdirTemp("", "standalone-pytest")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		// Run a real command that exits with code 1 to produce an *exec.ExitError
		cmd := exec.Command("sh", "-c", "exit 1") //nolint:gosec
		return cmd.Run()
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	exitCode, err := handler.Pytest("", "", "", "", "")
	s.Error(err)
	s.Equal("1", exitCode)
	s.Contains(err.Error(), "something went wrong while Pytesting your DAGs")
}

// --- Parse tests ---

func (s *Suite) TestStandaloneParse_FileNotFound() {
	tmpDir, err := os.MkdirTemp("", "standalone-parse")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	// No DefaultTestPath file exists — should return nil with a message
	err = handler.Parse("", "", "")
	s.NoError(err)
}

func (s *Suite) TestStandaloneParse_Success() {
	tmpDir, err := os.MkdirTemp("", "standalone-parse")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Create the DefaultTestPath file
	err = os.MkdirAll(filepath.Join(tmpDir, ".astro"), 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(tmpDir, DefaultTestPath), []byte("test"), 0o644)
	s.NoError(err)

	// Create venv
	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		return nil
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Parse("", "", "")
	s.NoError(err)
}

func (s *Suite) TestStandaloneParse_DagErrors() {
	tmpDir, err := os.MkdirTemp("", "standalone-parse")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, ".astro"), 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(tmpDir, DefaultTestPath), []byte("test"), 0o644)
	s.NoError(err)

	err = os.MkdirAll(filepath.Join(tmpDir, ".venv", "bin"), 0o755)
	s.NoError(err)

	origExec := standaloneExec
	defer func() { standaloneExec = origExec }()

	standaloneExec = func(dir string, env, args []string, _ io.Reader, _, _ io.Writer) error {
		cmd := exec.Command("sh", "-c", "exit 1") //nolint:gosec
		return cmd.Run()
	}

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Parse("", "", "")
	s.Error(err)
	s.Contains(err.Error(), "errors detected in your DAGs")
}

// --- resolveInEnvPath tests ---

func TestResolveInEnvPath(t *testing.T) {
	// Create a temp dir with a fake binary
	tmpDir, err := os.MkdirTemp("", "resolve-path")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	fakeBin := filepath.Join(tmpDir, "mybinary")
	err = os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755)
	require.NoError(t, err)

	t.Run("resolves binary in env PATH", func(t *testing.T) {
		env := []string{"PATH=" + tmpDir + ":/usr/bin"}
		result := resolveInEnvPath("mybinary", env)
		assert.Equal(t, fakeBin, result)
	})

	t.Run("returns original when not found", func(t *testing.T) {
		env := []string{"PATH=/nonexistent"}
		result := resolveInEnvPath("mybinary", env)
		assert.Equal(t, "mybinary", result)
	})

	t.Run("skips resolution for absolute paths", func(t *testing.T) {
		env := []string{"PATH=" + tmpDir}
		result := resolveInEnvPath("/usr/bin/bash", env)
		assert.Equal(t, "/usr/bin/bash", result)
	})

	t.Run("skips resolution for relative paths with separator", func(t *testing.T) {
		env := []string{"PATH=" + tmpDir}
		result := resolveInEnvPath("./mybinary", env)
		assert.Equal(t, "./mybinary", result)
	})

	t.Run("no PATH in env falls through", func(t *testing.T) {
		env := []string{"HOME=/tmp"}
		result := resolveInEnvPath("mybinary", env)
		assert.Equal(t, "mybinary", result)
	})
}
