package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

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
		w.Write([]byte("hello from backend"))
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

func TestProxy_StartPrefersTheFallbackPort(t *testing.T) {
	s := testStore(t)

	taken, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer taken.Close()
	takenPort := fmt.Sprintf("%d", taken.Addr().(*net.TCPAddr).Port)

	free, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	freePort := fmt.Sprintf("%d", free.Addr().(*net.TCPAddr).Port)
	require.NoError(t, free.Close())

	p := NewProxy(takenPort, s)
	p.FallbackPort = freePort
	require.NoError(t, p.Start())
	defer p.Stop()

	assert.Equal(t, freePort, p.Port())
}

func TestProxy_StartSkipsATakenFallbackPort(t *testing.T) {
	s := testStore(t)

	taken, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer taken.Close()
	takenPort := fmt.Sprintf("%d", taken.Addr().(*net.TCPAddr).Port)

	p := NewProxy(takenPort, s)
	p.FallbackPort = takenPort
	require.NoError(t, p.Start())
	defer p.Stop()

	assert.NotEqual(t, takenPort, p.Port())
}

// A Host header is whatever the client typed, and it reaches a log line. A
// newline in it would end that line and start one the reader has no way to tell
// from something the proxy wrote.
func TestSafeHostCannotForgeALogLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
		want string
	}{
		{"an ordinary host passes through", "my-project.localtest.me:6563", "my-project.localtest.me:6563"},
		{"a newline cannot end the line", "host\nlevel=ERROR msg=\"database deleted\"", `hostlevel=ERROR msg="database deleted"`},
		{"a carriage return cannot either", "host\rmsg=fake", "hostmsg=fake"},
		{"DEL goes too", "host\x7fmsg", "hostmsg"},
		{"an empty host stays empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, safeHost(tc.host))
		})
	}
}

// Capped so a Host cannot flood the log either. 253 is the longest a DNS name
// can be, so nothing this proxy actually routes is truncated.
func TestSafeHostIsBounded(t *testing.T) {
	assert.Len(t, safeHost(strings.Repeat("a", maxLoggedHost*4)), maxLoggedHost)
	full := strings.Repeat("b", maxLoggedHost)
	assert.Equal(t, full, safeHost(full), "a host at the cap is not truncated")
}

// The cap has to hold for bytes that are not UTF-8, which is the input a
// hostile client is most likely to send. Each undecodable byte becomes a 3-byte
// replacement character, so a cap applied before that expansion is not a cap:
// cleaning after cutting returns 759 bytes for 253 of garbage.
func TestSafeHostIsBoundedForBytesThatAreNotText(t *testing.T) {
	got := safeHost(strings.Repeat("\xff", maxLoggedHost*2))
	assert.LessOrEqual(t, len(got), maxLoggedHost, "the cap counts bytes, and replacement characters are bytes")
	assert.True(t, utf8.ValidString(got), "what reaches the log should still be text")
}

// Cutting mid-rune would put a replacement character at the end of every long
// internationalized host — manufactured by the truncation, not sent by anyone.
func TestSafeHostCutsOnARuneBoundary(t *testing.T) {
	got := safeHost(strings.Repeat("a", maxLoggedHost-1) + "\u00e9tail")
	assert.True(t, utf8.ValidString(got))
	assert.False(t, strings.ContainsRune(got, utf8.RuneError), "truncation invented a replacement character: %q", got)
}
