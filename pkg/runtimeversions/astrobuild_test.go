package runtimeversions

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// listing is the shape pip.astronomer.io serves: one file per line, its size
// and upload time ahead of the link.
func listing(files ...string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><body><pre>\n")
	for _, f := range files {
		stamp, name, _ := strings.Cut(f, " ")
		b.WriteString(" 13.12 KiB  " + stamp + "  <a href=\"" + name + "\">" + name + "</a>\n")
	}
	b.WriteString("</pre></body></html>\n")
	return b.String()
}

var airflowPage = listing(
	"2026-08-21T10:00:00Z apache_airflow-3.3.1+astro.3-py3-none-any.whl",
	"2026-08-31T10:00:00Z apache_airflow-3.3.1+astro.4-py3-none-any.whl",
	"2026-08-31T10:00:00Z apache_airflow-3.3.1+astro.4.tar.gz",
	"2026-09-10T10:00:00Z apache_airflow-3.3.2.dev24126+astro.1-py3-none-any.whl",
	"2026-09-21T08:51:04Z apache_airflow-3.3.2+astro.1-py3-none-any.whl",
	"2026-09-01T10:00:00Z apache_airflow-3.2.2+astro.6-py3-none-any.whl",
	"2026-05-01T10:00:00Z apache_airflow-2.11.2+astro.7-py3-none-any.whl",
	"2026-09-22T10:00:00Z apache_airflow_core-3.3.2+astro.1-py3-none-any.whl",
)

var sdkPage = listing(
	"2026-08-31T10:00:00Z apache_airflow_task_sdk-1.3.1+astro.2-py3-none-any.whl",
	"2026-09-21T08:51:04Z apache_airflow_task_sdk-1.3.2+astro.1-py3-none-any.whl",
	"2026-09-23T08:51:04Z apache_airflow_task_sdk-1.3.2.dev24123+astro.1-py3-none-any.whl",
)

type index struct {
	pages map[string]string
	hits  atomic.Int32
	down  atomic.Bool
}

