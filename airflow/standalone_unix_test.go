//go:build !windows

package airflow

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/pkg/airflowrt"

	"github.com/astronomer/astro-cli/docker"
)

// Tests in this file use Unix shell scripts (#!/bin/sh) as fake binaries
// or Unix-specific PATH separators and cannot run on Windows.

func (s *Suite) TestStandaloneStart_Airflow2Accepted() {
	// Airflow 2 runtime versions (old format like 12.0.0) should be accepted
	tmpDir, err := os.MkdirTemp("", "standalone-af2-test")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create cached constraints + freeze for AF2 tag
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-12.0.0-python-3.12.txt"), []byte("apache-airflow==2.7.3+astro.1\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-12.0.0-python-3.12.txt"), []byte("apache-airflow==2.7.3+astro.1\n"), 0o644)
	s.NoError(err)

	// Create fake airflow and python binaries (python needed for macOS shim path)
	venvBin := filepath.Join(tmpDir, ".venv", venvBinDir())
	err = os.MkdirAll(venvBin, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(venvBin, "airflow"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(venvBin, "python"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	s.NoError(err)

	// Pre-create site-packages so writeDarwinForkSafetyPatch can install the fork-safety patch.
	// Without this the patch fails and Start() returns a hard error on macOS.
	sitePackages := filepath.Join(tmpDir, ".venv", "lib", "python3.12", "site-packages")
	err = os.MkdirAll(sitePackages, 0o755)
	s.NoError(err)

	origParseFile := standaloneParseFile
	origResolve := resolveFloatingTag
	origLookPath := lookPath
	origRunCommand := runCommand
	origCheckHealth := checkWebserverHealth
	origResolvePython := resolvePythonVersion
	origCheckPort := checkPortAvailable
	defer func() {
		standaloneParseFile = origParseFile
		resolveFloatingTag = origResolve
		lookPath = origLookPath
		runCommand = origRunCommand
		checkWebserverHealth = origCheckHealth
		resolvePythonVersion = origResolvePython
		checkPortAvailable = origCheckPort
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"quay.io/astronomer/astro-runtime:12.0.0"}},
		}, nil
	}
	resolveFloatingTag = func(tag string) (string, error) {
		return "", fmt.Errorf("no runtime version found matching '%s'", tag)
	}
	lookPath = func(file string) (string, error) { return "/usr/local/bin/uv", nil }
	runCommand = func(dir, name string, args ...string) error { return nil }
	checkWebserverHealth = func(url string, timeout time.Duration, component string) error { return nil }
	resolvePythonVersion = func(_, _ string) string { return "3.12" }
	checkPortAvailable = func(_ string) error { return nil }

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start(&types.StartOptions{SettingsFile: "airflow_settings.yaml", WaitTime: 1 * time.Minute})
	s.NoError(err)
	s.Equal("2", handler.airflowMajorVersion)

	// On macOS the shim and fork-safety patch should have been written
	if runtime.GOOS == "darwin" {
		shimPath := filepath.Join(venvBin, "_standalone_macos.py")
		_, statErr := os.Stat(shimPath)
		s.NoError(statErr, "macOS shim should be written for AF2")

		_, statErr = os.Stat(filepath.Join(sitePackages, "_fix_setproctitle.py"))
		s.NoError(statErr, "fork-safety patch _fix_setproctitle.py should be installed in site-packages")

		_, statErr = os.Stat(filepath.Join(sitePackages, "_fix_setproctitle.pth"))
		s.NoError(statErr, "fork-safety patch _fix_setproctitle.pth should be installed in site-packages")
	}
}

