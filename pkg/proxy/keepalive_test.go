package proxy

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Each proxied request opens its own connection to the backend. A connection
// pooled from before a project restart reaches a process that is gone; see
// newBackendTransport.
func TestEachProxiedRequestOpensItsOwnBackendConnection(t *testing.T) {
	var opened atomic.Int32
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	backend.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	backend.Start()
	t.Cleanup(backend.Close)
	_, port, _ := strings.Cut(strings.TrimPrefix(backend.URL, "http://"), ":")

	p := NewProxy("0", NewStore(t.TempDir()))
	rp := p.getOrCreateProxy(port)

	for range 3 {
		rec := httptest.NewRecorder()
		rp.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x.localhost/", http.NoBody))
		if rec.Body.String() != "ok" {
			t.Fatalf("backend not reached: %q", rec.Body.String())
		}
	}
	if got := opened.Load(); got != 3 {
		t.Errorf("3 requests opened %d backend connections, want 3: a connection is being reused across requests", got)
	}
}

// A POST sent the moment a project's Airflow comes back on the same port
// reaches it. With a connection pooled from before the restart it would go out
// on a socket to the old process, and Go does not replay a POST, so it would
// fail as "Backend unavailable" though the backend is up. Repeated because the
// failure depends on timing: with keep-alives on it lands most of the time, not
// every time.
func TestAPostJustAfterARestartReachesTheNewBackend(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	for i := range 20 {
		first := httptest.NewServer(handler)
		_, port, _ := strings.Cut(strings.TrimPrefix(first.URL, "http://"), ":")

		p := NewProxy("0", NewStore(t.TempDir()))
		rp := p.getOrCreateProxy(port)

		rec := httptest.NewRecorder()
		rp.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x.localhost/", http.NoBody))
		if rec.Body.String() != "ok" {
			t.Fatalf("run %d: first backend not reached: %q", i, rec.Body.String())
		}

		// The restart: the old process goes, a new one takes the same port.
		first.Close()
		l, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Skipf("could not rebind the backend's port: %v", err)
		}
		second := httptest.NewUnstartedServer(handler)
		second.Listener = l
		second.Start()

		rec = httptest.NewRecorder()
		rp.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://x.localhost/api", strings.NewReader("{}")))
		second.Close()

		if rec.Code != http.StatusOK {
			t.Fatalf("run %d: a POST just after the restart got %d %q, want it to reach the new backend", i, rec.Code, rec.Body.String())
		}
	}
}

// The transport reports a connection closed before it wrote the request as
// net/http's unexported errServerClosedIdle, which it raises on a brand-new
// connection as well as a pooled one. That is a backend going away, the same as
// a bare EOF.
func TestAConnectionClosedBeforeTheRequestIsUnreachable(t *testing.T) {
	if !isBackendUnreachable(errors.New(serverClosedIdle)) {
		t.Errorf("%q was not taken for an unreachable backend", serverClosedIdle)
	}
}