// serveIndex stands a test index up, points the lookup at it, and leaves the
// catalog unreachable unless catalog is set.
func serveIndex(t *testing.T, catalog string) (*index, Options) {
	t.Helper()
	ix := &index{pages: map[string]string{
		"/v2/apache-airflow/":          airflowPage,
		"/v2/apache-airflow-task-sdk/": sdkPage,
		"/catalog":                     catalog,
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ix.hits.Add(1)
		page, ok := ix.pages[r.URL.Path]
		if ix.down.Load() || !ok || page == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(IndexURLEnv, srv.URL+"/v2")
	t.Setenv(URLEnv, srv.URL+"/catalog")
	return ix, Options{CacheDir: t.TempDir()}
}

func mustLookup(t *testing.T, o Options, pin, runtime string, excludeNewer time.Time) AstroBuild {
	t.Helper()
	b, err := LookupAstroBuild(t.Context(), o, pin, runtime, excludeNewer)
	if err != nil {
		t.Fatalf("LookupAstroBuild(%q, %q): %v", pin, runtime, err)
	}
	return b
}

func TestLookupAstroBuildTakesTheNewestBuildThePinCovers(t *testing.T) {
	_, o := serveIndex(t, "")

	b := mustLookup(t, o, "3.3", "", time.Time{})

	if b.Airflow != "3.3.2+astro.1" || b.TaskSDK != "1.3.2+astro.1" {
		t.Errorf("build = %+v, want 3.3.2+astro.1 with SDK 1.3.2+astro.1", b)
	}
	if !strings.HasSuffix(b.Index, "/v2/") {
		t.Errorf("Index = %q, want the index looked up", b.Index)
	}
	if want := []string{"apache-airflow==3.3.2+astro.1", "apache-airflow-task-sdk==1.3.2+astro.1"}; !slices.Equal(b.Pins(), want) {
		t.Errorf("Pins() = %q, want %q", b.Pins(), want)
	}
	if want := []string{DistAirflow, DistCore, DistTaskSDK}; !slices.Equal(b.Dists(), want) {
		t.Errorf("Dists() = %q, want %q", b.Dists(), want)
	}
}

// The catalog says which release a runtime build carries; the newest build of
// that release is taken, not the newest build of the series.
func TestLookupAstroBuildFollowsTheRuntimeBuild(t *testing.T) {
	_, o := serveIndex(t, `{"runtimeVersionsV3": {
		"3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable"}},
		"3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable"}}}}`)

	if b := mustLookup(t, o, "3.3", "3.3-7", time.Time{}); b.Airflow != "3.3.1+astro.4" || b.TaskSDK != "1.3.1+astro.2" {
		t.Errorf("runtime 3.3-7: build = %+v, want 3.3.1+astro.4 with SDK 1.3.1+astro.2", b)
	}
	// The requirement decides: a runtime whose release the pin excludes does
	// not move the build off the pin.
	if b := mustLookup(t, o, "3.3.2", "3.3-7", time.Time{}); b.Airflow != "3.3.2+astro.1" {
		t.Errorf("pin 3.3.2 beside 3.3-7: build = %+v, want 3.3.2+astro.1", b)
	}
}

func TestLookupAstroBuildPinsOnlyAirflowForAirflow2(t *testing.T) {
	_, o := serveIndex(t, "")

	b := mustLookup(t, o, "2.11", "", time.Time{})

	if b.Airflow != "2.11.2+astro.7" || b.TaskSDK != "" {
		t.Errorf("build = %+v, want 2.11.2+astro.7 and no SDK", b)
	}
	if want := []string{DistAirflow}; !slices.Equal(b.Dists(), want) {
		t.Errorf("Dists() = %q, want %q", b.Dists(), want)
	}
}

func TestLookupAstroBuildSaysWhenThereIsNone(t *testing.T) {
	_, o := serveIndex(t, "")

	_, err := LookupAstroBuild(t.Context(), o, "3.4", "", time.Time{})

	if !errors.Is(err, ErrNoAstroBuild) {
		t.Errorf("err = %v, want ErrNoAstroBuild", err)
	}
}

// A pin to the plain release could not be told apart from one the project
// wrote, so a missing SDK build is no answer rather than half of one.
func TestLookupAstroBuildWithNoBuildOfTheSDK(t *testing.T) {
	ix, o := serveIndex(t, "")
	ix.pages["/v2/apache-airflow-task-sdk/"] = listing(
		"2026-08-31T10:00:00Z apache_airflow_task_sdk-1.3.1+astro.2-py3-none-any.whl")

	b, err := LookupAstroBuild(t.Context(), o, "3.3", "", time.Time{})
	if err == nil || errors.Is(err, ErrNoAstroBuild) {
		t.Errorf("build = %+v, err = %v; want no answer, and not a claim that no Airflow build exists", b, err)
	}
}

func TestLookupAstroBuildIgnoresBuildsPublishedAfterExcludeNewer(t *testing.T) {
	_, o := serveIndex(t, "")

	b := mustLookup(t, o, "3.3", "", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))

	if b.Airflow != "3.3.1+astro.4" || b.TaskSDK != "1.3.1+astro.2" {
		t.Errorf("build = %+v, want 3.3.1+astro.4 with SDK 1.3.1+astro.2", b)
	}
}

func TestLookupAstroBuildReusesAPageForADayAndAStaleOneWhenDown(t *testing.T) {
	ix, o := serveIndex(t, "")
	first := mustLookup(t, o, "3.3", "", time.Time{})
	hits := ix.hits.Load()
	if again := mustLookup(t, o, "3.3", "", time.Time{}); again != first || ix.hits.Load() != hits+1 {
		// +1: the catalog is asked each time, the pages are not.
		t.Errorf("a fresh cached page was fetched again (%d requests)", ix.hits.Load()-hits)
	}

	old := time.Now().Add(-48 * time.Hour)
	pages, err := filepath.Glob(filepath.Join(o.CacheDir, "airflow-index", "*.html"))
	if err != nil || len(pages) != 2 {
		t.Fatalf("cached pages = %v (%v), want two", pages, err)
	}
	for _, p := range pages {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	ix.down.Store(true)
	if b := mustLookup(t, o, "3.3", "", time.Time{}); b != first {
		t.Errorf("with the index down, build = %+v, want the stale copy's %+v", b, first)
	}
}

func TestLookupAstroBuildNeverKeepsAPageThatIsNotTheIndex(t *testing.T) {
	ix, o := serveIndex(t, "")
	ix.pages["/v2/apache-airflow/"] = "<html><body>Sign in to the network</body></html>"

	if _, err := LookupAstroBuild(t.Context(), o, "3.3", "", time.Time{}); err == nil || errors.Is(err, ErrNoAstroBuild) {
		t.Fatalf("err = %v, want the index reported unreadable", err)
	}
	ix.pages["/v2/apache-airflow/"] = airflowPage
	if b := mustLookup(t, o, "3.3", "", time.Time{}); b.Airflow != "3.3.2+astro.1" {
		t.Errorf("build = %+v; the portal's page was kept in the index's place", b)
	}
}

func TestParseExcludeNewer(t *testing.T) {
	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Time{
		"2026-09-24T00:00:00Z": day,
		"2026-09-24":           day,
		"1 week":               {},
		"":                     {},
	} {
		if got := ParseExcludeNewer(v); !got.Equal(want) {
			t.Errorf("ParseExcludeNewer(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestLookupAstroBuildWithTheIndexUnreachable(t *testing.T) {
	ix, o := serveIndex(t, "")
	ix.down.Store(true)

	_, err := LookupAstroBuild(t.Context(), o, "3.3", "", time.Time{})

	if err == nil || errors.Is(err, ErrNoAstroBuild) {
		t.Errorf("err = %v, want an unreadable index, which says nothing about whether a build exists", err)
	}
}

func TestPublicIndexURLDropsCredentials(t *testing.T) {
	t.Setenv(IndexURLEnv, "https://user:token@mirror.example.com/astro")
	if got, want := PublicIndexURL(), "https://mirror.example.com/astro/"; got != want {
		t.Fatalf("PublicIndexURL() = %q, want %q", got, want)
	}
	if got := IndexURL(); got != "https://user:token@mirror.example.com/astro/" {
		t.Fatalf("IndexURL() = %q, want the credentials kept for the lookup", got)
	}
}

func TestIndexErrorsLeaveOutTheMirrorsCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(IndexURLEnv, strings.Replace(srv.URL, "http://", "http://user:s3cret@", 1))
	_, err := Options{}.indexPage(t.Context(), DistAirflow)
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v, want an error without the token", err)
	}
}
