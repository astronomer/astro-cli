package airflowrt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPID_NoPIDFile(t *testing.T) {
	pid, alive := ReadPID("/nonexistent/airflow.pid")
	assert.Equal(t, 0, pid)
	assert.False(t, alive)
}

func TestReadPID_CurrentProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, os.Getpid()))

	pid, alive := ReadPID(pidFile)
	assert.Equal(t, os.Getpid(), pid)
	assert.True(t, alive)
}

func TestReadPID_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, 99999999))

	pid, alive := ReadPID(pidFile)
	assert.Equal(t, 99999999, pid)
	assert.False(t, alive)
}

func TestWritePID(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")

	require.NoError(t, WritePID(pidFile, 12345))

	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	assert.Equal(t, "12345", string(data))
}

func TestResolveInEnvPath(t *testing.T) {
	// Create a fake binary
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	fakeBin := filepath.Join(binDir, "mybin")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755))

	env := []string{"PATH=" + binDir + ":/usr/bin"}

	// Should resolve to our fake binary
	resolved := ResolveInEnvPath("mybin", env)
	assert.Equal(t, fakeBin, resolved)

	// Absolute path should be returned as-is
	resolved = ResolveInEnvPath("/usr/bin/env", env)
	assert.Equal(t, "/usr/bin/env", resolved)

	// Unknown binary should be returned as-is
	resolved = ResolveInEnvPath("nonexistent-binary-xyz", env)
	assert.Equal(t, "nonexistent-binary-xyz", resolved)
}

func TestCheckPortAvailable(t *testing.T) {
	// Port 1 is privileged and likely not in use — should be available
	err := CheckPortAvailable("1")
	assert.NoError(t, err)
}

func TestStopProcess_NoPIDFile(t *testing.T) {
	stopped, err := StopProcess("/nonexistent/airflow.pid")
	assert.NoError(t, err)
	assert.False(t, stopped)
}

// stubStopSeams replaces the process-signal seams and poll timing for the
// duration of a test, restoring the real implementations afterward.
func stubStopSeams(t *testing.T, groupAlive func(pid int) bool, terminated, killed *[]int) {
	t.Helper()
	origAlive, origGroupAlive := isProcessAlive, isProcessGroupAlive
	origTerm, origKill := terminateProcess, killProcess
	origPoll, origTimeout := stopPollInterval, stopTimeout
	t.Cleanup(func() {
		isProcessAlive, isProcessGroupAlive = origAlive, origGroupAlive
		terminateProcess, killProcess = origTerm, origKill
		stopPollInterval, stopTimeout = origPoll, origTimeout
	})
	isProcessAlive = func(int) bool { return true }
	isProcessGroupAlive = groupAlive
	terminateProcess = func(pid int) { *terminated = append(*terminated, pid) }
	killProcess = func(pid int) { *killed = append(*killed, pid) }
	stopPollInterval = time.Millisecond
	stopTimeout = 50 * time.Millisecond
}

func TestStopProcess_WaitsForProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, 12345))

	// Simulate airflow standalone's shutdown: the master exits immediately on
	// SIGTERM, but scheduler/triggerer children in the process group take a few
	// polls to finish. StopProcess must wait for the whole group, not just the
	// master.
	var terminated, killed []int
	groupPolls := 0
	stubStopSeams(t, func(int) bool {
		groupPolls++
		return groupPolls < 3
	}, &terminated, &killed)

	stopped, err := StopProcess(pidFile)
	assert.NoError(t, err)
	assert.True(t, stopped)

	assert.Equal(t, []int{12345}, terminated)
	assert.Empty(t, killed, "group exited gracefully; SIGKILL must not be sent")
	assert.GreaterOrEqual(t, groupPolls, 3, "must poll until the whole group is gone")

	_, statErr := os.Stat(pidFile)
	assert.True(t, os.IsNotExist(statErr), "PID file should be cleaned up")
}

func TestStopProcess_ForceKillsSurvivingGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, 12345))

	// The group never exits within the timeout: StopProcess must escalate to
	// SIGKILL and still clean up.
	var terminated, killed []int
	stubStopSeams(t, func(int) bool { return true }, &terminated, &killed)

	stopped, err := StopProcess(pidFile)
	assert.NoError(t, err)
	assert.True(t, stopped)

	assert.Equal(t, []int{12345}, terminated)
	assert.Equal(t, []int{12345}, killed)

	_, statErr := os.Stat(pidFile)
	assert.True(t, os.IsNotExist(statErr), "PID file should be cleaned up")
}

func TestStopProcess_StalePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, 99999999))

	stopped, err := StopProcess(pidFile)
	assert.NoError(t, err)
	assert.False(t, stopped)

	// PID file should be cleaned up
	_, err = os.Stat(pidFile)
	assert.True(t, os.IsNotExist(err))
}
