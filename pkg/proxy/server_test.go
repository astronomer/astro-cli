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
	s := testStore(t)

	p := NewProxy("6563", s)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Astro Local Proxy")
	assert.Contains(t, w.Body.String(), "astro local start")
	assert.Contains(t, w.Body.String(), "No active projects")
}

func TestProxy_LandingPageWithRoutes(t *testing.T) {
	s := testStore(t)

	err := s.AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       "12345",
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	p := NewProxy("6563", s)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "my-project.localhost")
	assert.Contains(t, w.Body.String(), "12345")
}

func TestProxy_NotFound(t *testing.T) {
	s := testStore(t)

	p := NewProxy("6563", s)
	req := httptest.NewRequest(http.MethodGet, "http://unknown.localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Project Not Found")
	assert.Contains(t, w.Body.String(), "unknown.localhost")
	assert.Contains(t, w.Body.String(), "astro local start")
}

func TestProxy_ReverseProxy(t *testing.T) {
	s := testStore(t)

	// Create a test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "yes")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend")) //nolint:errcheck
	}))
	defer backend.Close()

	backendPort := backend.Listener.Addr().(*net.TCPAddr).Port

	err := s.AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       fmt.Sprintf("%d", backendPort),
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	p := NewProxy("6563", s)
	req := httptest.NewRequest(http.MethodGet, "http://my-project.localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello from backend", w.Body.String())
}

func TestProxy_ModifyResponseHooks(t *testing.T) {
	s := testStore(t)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendPort := backend.Listener.Addr().(*net.TCPAddr).Port

	err := s.AddRoute(&Route{
		Hostname:   "my-project.localhost",
		Port:       fmt.Sprintf("%d", backendPort),
		ProjectDir: "/home/user/my-project",
		PID:        os.Getpid(),
	})
	require.NoError(t, err)

	// Hooks must run in order on the proxied response.
	var order []string
	p := NewProxy("6563", s)
	p.ModifyResponse = []func(*http.Response) error{
		func(resp *http.Response) error {
			order = append(order, "strip")
			resp.Header.Del("X-Frame-Options")
			return nil
		},
		func(resp *http.Response) error {
			order = append(order, "mark")
			resp.Header.Set("X-Hooked", "yes")
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "http://my-project.localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"strip", "mark"}, order)
	assert.Empty(t, w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "yes", w.Header().Get("X-Hooked"))
}

func TestProxy_StartReportsBoundPort(t *testing.T) {
	s := testStore(t)

	// Occupy a port so Start has to fall back to an ephemeral one.
	taken, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer taken.Close()
	takenPort := fmt.Sprintf("%d", taken.Addr().(*net.TCPAddr).Port)

	p := NewProxy(takenPort, s)
	require.NoError(t, p.Start())
	defer p.Stop()

	assert.True(t, p.Running())
	assert.NotEqual(t, takenPort, p.Port())

	// The bound port must answer.
	resp, err := http.Get("http://127.0.0.1:" + p.Port() + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
