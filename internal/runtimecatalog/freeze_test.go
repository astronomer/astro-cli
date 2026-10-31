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
	for _, client := range []*http.Client{cdnClient, mirrorClient} {
		got, err := fetchFile(context.Background(), client, srv.URL+"/old")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "apache-airflow==3.3.2\n" {
			t.Errorf("fetchFile = %q", got)
		}
	}
}

// redirectingPair is a server whose every path redirects to other, which
// serves body, with a presigned-looking query on the way.
func redirectingPair(t *testing.T, body string) *httptest.Server {
	t.Helper()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/file.txt?X-Amz-Signature=secret", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTheCDNRefusesARedirectToAnotherHost(t *testing.T) {
	srv := redirectingPair(t, "apache-airflow==6.6.6\n")
	_, err := fetchFile(context.Background(), cdnClient, srv.URL+"/file.txt")
	if err == nil || !strings.Contains(err.Error(), "refusing a redirect") {
		t.Fatalf("err = %v, want a refused redirect", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("err = %v, shows the target's query", err)
	}
}

func TestAMirrorMayRedirectToItsObjectStore(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	srv := redirectingPair(t, "apache-airflow==3.3.2\n")
	t.Setenv(ConstraintsURLEnv, srv.URL)
	got, err := RuntimeConstraints(context.Background(), "3.3-9", "3.14")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "apache-airflow==3.3.2" {
		t.Errorf("RuntimeConstraints = %q", got)
	}
}

func TestFetchFileStopsARedirectLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	if _, err := fetchFile(context.Background(), mirrorClient, srv.URL+"/x"); err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Errorf("err = %v, want the redirect cap", err)
	}
}

func TestPinRedirectPolicy(t *testing.T) {
	for _, c := range []struct {
		from, to    string
		cdn, mirror bool
	}{
		{"https://cdn.example.com/a", "https://cdn.example.com/b", true, true},
		{"https://cdn.example.com/a", "https://CDN.example.com:443/b", true, true},
		{"https://cdn.example.com/a", "https://cdn.example.com:8443/b", false, true},
		{"https://cdn.example.com/a", "https://bucket.s3.amazonaws.com/b", false, true},
		{"https://cdn.example.com/a", "http://cdn.example.com/b", false, false},
		{"https://mirror.corp/a", "http://bucket.corp/b", false, false},
		{"http://mirror.corp/a", "http://mirror.corp/b", true, true},
		{"http://mirror.corp/a", "https://bucket.corp/b", false, true},
	} {
		from, err := http.NewRequest(http.MethodGet, c.from, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		to, err := http.NewRequest(http.MethodGet, c.to, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		via := []*http.Request{from}
		if err := pinRedirectPolicy(false)(to, via); (err == nil) != c.cdn {
			t.Errorf("CDN %s -> %s: err = %v, want allowed=%v", c.from, c.to, err, c.cdn)
		}
		if err := pinRedirectPolicy(true)(to, via); (err == nil) != c.mirror {
			t.Errorf("mirror %s -> %s: err = %v, want allowed=%v", c.from, c.to, err, c.mirror)
		}
	}
}

func TestRuntimeConstraintsRefusesANameThatIsNotAVersion(t *testing.T) {
	serveFiles(t, "x==1\n")
	if _, err := RuntimeConstraints(context.Background(), "../x", "3.14"); !errors.Is(err, ErrNotPublished) {
		t.Errorf("err = %v, want ErrNotPublished", err)
	}
}
