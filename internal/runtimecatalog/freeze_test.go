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

func TestFetchFileFollowsARedirectOnTheSameHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("apache-airflow==3.3.2\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	got, err := fetchFile(context.Background(), srv.URL+"/old")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "apache-airflow==3.3.2\n" {
		t.Errorf("fetchFile = %q", got)
	}
}

func TestFetchFileRefusesARedirectToAnotherHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("apache-airflow==6.6.6\n"))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/file.txt", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	if _, err := fetchFile(context.Background(), srv.URL+"/file.txt"); err == nil || !strings.Contains(err.Error(), "refusing a redirect") {
		t.Errorf("err = %v, want a refused redirect", err)
	}
}

func TestSameHostRedirectRefusesHTTPSToHTTP(t *testing.T) {
	from, _ := http.NewRequest(http.MethodGet, "https://cdn.example.com/a.txt", http.NoBody)
	for to, ok := range map[string]bool{
		"https://cdn.example.com/b.txt":  true,
		"http://cdn.example.com/b.txt":   false,
		"https://evil.example.com/b.txt": false,
	} {
		req, _ := http.NewRequest(http.MethodGet, to, http.NoBody)
		if err := sameHostRedirect(req, []*http.Request{from}); (err == nil) != ok {
			t.Errorf("redirect to %s: err = %v, want allowed=%v", to, err, ok)
		}
	}
}

func TestRuntimeConstraintsRefusesANameThatIsNotAVersion(t *testing.T) {
	serveFiles(t, "x==1\n")
	if _, err := RuntimeConstraints(context.Background(), "../x", "3.14"); !errors.Is(err, ErrNotPublished) {
		t.Errorf("err = %v, want ErrNotPublished", err)
	}
}
