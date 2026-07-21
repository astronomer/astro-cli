//go:build !windows

package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// setupTestDir points config.HomeConfigPath at a temp dir so Routes() and the
// daemon files live under it.
func setupTestDir(t *testing.T) {
	t.Helper()
	orig := config.HomeConfigPath
	config.HomeConfigPath = t.TempDir()
	t.Cleanup(func() {
		config.HomeConfigPath = orig
	})
}

func TestIsRunning_NoPIDFile(t *testing.T) {
	setupTestDir(t)

	_, alive := IsRunning()
	assert.False(t, alive)
}

func TestIsRunning_StalePIDFile(t *testing.T) {
	setupTestDir(t)

	// Write a PID file with a dead PID
	dir := Routes().Dir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, pidFileName), []byte("99999999"), 0o644)

	origIsPIDAlive := pkgproxy.IsPIDAlive
	defer func() { pkgproxy.IsPIDAlive = origIsPIDAlive }()
	pkgproxy.IsPIDAlive = func(_ int) bool { return false }

	_, alive := IsRunning()
	assert.False(t, alive)
}

func TestIsRunning_AlivePID(t *testing.T) {
	setupTestDir(t)

	pid := os.Getpid()
	dir := Routes().Dir()
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
	dir := Routes().Dir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, pidFileName), []byte(strconv.Itoa(pid)), 0o644)

	gotPid, alive := IsRunning()
	assert.True(t, alive)
	assert.Equal(t, pid, gotPid)
}

func TestParsePIDFile_WithPort(t *testing.T) {
	setupTestDir(t)

	pid := os.Getpid()
	require.NoError(t, os.MkdirAll(Routes().Dir(), 0o755))
	require.NoError(t, writePIDFile(pid, "16123"))

	gotPid, ver, port, err := parsePIDFile()
	require.NoError(t, err)
	assert.Equal(t, pid, gotPid)
	// Test builds have no version; writePIDFile stores "-" which reads back empty.
	assert.Equal(t, "", ver)
	assert.Equal(t, "16123", port)

	assert.Equal(t, "16123", BoundPort())
}

func TestBoundPort_OldPIDFileWithoutPort(t *testing.T) {
	setupTestDir(t)

	pid := os.Getpid()
	dir := Routes().Dir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, pidFileName), []byte(strconv.Itoa(pid)+" 1.0.0"), 0o644)

	assert.Equal(t, "", BoundPort())
}

func TestEnsureRunning_AlreadyRunningReturnsBoundPort(t *testing.T) {
	setupTestDir(t)

	pid := os.Getpid()
	require.NoError(t, os.MkdirAll(Routes().Dir(), 0o755))
	require.NoError(t, writePIDFile(pid, "16123"))

	// Daemon alive with recorded port — EnsureRunning must report that port,
	// not the requested one.
	port, err := EnsureRunning("6563")
	require.NoError(t, err)
	assert.Equal(t, "16123", port)
}

func TestEnsureRunning_ConcurrentStartsOnlyOne(t *testing.T) {
	setupTestDir(t)

	var mu sync.Mutex
	starts := 0

	origStartDaemon := StartDaemon
	defer func() { StartDaemon = origStartDaemon }()
	StartDaemon = func(port string) (string, error) {
		mu.Lock()
		starts++
		mu.Unlock()
		// Mimic the real StartDaemon: record a live daemon in the PID file.
		if err := os.MkdirAll(Routes().Dir(), 0o755); err != nil {
			return "", err
		}
		if err := writePIDFile(os.Getpid(), port); err != nil {
			return "", err
		}
		return port, nil
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	ports := make([]string, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ports[i], errs[i] = EnsureRunning("6563")
		}()
	}
	wg.Wait()

	for i := range 8 {
		require.NoError(t, errs[i])
		assert.Equal(t, "6563", ports[i])
	}
	// The routes lock serializes check-then-start: exactly one goroutine may
	// find no daemon and start one.
	assert.Equal(t, 1, starts)
}

func TestAddRoute_PersistsWhenDaemonFails(t *testing.T) {
	setupTestDir(t)

	// Simulate the docker.go flow where AddRoute is called before EnsureRunning.
	// Even when the daemon cannot start (as on Windows), routes.json must be
	// populated so other tools can discover the project.
	origStartDaemon := StartDaemon
	defer func() { StartDaemon = origStartDaemon }()
	StartDaemon = func(_ string) (string, error) {
		return "", fmt.Errorf("proxy daemon is not supported on Windows")
	}

	route := &pkgproxy.Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        0,
		Services:   map[string]string{"postgres": "15432"},
		Mode:       "docker",
	}
	err := Routes().AddRoute(route)
	require.NoError(t, err)

	// EnsureRunning fails — but route must still be in routes.json.
	_, err = EnsureRunning("6563")
	assert.Error(t, err)

	routes, err := Routes().ListRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)
	assert.Equal(t, "12345", routes[0].Port)
	assert.Equal(t, "docker", routes[0].Mode)
	assert.Equal(t, "15432", routes[0].Services["postgres"])

	// GetRouteByProject should also find it.
	found, err := Routes().GetRouteByProject("/home/user/my-project")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "my-project.localhost", found.Hostname)
}

func TestStopIfEmpty_NoRoutes(t *testing.T) {
	setupTestDir(t)

	// StopIfEmpty should not panic when there are no routes
	StopIfEmpty()
}
