package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxy_LandingPage(t *testing.T) {
	setupTestDir(t)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Astro Dev Proxy")
	assert.Contains(t, w.Body.String(), "No active projects")
}

func TestProxy_LandingPageWithRoutes(t *testing.T) {
	setupTestDir(t)

	err := AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "my-project.localhost")
	assert.Contains(t, w.Body.String(), "12345")
}

func TestProxy_NotFound(t *testing.T) {
	setupTestDir(t)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://unknown.localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Project Not Found")
	assert.Contains(t, w.Body.String(), "unknown.localhost")
}

func TestAddRoute_PersistsWhenDaemonFails(t *testing.T) {
	setupTestDir(t)

	// Simulate the docker.go flow where AddRoute is called before EnsureRunning.
	// Even when the daemon cannot start (as on Windows), routes.json must be
	// populated so other tools can discover the project.
	origStartDaemon := StartDaemon
	defer func() { StartDaemon = origStartDaemon }()
	StartDaemon = func(_ string) error {
		return fmt.Errorf("proxy daemon is not supported on Windows")
	}

	route := &Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        0,
		Services:   map[string]string{"postgres": "15432"},
		Mode:       "docker",
	}
	err := AddRoute(route)
	require.NoError(t, err)

	// EnsureRunning fails — but route must still be in routes.json.
	_, err = EnsureRunning("6563")
	assert.Error(t, err)

	routes, err := ListRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, "my-project.localhost", routes[0].Hostname)
	assert.Equal(t, "12345", routes[0].Port)
	assert.Equal(t, "docker", routes[0].Mode)
	assert.Equal(t, "15432", routes[0].Services["postgres"])

	// GetRouteByProject should also find it.
	found, err := GetRouteByProject("/home/user/my-project")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "my-project.localhost", found.Hostname)
}

func TestProxy_ReverseProxy(t *testing.T) {
	setupTestDir(t)

	// Create a test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend")) //nolint:errcheck
	}))
	defer backend.Close()

	// Extract port from backend URL
	backendPort := backend.Listener.Addr().(*net.TCPAddr).Port

	err := AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       fmt.Sprintf("%d", backendPort),
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://my-project.localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello from backend", w.Body.String())
}
