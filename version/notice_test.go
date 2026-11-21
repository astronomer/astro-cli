package version

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// releaseServer answers the release list with one release, counting the asks;
// gate, when set, holds every answer until it is closed.
type releaseServer struct {
	version string
	asked   atomic.Int32
	gate    chan struct{}
}

func (s *releaseServer) RoundTrip(*http.Request) (*http.Response, error) {
	s.asked.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	body := `{"available_releases":[{"version":"` + s.version + `"}]}`
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func waitFor(t *testing.T, c *UpdateCheck) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(c.result) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the lookup never finished")
		}
		time.Sleep(time.Millisecond)
	}
}

// An outdated release prints two lines, after the lookup is in, once a day.
func TestUpdateNoticePrintsOnceADay(t *testing.T) {
	withCurrVersion(t, "1.40.0")
	srv := &releaseServer{version: "1.45.0"}
	client := &http.Client{Transport: srv}
	cachePath := filepath.Join(t.TempDir(), "release-check.json")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	check := StartUpdateCheck(context.Background(), client, cachePath, now)
	waitFor(t, check)
	var out bytes.Buffer
	check.Print(&out, now)
	got := out.String()
	if !strings.Contains(got, "Astro CLI 1.45.0 is available. You have 1.40.0.") || !strings.Contains(got, upgradeURL) {
		t.Errorf("notice = %q", got)
	}
	if lines := strings.Count(strings.TrimSpace(got), "\n") + 1; lines != 2 {
		t.Errorf("notice has %d lines, want 2: %q", lines, got)
	}

	// An hour later: shown today, so nothing starts and nothing prints.
	if later := StartUpdateCheck(context.Background(), client, cachePath, now.Add(time.Hour)); later != nil {
		t.Error("a lookup started within a day of the last notice")
	}

	// The release answer refreshing a day on keeps the stamp's history: the
	// notice is due again, and shows again.
	next := now.Add(noticeInterval)
	check = StartUpdateCheck(context.Background(), client, cachePath, next)
	if check == nil {
		t.Fatal("no lookup a day after the last notice")
	}
	waitFor(t, check)
	out.Reset()
	check.Print(&out, next)
	if out.Len() == 0 {
		t.Error("no notice a day after the last one")
	}
	if StartUpdateCheck(context.Background(), client, cachePath, next.Add(time.Minute)) != nil {
		t.Error("the second notice was not recorded")
	}
}

// A lookup that has not finished is skipped, never waited for.
func TestUpdateNoticeNeverWaits(t *testing.T) {
	withCurrVersion(t, "1.40.0")
	srv := &releaseServer{version: "1.45.0", gate: make(chan struct{})}
	cachePath := filepath.Join(t.TempDir(), "release-check.json")
	now := time.Now()

	check := StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, cachePath, now)
	// Let the lookup end before the test restores the version it reads.
	t.Cleanup(func() {
		close(srv.gate)
		select {
		case <-check.result:
		case <-time.After(5 * time.Second):
			t.Error("the lookup never finished")
		}
	})
	var out bytes.Buffer
	done := make(chan struct{})
	go func() { check.Print(&out, now); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Print waited for the lookup")
	}
	if out.Len() != 0 {
		t.Errorf("printed %q before the answer came", out.String())
	}
	if !noticeDue(cachePath, now) {
		t.Error("a skipped notice was recorded as shown")
	}
}

// Up to date, a local build, and a nil check print nothing.
func TestUpdateNoticeSaysNothingWhenThereIsNothingToSay(t *testing.T) {
	var out bytes.Buffer
	var nilCheck *UpdateCheck
	nilCheck.Print(&out, time.Now())

	withCurrVersion(t, "1.45.0")
	srv := &releaseServer{version: "1.45.0"}
	cachePath := filepath.Join(t.TempDir(), "release-check.json")
	check := StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, cachePath, time.Now())
	waitFor(t, check)
	check.Print(&out, time.Now())

	withCurrVersion(t, "SNAPSHOT-abc1234")
	srv.version = "9.9.9"
	check = StartUpdateCheck(context.Background(), &http.Client{Transport: srv}, filepath.Join(t.TempDir(), "r.json"), time.Now())
	waitFor(t, check)
	check.Print(&out, time.Now())

	if out.Len() != 0 {
		t.Errorf("printed %q", out.String())
	}
}
