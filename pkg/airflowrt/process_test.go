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
	origPoll, origTimeout, origKillTimeout := stopPollInterval, stopTimeout, killTimeout
	t.Cleanup(func() {
		isProcessAlive, isProcessGroupAlive = origAlive, origGroupAlive
		terminateProcess, killProcess = origTerm, origKill
		stopPollInterval, stopTimeout, killTimeout = origPoll, origTimeout, origKillTimeout
	})
	isProcessAlive = func(int) bool { return true }
	isProcessGroupAlive = groupAlive
	terminateProcess = func(pid int) { *terminated = append(*terminated, pid) }
	killProcess = func(pid int) { *killed = append(*killed, pid) }
	stopPollInterval = time.Millisecond
	stopTimeout = 50 * time.Millisecond
	killTimeout = 20 * time.Millisecond
}

// The master died on its own (a crash, an OOM kill) and its children run on
// in its group: that is a running instance to stop, not a stale PID file.
func TestStopProcess_StopsChildrenOfADeadMaster(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, 12345))

	var terminated, killed []int
	stubStopSeams(t, func(int) bool { return len(terminated) == 0 }, &terminated, &killed)
	isProcessAlive = func(int) bool { return false }

	stopped, err := StopProcess(pidFile)
	assert.NoError(t, err)
	assert.True(t, stopped, "the children of a dead master were left running")
	assert.Equal(t, []int{12345}, terminated)

	_, statErr := os.Stat(pidFile)
	assert.True(t, os.IsNotExist(statErr), "PID file should be cleaned up")
}

// A group that outlives SIGKILL is an error, and its PID file is kept so a
// later stop can still reach it.
func TestStopProcess_GroupThatSurvivesSIGKILL(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "airflow.pid")
	require.NoError(t, WritePID(pidFile, 12345))

	var terminated, killed []int
	stubStopSeams(t, func(int) bool { return true }, &terminated, &killed)

	stopped, err := StopProcess(pidFile)
	assert.True(t, stopped)
	assert.ErrorContains(t, err, "still running after SIGKILL")
	assert.Equal(t, []int{12345}, killed)

	_, statErr := os.Stat(pidFile)
	assert.NoError(t, statErr, "the PID file of a group still running should be kept")
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

	// The group ignores SIGTERM past the timeout: StopProcess must escalate to
	// SIGKILL, which it does go for, and clean up.
	var terminated, killed []int
	stubStopSeams(t, func(int) bool { return len(killed) == 0 }, &terminated, &killed)

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
