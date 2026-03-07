//go:build !windows

package proxy

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRunning_NoPIDFile(t *testing.T) {
	setupTestDir(t)

	_, alive := IsRunning()
	assert.False(t, alive)
}

func TestIsRunning_StalePIDFile(t *testing.T) {
	setupTestDir(t)

	// Write a PID file with a dead PID
	dir := proxyDirPath()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, pidFileName), []byte("99999999"), 0o644)

	origIsPIDAlive := isPIDAlive
	defer func() { isPIDAlive = origIsPIDAlive }()
	isPIDAlive = func(_ int) bool { return false }

	_, alive := IsRunning()
	assert.False(t, alive)
}

func TestIsRunning_AlivePID(t *testing.T) {
	setupTestDir(t)

	pid := os.Getpid()
	dir := proxyDirPath()
	os.MkdirAll(dir, 0o755)
	// PID file format: "<pid> <version>"
	os.WriteFile(filepath.Join(dir, pidFileName), []byte(strconv.Itoa(pid)+" 1.0.0"), 0o644)

	gotPid, alive := IsRunning()
	assert.True(t, alive)
	assert.Equal(t, pid, gotPid)
}

func TestIsRunning_AlivePID_NoVersion(t *testing.T) {
	setupTestDir(t)

	// Backwards-compat: PID file with no version (old daemon)
	pid := os.Getpid()
	dir := proxyDirPath()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, pidFileName), []byte(strconv.Itoa(pid)), 0o644)

	gotPid, alive := IsRunning()
	assert.True(t, alive)
	assert.Equal(t, pid, gotPid)
}

func TestStopIfEmpty_NoRoutes(t *testing.T) {
	setupTestDir(t)

	// StopIfEmpty should not panic when there are no routes
	StopIfEmpty()
}
