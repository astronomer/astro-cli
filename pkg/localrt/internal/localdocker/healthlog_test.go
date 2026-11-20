package localdocker

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A health wait's debug log says what the probes answered each time the answer
// changed, not once per poll: a five-minute wait is three lines, not three
// hundred.
func TestHealthWaitLogsOnlyChanges(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls <= 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)

	var last string
	for range 3 {
		if anyHealthy(context.Background(), srv.Client(), []string{srv.URL}, &last) {
			t.Fatal("anyHealthy() = true on a 503")
		}
	}
	if !anyHealthy(context.Background(), srv.Client(), []string{srv.URL}, &last) {
		t.Fatal("anyHealthy() = false on a 200")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("logged %d lines, want 2 (the 503, then the 200):\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "503 Service Unavailable") || !strings.Contains(lines[1], "200 OK") {
		t.Errorf("lines = %q, want the 503 then the 200", lines)
	}
}
