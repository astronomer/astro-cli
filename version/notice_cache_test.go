package version

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// With a fresh cached answer the check resolves before StartUpdateCheck
// returns, with no request, so a command that ends at once still shows it.
func TestUpdateNoticeFromAFreshCacheNeedsNoNetwork(t *testing.T) {
	withCurrVersion(t, "1.40.0")
	cachePath := filepath.Join(t.TempDir(), "release-check.json")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	writeCheckFile(cachePath, releaseCheck{CheckedAt: now.Add(-time.Hour), Major: 1, Latest: "1.45.0"})

	srv := &releaseServer{version: "9.9.9", gate: make(chan struct{})} // would block forever
	check := StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, cachePath, now)
	var out bytes.Buffer
	check.Print(&out, now) // no waiting: the answer is already in
	if !strings.Contains(out.String(), "Astro CLI 1.45.0 is available") {
		t.Errorf("notice = %q", out.String())
	}
	if srv.asked.Load() != 0 {
		t.Error("a fresh cache still asked the network")
	}

	// A stale cache is the one that depends on the network.
	stale := now.Add(2 * releaseCheckTTL)
	check = StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, cachePath, stale)
	t.Cleanup(func() {
		close(srv.gate)
		select {
		case <-check.result:
		case <-time.After(5 * time.Second):
			t.Error("the lookup never finished")
		}
	})
	out.Reset()
	check.Print(&out, stale)
	if out.Len() != 0 {
		t.Errorf("a stale cache printed %q before the network answered", out.String())
	}
}

// A lookup still in flight is waited for, but only for the grace: a transport
// that never answers delays Print by about that long, and no more.
func TestUpdateNoticeWaitIsBounded(t *testing.T) {
	withCurrVersion(t, "1.40.0")
	cachePath := filepath.Join(t.TempDir(), "release-check.json")
	srv := &releaseServer{version: "1.45.0", gate: make(chan struct{})}
	now := time.Now()
	check := StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, cachePath, now)
	t.Cleanup(func() {
		close(srv.gate)
		select {
		case <-check.result:
		case <-time.After(5 * time.Second):
			t.Error("the lookup never finished")
		}
	})
	if check.grace != lookupGrace {
		t.Fatalf("a stale-cache lookup waits %v, want %v", check.grace, lookupGrace)
	}
	check.grace = 50 * time.Millisecond

	var out bytes.Buffer
	start := time.Now()
	check.Print(&out, now)
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Print took %v, want about the 50ms grace", elapsed)
	}
	if out.Len() != 0 {
		t.Errorf("printed %q with no answer", out.String())
	}
}

// An answer that lands inside the grace is shown.
func TestUpdateNoticeShowsAnAnswerThatLandsInTheGrace(t *testing.T) {
	withCurrVersion(t, "1.40.0")
	cachePath := filepath.Join(t.TempDir(), "release-check.json")
	srv := &releaseServer{version: "1.45.0", gate: make(chan struct{})}
	now := time.Now()
	check := StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, cachePath, now)
	check.grace = 5 * time.Second
	time.AfterFunc(20*time.Millisecond, func() { close(srv.gate) })

	var out bytes.Buffer
	check.Print(&out, now)
	if !strings.Contains(out.String(), "Astro CLI 1.45.0 is available") {
		t.Errorf("notice = %q", out.String())
	}
}
