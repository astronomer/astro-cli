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
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", nil)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Astro Dev Proxy")
	assert.Contains(t, w.Body.String(), "No active projects")
}

func TestProxy_LandingPageWithRoutes(t *testing.T) {
	setupTestDir(t)

	err := AddRoute(Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", nil)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "my-project.localhost")
	assert.Contains(t, w.Body.String(), "12345")
}

func TestProxy_NotFound(t *testing.T) {
	setupTestDir(t)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://unknown.localhost:6563/", nil)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Project Not Found")
	assert.Contains(t, w.Body.String(), "unknown.localhost")
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

	err := AddRoute(Route{
		Hostname:   "my-project.localhost",
		Port:       fmt.Sprintf("%d", backendPort),
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	p := NewProxy("6563")
	req := httptest.NewRequest(http.MethodGet, "http://my-project.localhost:6563/", nil)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello from backend", w.Body.String())
}
