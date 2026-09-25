package runtimeversions

import (
	"net/http"
	"testing"
	"time"
)

// A catalog whose newest series is far above any fallback this binary will
// carry, so a catalog answer can never be mistaken for the fallback.
const aheadOfTheFallback = `{"runtimeVersionsV3": {
	"3.50-1": {"metadata": {"airflowVersion": "3.50.0", "channel": "stable", "releaseDate": "2026-09-01", "pythonVersions": ["3.13", "3.14"]}},
	"3.51-1": {"metadata": {"airflowVersion": "3.51.0", "channel": "stable", "releaseDate": "2099-01-01", "pythonVersions": ["3.15"]}}}}`

func TestDefaultFromTheCatalog(t *testing.T) {
	standOn(t, "2026-09-25")
	s := serve(t, aheadOfTheFallback)

	series, rp, src := Default(t.Context(), Options{CacheDir: t.TempDir(), UserAgent: "astro-cli/test"})
	if series != "3.50" || rp != ">=3.13" || src != SourceCatalog {
		t.Errorf("Default = %q, %q, %q; want 3.50, >=3.13, catalog", series, rp, src)
	}
	if ua := s.agents(); len(ua) != 1 || ua[0] != "astro-cli/test" {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestDefaultFromTheCache(t *testing.T) {
	standOn(t, "2026-09-25")
	s := serve(t, "")
	s.status = http.StatusServiceUnavailable
	dir := t.TempDir()

	writeCached(t, dir, aheadOfTheFallback, time.Hour)
	if series, _, src := Default(t.Context(), Options{CacheDir: dir}); series != "3.50" || src != SourceCache {
		t.Errorf("fresh cache: %q, %q", series, src)
	}
	writeCached(t, dir, aheadOfTheFallback, 90*24*time.Hour)
	if series, _, src := Default(t.Context(), Options{CacheDir: dir}); series != "3.50" || src != SourceStaleCache {
		t.Errorf("stale cache: %q, %q", series, src)
	}
}

// With no catalog anywhere, the answer is the built-in series, and
// requires-python is left to the caller's rule.
func TestDefaultOffline(t *testing.T) {
	s := serve(t, "")
	s.status = http.StatusNotFound

	series, rp, src := Default(t.Context(), Options{CacheDir: t.TempDir()})
	if series != FallbackAirflowSeries || rp != "" || src != SourceFallback {
		t.Errorf("Default = %q, %q, %q; want the fallback", series, rp, src)
	}
}

func TestDefaultOnATimeout(t *testing.T) {
	s := serve(t, aheadOfTheFallback)
	s.delay = 10 * time.Second

	start := time.Now()
	series, _, src := Default(t.Context(), Options{Timeout: 50 * time.Millisecond})
	if series != FallbackAirflowSeries || src != SourceFallback {
		t.Errorf("Default = %q, %q; want the fallback", series, src)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Default took %s", took)
	}
}

// heldBack is a catalog in which Astronomer has withdrawn 3.3 after the
// fallback reached it: every 3.3 build is yanked or deprecated, so the newest
// supported series is 3.2, below the constant.
const heldBack = `{"runtimeVersionsV3": {
	"3.2-10": {"metadata": {"airflowVersion": "3.2.2", "channel": "stable", "releaseDate": "2026-09-24", "pythonVersions": ["3.12", "3.13", "3.14"]}},
	"3.3-1":  {"metadata": {"airflowVersion": "3.3.0", "channel": "deprecated", "releaseDate": "2026-07-09", "pythonVersions": ["3.12", "3.13", "3.14"]}},
	"3.3-8":  {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-09-23", "yanked": true, "pythonVersions": ["3.12", "3.13", "3.14"]}}}}`

// A current catalog is authoritative, below the constant included: holding a
// series back is done in the catalog, and the binary must not re-pin what the
// catalog withdrew, nor say it could not reach a catalog it just read.
func TestDefaultFollowsACatalogThatHoldsASeriesBack(t *testing.T) {
	standOn(t, "2026-09-25")
	if compareVersions("3.2", FallbackAirflowSeries) >= 0 {
		t.Fatalf("this case needs FallbackAirflowSeries above 3.2, and it is %s", FallbackAirflowSeries)
	}

	serve(t, heldBack)
	series, rp, src := Default(t.Context(), Options{})
	if series != "3.2" || rp != ">=3.12" || src != SourceCatalog {
		t.Errorf("fetched: Default = %q, %q, %q; want 3.2, >=3.12, catalog", series, rp, src)
	}

	s := serve(t, "")
	s.status = http.StatusServiceUnavailable
	dir := t.TempDir()
	writeCached(t, dir, heldBack, time.Hour)
	series, rp, src = Default(t.Context(), Options{CacheDir: dir})
	if series != "3.2" || rp != ">=3.12" || src != SourceCache {
		t.Errorf("fresh cache: Default = %q, %q, %q; want 3.2, >=3.12, cache", series, rp, src)
	}
	if s.callCount() != 0 {
		t.Error("a fresh cache made a request")
	}
}

// A stale copy is only a stand-in for a catalog the fetch could not reach, so it
// may not put a project below the series the binary knows. The floored answer
// is the binary's, and says so: SourceFallback, with requires-python left to
// the caller's built-in rule.
func TestDefaultFloorsAStaleCacheAtTheFallback(t *testing.T) {
	standOn(t, "2026-09-25")
	s := serve(t, "")
	s.status = http.StatusServiceUnavailable
	dir := t.TempDir()
	writeCached(t, dir, heldBack, 90*24*time.Hour)

	series, rp, src := Default(t.Context(), Options{CacheDir: dir})
	if series != FallbackAirflowSeries || rp != "" || src != SourceFallback {
		t.Errorf("Default = %q, %q, %q; want the fallback with no catalog Python", series, rp, src)
	}
}

func TestDefaultWithNoAirflow3InTheCatalog(t *testing.T) {
	standOn(t, "2026-09-25")
	serve(t, `{"runtimeVersions": {
		"13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable", "releaseDate": "2026-09-17"}}},
		"runtimeVersionsV3": {}}`)

	if series, _, src := Default(t.Context(), Options{}); series != FallbackAirflowSeries || src != SourceCatalogEmpty {
		t.Errorf("Default = %q, %q; want the fallback, reported as catalog-empty", series, src)
	}
}

// A fresh cache with nothing usable is a catalog that was read, so it says
// catalog-empty; a stale one with nothing usable stands in for a failed fetch,
// so it says fallback.
func TestDefaultTellsAnEmptyCatalogFromAnUnreachableOne(t *testing.T) {
	standOn(t, "2026-09-25")
	s := serve(t, "")
	s.status = http.StatusServiceUnavailable
	empty := `{"runtimeVersions": {"13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable", "releaseDate": "2026-09-17"}}}}`

	dir := t.TempDir()
	writeCached(t, dir, empty, time.Hour)
	if series, _, src := Default(t.Context(), Options{CacheDir: dir}); series != FallbackAirflowSeries || src != SourceCatalogEmpty {
		t.Errorf("fresh cache: Default = %q, %q; want catalog-empty", series, src)
	}
	writeCached(t, dir, empty, 90*24*time.Hour)
	if series, _, src := Default(t.Context(), Options{CacheDir: dir}); series != FallbackAirflowSeries || src != SourceFallback {
		t.Errorf("stale cache: Default = %q, %q; want fallback", series, src)
	}
}

// The recorded catalog's answer, the one the bump script is checked against in
// CI, so the Go rule and the jq rule cannot drift apart unseen.
func TestDefaultOnTheRecordedCatalog(t *testing.T) {
	standOn(t, "2026-09-25")
	serve(t, fixtureBody(t))

	series, rp, _ := Default(t.Context(), Options{})
	if series != "3.3" || rp != ">=3.12" {
		t.Errorf("Default = %q, %q; want 3.3, >=3.12", series, rp)
	}
}
