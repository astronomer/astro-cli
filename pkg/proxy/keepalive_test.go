package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Each proxied request opens its own connection to the backend. A connection
// kept from before a project restart would reach a process that is gone, and a
// request sent on it hangs instead of failing over to a fresh one.
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
		t.Errorf("3 requests opened %d backend connections, want 3: a pooled connection outlives a project restart", got)
	}
}
