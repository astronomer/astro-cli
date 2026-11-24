package proxy

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

func setupTestDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	// Reset initOnce so ensureInit doesn't overwrite our test dir.
	// We use a fresh Once and mark it done so ensureInit is a no-op.
	initOnce = sync.Once{} //nolint:govet // intentional reset for testing
	pkgproxy.SetRoutesDir(filepath.Join(dir, "proxy"))
	initOnce.Do(func() {})

	t.Cleanup(func() {
		initOnce = sync.Once{} //nolint:govet // intentional reset for testing
		pkgproxy.SetRoutesDir("")
	})
}

// Route CRUD logic is thoroughly tested in pkg/proxy/routes_test.go.
// These tests verify the CLI wrappers delegate correctly after ensureInit.

func TestRoutes_InitAndDelegate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the proxy cannot tell a live route from a stale one on Windows in 1.x: pkg/proxy IsPIDAlive always reports false there, so every route is pruned")
	}
	setupTestDir(t)

	// AddRoute + ListRoutes
	err := AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	routes, err := ListRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)

	// GetRoute
	route, err := GetRoute("my-project.localhost")
	require.NoError(t, err)
	require.NotNil(t, route)
	assert.Equal(t, "12345", route.Port)

	// GetRouteByProject
	route, err = GetRouteByProject("/home/user/my-project")
	require.NoError(t, err)
	require.NotNil(t, route)
	assert.Equal(t, "12345", route.Port)

	// RemoveRoute
	remaining, err := RemoveRoute("my-project.localhost")
	require.NoError(t, err)
	assert.Equal(t, 0, remaining)

	routes, err = ListRoutes()
	require.NoError(t, err)
	assert.Empty(t, routes)
}
