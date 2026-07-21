package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "proxy"))
}

func TestAddRoute(t *testing.T) {
	s := testStore(t)

	route := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(), // current process is alive
	}

	err := s.AddRoute(route)
	require.NoError(t, err)

	routes, err := s.ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)
	assert.Equal(t, "12345", routes[0].Port)
}

func TestAddRoute_UpdateSameProject(t *testing.T) {
	s := testStore(t)
	pid := os.Getpid()

	route1 := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        pid,
	}
	err := s.AddRoute(route1)
	require.NoError(t, err)

	// Update with new port for same project
	route2 := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/my-project",
		PID:        pid,
	}
	err = s.AddRoute(route2)
	require.NoError(t, err)

	routes, err := s.ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "12346", routes[0].Port)
}

func TestAddRoute_HostnameCollision(t *testing.T) {
	s := testStore(t)
	pid := os.Getpid()

	route1 := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/project-a",
		PID:        pid,
	}
	err := s.AddRoute(route1)
	require.NoError(t, err)

	// Different project with same hostname should fail
	route2 := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/project-b",
		PID:        pid,
	}
	err = s.AddRoute(route2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRemoveRoute(t *testing.T) {
	s := testStore(t)
	pid := os.Getpid()

	err := s.AddRoute(&Route{
		Hostname:   "project-a.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/project-a",
		PID:        pid,
	})
	require.NoError(t, err)

	err = s.AddRoute(&Route{
		Hostname:   "project-b.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/project-b",
		PID:        pid,
	})
	require.NoError(t, err)

	remaining, err := s.RemoveRoute("project-a.localhost")
	require.NoError(t, err)
	assert.Equal(t, 1, remaining)

	routes, err := s.ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "project-b.localhost", routes[0].Hostname)
}

func TestRemoveRoute_LastRoute(t *testing.T) {
	s := testStore(t)

	err := s.AddRoute(&Route{
		Hostname:   "project-a.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/project-a",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	remaining, err := s.RemoveRoute("project-a.localhost")
	require.NoError(t, err)
	assert.Equal(t, 0, remaining)
}

func TestListRoutes_Empty(t *testing.T) {
	s := testStore(t)

	routes, err := s.ListRoutes()
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestGetRoute(t *testing.T) {
	s := testStore(t)

	err := s.AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	route, err := s.GetRoute("my-project.localhost")
	require.NoError(t, err)
	require.NotNil(t, route)
	assert.Equal(t, "12345", route.Port)

	// Non-existent route
	route, err = s.GetRoute("nonexistent.localhost")
	require.NoError(t, err)
	assert.Nil(t, route)
}

func TestGetRouteByProject(t *testing.T) {
	s := testStore(t)

	err := s.AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	route, err := s.GetRouteByProject("/home/user/my-project")
	require.NoError(t, err)
	require.NotNil(t, route)
	assert.Equal(t, "12345", route.Port)

	// Non-existent project
	route, err = s.GetRouteByProject("/home/user/other")
	require.NoError(t, err)
	assert.Nil(t, route)
}

func TestPruneStaleRoutes(t *testing.T) {
	s := testStore(t)

	// Override IsPIDAlive to simulate stale routes
	origIsPIDAlive := IsPIDAlive
	defer func() { IsPIDAlive = origIsPIDAlive }()

	alivePID := os.Getpid()
	deadPID := 99999999 // very unlikely to be a real PID

	IsPIDAlive = func(pid int) bool {
		return pid == alivePID
	}

	err := s.AddRoute(&Route{
		Hostname:   "alive.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/alive",
		PID:        alivePID,
	})
	require.NoError(t, err)

	// Manually write a route with a dead PID (bypass AddRoute's PID check)
	routes, err := s.ListRoutes()
	require.NoError(t, err)
	routes = append(routes, Route{
		Hostname:   "dead.localhost",
		Port:       "12346",
		ProjectDir: "/home/user/dead",
		PID:        deadPID,
	})
	err = s.WriteRoutes(routes)
	require.NoError(t, err)

	// ListRoutes should prune the dead route
	routes, err = s.ListRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "alive.localhost", routes[0].Hostname)
}

func TestPruneStaleRoutes_DockerNotPruned(t *testing.T) {
	origIsPIDAlive := IsPIDAlive
	defer func() { IsPIDAlive = origIsPIDAlive }()
	IsPIDAlive = func(_ int) bool { return false }

	// Docker routes should survive pruning even with dead PIDs
	routes := []Route{
		{Hostname: "docker.localhost", Port: "12345", ProjectDir: "/tmp/docker", PID: 99999, Mode: "docker"},
		{Hostname: "standalone.localhost", Port: "12346", ProjectDir: "/tmp/standalone", PID: 99999},
	}
	pruned := PruneStaleRoutes(routes)
	assert.Len(t, pruned, 1)
	assert.Equal(t, "docker.localhost", pruned[0].Hostname)
}

func TestAddRoute_DockerRouteSurvivesPruningWithDeadPID(t *testing.T) {
	s := testStore(t)

	// Simulate the docker.go flow: AddRoute is called with PID 0 and Mode "docker".
	// Even when IsPIDAlive returns false (as it does on Windows), docker routes
	// must not be pruned — they are cleaned up explicitly via RemoveRoute.
	origIsPIDAlive := IsPIDAlive
	defer func() { IsPIDAlive = origIsPIDAlive }()
	IsPIDAlive = func(_ int) bool { return false }

	route := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        0,
		Services:   map[string]string{"postgres": "15432"},
		Mode:       "docker",
	}
	err := s.AddRoute(route)
	require.NoError(t, err)

	// ListRoutes prunes stale routes internally — docker routes must survive.
	routes, err := s.ListRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)
	assert.Equal(t, "12345", routes[0].Port)
	assert.Equal(t, "docker", routes[0].Mode)
	assert.Equal(t, "15432", routes[0].Services["postgres"])
}

func TestAddRoute_WithServices(t *testing.T) {
	s := testStore(t)

	route := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
		Services:   map[string]string{"postgres": "15432"},
	}

	err := s.AddRoute(route)
	require.NoError(t, err)

	got, err := s.GetRoute("my-project.localhost")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "15432", got.Services["postgres"])
}
