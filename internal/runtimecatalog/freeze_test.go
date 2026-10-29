package runtimecatalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// serveFiles points both published files at a local server answering body
// for any name (404 when body is ""), with an empty cache, and counts its
// requests.
func serveFiles(t *testing.T, body string) *atomic.Int32 {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(ConstraintsURLEnv, "")
	t.Setenv(FreezeURLEnv, "")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if body == "" {
			http.NotFound(w, nil)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	oldC, oldF := constraintsFile, freezeFile
	constraintsFile.baseURL, freezeFile.baseURL = srv.URL, srv.URL
	t.Cleanup(func() { constraintsFile, freezeFile = oldC, oldF })
	return &hits
}

func TestRuntimeConstraintsReadsAndKeepsTheFile(t *testing.T) {
	hits := serveFiles(t, "apache-airflow==3.3.2+astro.2\napache-airflow-task-sdk==1.3.2+astro.1\nSQLAlchemy==2.0.50\n")
	for range 2 {
		got, err := RuntimeConstraints(context.Background(), "3.3-9", "3.14")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != "apache-airflow==3.3.2+astro.2,apache-airflow-task-sdk==1.3.2+astro.1,SQLAlchemy==2.0.50" {
			t.Errorf("RuntimeConstraints = %q", got)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests, want 1: a build's file is kept", hits.Load())
	}
}

func TestRuntimeFreezeRemembersANotPublishedFile(t *testing.T) {
	hits := serveFiles(t, "")
	for range 2 {
		if _, err := RuntimeFreeze(context.Background(), "10.9.0", "3.11"); !errors.Is(err, ErrNotPublished) {
			t.Fatalf("err = %v, want ErrNotPublished", err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests, want 1: a 404 is remembered", hits.Load())
	}
}

func TestRuntimeConstraintsRefusesAPageWithNoPins(t *testing.T) {
	serveFiles(t, "<html>captive portal</html>")
	if _, err := RuntimeConstraints(context.Background(), "3.3-9", "3.14"); err == nil || errors.Is(err, ErrNotPublished) {
		t.Errorf("err = %v, want a read error", err)
	}
}

func TestRuntimeConstraintsRefusesANameThatIsNotAVersion(t *testing.T) {
	serveFiles(t, "x==1\n")
	if _, err := RuntimeConstraints(context.Background(), "../x", "3.14"); !errors.Is(err, ErrNotPublished) {
		t.Errorf("err = %v, want ErrNotPublished", err)
	}
}
