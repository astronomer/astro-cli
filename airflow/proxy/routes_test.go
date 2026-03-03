package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origProxyDirPath := proxyDirPath
	proxyDirPath = func() string {
		return filepath.Join(dir, "proxy")
	}
	t.Cleanup(func() {
		proxyDirPath = origProxyDirPath
	})
	return dir
}

func TestAddRoute(t *testing.T) {
	setupTestDir(t)

	route := Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(), // current process is alive
	}

	err := AddRoute(route)
	require.NoError(t, err)

	routes, err := ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)
	assert.Equal(t, "12345", routes[0].Port)
}

func TestAddRoute_UpdateSameProject(t *testing.T) {
	setupTestDir(t)
	pid := os.Getpid()

	route1 := Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        pid,
	}
	err := AddRoute(route1)
	require.NoError(t, err)

	// Update with new port for same project
	route2 := Route{
		Hostname:   "my-project.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/my-project",
		PID:        pid,
	}
	err = AddRoute(route2)
	require.NoError(t, err)

	routes, err := ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "12346", routes[0].Port)
}

func TestAddRoute_HostnameCollision(t *testing.T) {
	setupTestDir(t)
	pid := os.Getpid()

	route1 := Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/project-a",
		PID:        pid,
	}
	err := AddRoute(route1)
	require.NoError(t, err)

	// Different project with same hostname should fail
	route2 := Route{
		Hostname:   "my-project.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/project-b",
		PID:        pid,
	}
	err = AddRoute(route2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRemoveRoute(t *testing.T) {
	setupTestDir(t)
	pid := os.Getpid()

	err := AddRoute(Route{
		Hostname:   "project-a.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/project-a",
		PID:        pid,
	})
	require.NoError(t, err)

	err = AddRoute(Route{
		Hostname:   "project-b.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/project-b",
		PID:        pid,
	})
	require.NoError(t, err)

	remaining, err := RemoveRoute("project-a.localhost")
	require.NoError(t, err)
	assert.Equal(t, 1, remaining)

	routes, err := ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "project-b.localhost", routes[0].Hostname)
}

func TestRemoveRoute_LastRoute(t *testing.T) {
	setupTestDir(t)

	err := AddRoute(Route{
		Hostname:   "project-a.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/project-a",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	remaining, err := RemoveRoute("project-a.localhost")
	require.NoError(t, err)
	assert.Equal(t, 0, remaining)
}

func TestListRoutes_Empty(t *testing.T) {
	setupTestDir(t)

	routes, err := ListRoutes()
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestGetRoute(t *testing.T) {
	setupTestDir(t)

	err := AddRoute(Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	route, err := GetRoute("my-project.localhost")
	require.NoError(t, err)
	require.NotNil(t, route)
	assert.Equal(t, "12345", route.Port)

	// Non-existent route
	route, err = GetRoute("nonexistent.localhost")
	require.NoError(t, err)
	assert.Nil(t, route)
}

func TestPruneStaleRoutes(t *testing.T) {
	setupTestDir(t)

	// Override isPIDAlive to simulate stale routes
	origIsPIDAlive := isPIDAlive
	defer func() { isPIDAlive = origIsPIDAlive }()

	alivePID := os.Getpid()
	deadPID := 99999999 // very unlikely to be a real PID

	isPIDAlive = func(pid int) bool {
		return pid == alivePID
	}

	err := AddRoute(Route{
		Hostname:   "alive.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/alive",
		PID:        alivePID,
	})
	require.NoError(t, err)

	// Manually write a route with a dead PID (bypass AddRoute's PID check)
	routes, err := ListRoutes()
	require.NoError(t, err)
	routes = append(routes, Route{
		Hostname:   "dead.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/dead",
		PID:        deadPID,
	})
	err = writeRoutes(routes)
	require.NoError(t, err)

	// ListRoutes should prune the dead route
	routes, err = ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "alive.localhost", routes[0].Hostname)
}

func TestAddRoute_WithServices(t *testing.T) {
	setupTestDir(t)

	route := Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
		Services:   map[string]string{"postgres": "15432"},
	}

	err := AddRoute(route)
	require.NoError(t, err)

	got, err := GetRoute("my-project.localhost")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "15432", got.Services["postgres"])
}