func (s *Suite) TestStandaloneStart_FloatingTag() {
	origParseFile := standaloneParseFile
	origGetImageTag := standaloneGetImageTag
	origLookPath := lookPath
	origResolve := resolveFloatingTag
	origResolvePython := resolvePythonVersion
	origRunCommand := runCommand
	origCheckHealth := checkWebserverHealth
	origCheckPort := checkPortAvailable
	defer func() {
		standaloneParseFile = origParseFile
		standaloneGetImageTag = origGetImageTag
		lookPath = origLookPath
		resolveFloatingTag = origResolve
		resolvePythonVersion = origResolvePython
		runCommand = origRunCommand
		checkWebserverHealth = origCheckHealth
		checkPortAvailable = origCheckPort
	}()

	standaloneParseFile = func(filename string) ([]docker.Command, error) {
		return []docker.Command{
			{Cmd: "from", Value: []string{"astrocrpublic.azurecr.io/runtime:3.1"}},
		}, nil
	}
	standaloneGetImageTag = func(cmds []docker.Command) (string, string) {
		return "astrocrpublic.azurecr.io/runtime", "3.1"
	}
	// "3.1" is a floating tag — resolve it to "3.1-12"
	resolveFloatingTag = func(tag string) (string, error) {
		if tag == "3.1" {
			return "3.1-12", nil
		}
		return "", fmt.Errorf("no match")
	}
	resolvePythonVersion = func(_, _ string) string { return "3.12" }
	lookPath = func(file string) (string, error) { return "/usr/local/bin/uv", nil }
	checkPortAvailable = func(_ string) error { return nil }
	runCommand = func(dir, name string, args ...string) error { return nil }
	checkWebserverHealth = func(url string, timeout time.Duration, component string) error { return nil }

	tmpDir, err := os.MkdirTemp("", "standalone-floating-tag")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Pre-create cached constraints + freeze (keyed on resolved tag 3.1-12)
	constraintsDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(constraintsDir, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "constraints-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\napache-airflow-task-sdk==1.0.0\n"), 0o644)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(constraintsDir, "freeze-3.1-12-python-3.12.txt"), []byte("apache-airflow==3.0.1\n"), 0o644)
	s.NoError(err)

	// Create a fake airflow binary
	venvBin := filepath.Join(tmpDir, ".venv", venvBinDir())
	err = os.MkdirAll(venvBin, 0o755)
	s.NoError(err)
	err = os.WriteFile(filepath.Join(venvBin, "airflow"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	s.NoError(err)

	handler, err := StandaloneInit(tmpDir, ".env", "Dockerfile")
	s.NoError(err)

	err = handler.Start(&types.StartOptions{SettingsFile: "airflow_settings.yaml", WaitTime: 1 * time.Minute, Foreground: true})
	s.NoError(err)
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
	venvBin := filepath.Join(tmpDir, ".venv", venvBinDir())
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

	err = handler.Start(&types.StartOptions{SettingsFile: "airflow_settings.yaml", WaitTime: 1 * time.Minute, Foreground: true})
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

	// Create a fake airflow binary that sleeps until killed
	venvBin := filepath.Join(tmpDir, ".venv", venvBinDir())
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

	err = handler.Start(&types.StartOptions{SettingsFile: "airflow_settings.yaml", WaitTime: 1 * time.Minute})
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

	venvBin := filepath.Join(tmpDir, ".venv", venvBinDir())
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

	err = handler.Start(&types.StartOptions{SettingsFile: "airflow_settings.yaml", WaitTime: 1 * time.Minute})
	s.Error(err)
	s.Contains(err.Error(), "already running")
}

func (s *Suite) TestStandaloneStop_Running() {
	tmpDir, err := os.MkdirTemp("", "standalone-stop-running")
	s.NoError(err)
	defer os.RemoveAll(tmpDir)

	// Start a real background process that we can stop
	standaloneStateDir := filepath.Join(tmpDir, ".astro", "standalone")
	err = os.MkdirAll(standaloneStateDir, 0o755)
	s.NoError(err)

	// Start a long-running process and reap it in the background so it
	// doesn't become a zombie after termination (zombies still respond to
	// signal 0, which would cause the Stop poll loop to spin for the full timeout).
	cmd := longSleepCommand()
	setProcGroup(cmd)
	err = cmd.Start()
	s.NoError(err)
	go cmd.Wait() //nolint:errcheck

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
		result := airflowrt.ResolveInEnvPath("mybinary", env)
		assert.Equal(t, fakeBin, result)
	})

	t.Run("returns original when not found", func(t *testing.T) {
		env := []string{"PATH=/nonexistent"}
		result := airflowrt.ResolveInEnvPath("mybinary", env)
		assert.Equal(t, "mybinary", result)
	})

	t.Run("skips resolution for absolute paths", func(t *testing.T) {
		env := []string{"PATH=" + tmpDir}
		result := airflowrt.ResolveInEnvPath("/usr/bin/bash", env)
		assert.Equal(t, "/usr/bin/bash", result)
	})

	t.Run("skips resolution for relative paths with separator", func(t *testing.T) {
		env := []string{"PATH=" + tmpDir}
		result := airflowrt.ResolveInEnvPath("./mybinary", env)
		assert.Equal(t, "./mybinary", result)
	})

	t.Run("no PATH in env falls through", func(t *testing.T) {
		env := []string{"HOME=/tmp"}
		result := airflowrt.ResolveInEnvPath("mybinary", env)
		assert.Equal(t, "mybinary", result)
	})
}

