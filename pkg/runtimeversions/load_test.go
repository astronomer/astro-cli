package runtimeversions

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// service is a fake catalog server. Every test reaches it through URLEnv, so
// the override is exercised on every path, and none reaches the live endpoint.
type service struct {
	mu         sync.Mutex
	body       string
	status     int
	delay      time.Duration
	calls      int
	userAgents []string
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.calls++
	s.userAgents = append(s.userAgents, r.UserAgent())
	body, status, delay := s.body, s.status, s.delay
	s.mu.Unlock()
	if delay > 0 {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(delay):
		}
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	_, _ = w.Write([]byte(body))
}

func (s *service) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *service) agents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.userAgents...)
}

func serve(t *testing.T, body string) *service {
	t.Helper()
	s := &service{body: body}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	t.Setenv(URLEnv, srv.URL)
	return s
}

func fixtureBody(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeCached puts body in dir's cache file, aged by age.
func writeCached(t *testing.T, dir, body string, age time.Duration) string {
	t.Helper()
	path := CachePath(dir)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return path
}

// A fetched catalog is cached as the bytes served, so readers of the file that
// predate this package decode it unchanged, and the next Load asks nobody.
func TestLoadFetchesThenServesTheCache(t *testing.T) {
	body := fixtureBody(t)
	s := serve(t, body)
	dir := t.TempDir()

	if _, src, err := Load(t.Context(), Options{CacheDir: dir}); err != nil || src != SourceCatalog {
		t.Fatalf("first Load: src %q, err %v; want catalog", src, err)
	}
	cached, err := os.ReadFile(CachePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cached, []byte(body)) {
		t.Error("the cache does not hold the catalog as served")
	}

	if _, src, err := Load(t.Context(), Options{CacheDir: dir}); err != nil || src != SourceCache {
		t.Fatalf("second Load: src %q, err %v; want cache", src, err)
	}
	if n := s.callCount(); n != 1 {
		t.Errorf("service asked %d times, want 1: a fresh cache needs no request", n)
	}
}

func TestLoadRefreshesAStaleCache(t *testing.T) {
	s := serve(t, fixtureBody(t))
	dir := t.TempDir()
	path := writeCached(t, dir, `{"runtimeVersions": {"1.0.0": {"metadata": {"airflowVersion": "2.0.0"}}}}`, 25*time.Hour)

	if _, src, err := Load(t.Context(), Options{CacheDir: dir}); err != nil || src != SourceCatalog {
		t.Fatalf("Load: src %q, err %v; want catalog", src, err)
	}
	if s.callCount() != 1 {
		t.Error("a stale cache did not trigger a fetch")
	}
	if info, err := os.Stat(path); err != nil || time.Since(info.ModTime()) > time.Hour {
		t.Error("the refreshed catalog was not written back")
	}
}

func TestLoadFallsBackToAStaleCache(t *testing.T) {
	s := serve(t, "")
	s.status = http.StatusBadGateway
	dir := t.TempDir()
	writeCached(t, dir, fixtureBody(t), 30*24*time.Hour)

	c, src, err := Load(t.Context(), Options{CacheDir: dir})
	if err != nil || src != SourceStaleCache {
		t.Fatalf("Load: src %q, err %v; want stale-cache", src, err)
	}
	if _, ok := c.Runtime("3.3-8"); !ok {
		t.Error("the stale copy was not the one returned")
	}
}

// A corrupt cache, fresh or not, is passed over rather than trusted, and a
// successful fetch replaces it.
func TestLoadPassesOverACorruptCache(t *testing.T) {
	serve(t, fixtureBody(t))
	dir := t.TempDir()
	path := writeCached(t, dir, `{"runtimeVersionsV3": {`, 0)

	if _, src, err := Load(t.Context(), Options{CacheDir: dir}); err != nil || src != SourceCatalog {
		t.Fatalf("Load: src %q, err %v; want catalog", src, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "3.3-8") {
		t.Error("the corrupt cache was not replaced")
	}
}

func TestLoadFailsWithNoCopyAnywhere(t *testing.T) {
	s := serve(t, "")
	s.status = http.StatusInternalServerError
	dir := t.TempDir()
	writeCached(t, dir, `not json`, 30*24*time.Hour)

	_, _, err := Load(t.Context(), Options{CacheDir: dir})
	if err == nil || !strings.Contains(err.Error(), "Astro Runtime versions") || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("Load error = %v", err)
	}
}

// An answer that is not a catalog is neither used nor cached over a good copy.
func TestLoadDoesNotCacheABadAnswer(t *testing.T) {
	serve(t, `{"runtimeVersions": {}}`)
	dir := t.TempDir()
	path := writeCached(t, dir, fixtureBody(t), 30*24*time.Hour)

	if _, src, err := Load(t.Context(), Options{CacheDir: dir}); err != nil || src != SourceStaleCache {
		t.Fatalf("Load: src %q, err %v; want stale-cache", src, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "3.3-8") {
		t.Error("an empty answer overwrote the cache")
	}
}

func TestLoadWithoutACacheFetchesEveryTime(t *testing.T) {
	s := serve(t, fixtureBody(t))
	for range 2 {
		if _, src, err := Load(t.Context(), Options{}); err != nil || src != SourceCatalog {
			t.Fatalf("Load: src %q, err %v", src, err)
		}
	}
	if s.callCount() != 2 {
		t.Errorf("service asked %d times, want 2", s.callCount())
	}
}

func TestLoadTimesOut(t *testing.T) {
	s := serve(t, fixtureBody(t))
	s.delay = 10 * time.Second

	start := time.Now()
	_, _, err := Load(t.Context(), Options{Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("Load returned a catalog from a service that never answered")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Load took %s; the timeout did not bound it", took)
	}
}

func TestLoadSendsTheUserAgent(t *testing.T) {
	s := serve(t, fixtureBody(t))
	if _, _, err := Load(t.Context(), Options{UserAgent: "astro-cli/1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if ua := s.agents(); len(ua) != 1 || ua[0] != "astro-cli/1.2.3" {
		t.Errorf("User-Agent = %q, want astro-cli/1.2.3", ua)
	}
}

func TestURLOverride(t *testing.T) {
	t.Setenv(URLEnv, "")
	if got := URL(); got != "https://updates.astronomer.io/astronomer-runtime" {
		t.Errorf("URL() = %q with no override", got)
	}
	t.Setenv(URLEnv, " http://mirror.internal/runtime.json ")
	if got := URL(); got != "http://mirror.internal/runtime.json" {
		t.Errorf("URL() = %q with an override", got)
	}
}

// An override never reads the shared default cache: a fresh copy of the public
// catalog there is not served in place of the address the user named, and the
// override's answer is cached under a name of its own, leaving the file
// Astro Desktop and older CLIs read as the public catalog untouched.
func TestAnOverrideKeepsOutOfTheDefaultCache(t *testing.T) {
	dir := t.TempDir()
	public := `{"runtimeVersions": {"13.11.0": {"metadata": {"airflowVersion": "2.11.2"}}}}`
	defaultPath := filepath.Join(dir, CacheFile)
	if err := os.WriteFile(defaultPath, []byte(public), 0o600); err != nil {
		t.Fatal(err)
	}

	s := serve(t, fixtureBody(t))
	c, src, err := Load(t.Context(), Options{CacheDir: dir})
	if err != nil || src != SourceCatalog {
		t.Fatalf("Load: src %q, err %v; want the override fetched", src, err)
	}
	if _, ok := c.Runtime("3.3-8"); !ok || s.callCount() != 1 {
		t.Error("the override's catalog was not the one returned")
	}
	if got, _ := os.ReadFile(defaultPath); string(got) != public {
		t.Errorf("the override's answer was written into %s:\n%s", CacheFile, got)
	}
	if CachePath(dir) == defaultPath {
		t.Fatal("an override shares the default cache file")
	}
	if got, _ := os.ReadFile(CachePath(dir)); string(got) != fixtureBody(t) {
		t.Error("the override's answer was not cached under its own name")
	}
}

func TestCachePathIsKeyedByAddress(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(URLEnv, "")
	if got, want := CachePath(dir), filepath.Join(dir, "runtime-versions.json"); got != want {
		t.Errorf("default address: %q, want %q, the name Astro Desktop reads", got, want)
	}
	t.Setenv(URLEnv, "http://mirror.internal/a")
	a := CachePath(dir)
	t.Setenv(URLEnv, "http://mirror.internal/b")
	b := CachePath(dir)
	if a == b || !strings.HasPrefix(filepath.Base(a), "runtime-versions-") || len(filepath.Base(a)) != len("runtime-versions-")+12+len(".json") {
		t.Errorf("override paths %q and %q are not distinct runtime-versions-<12 hex>.json names", a, b)
	}
	if CachePath("") != "" {
		t.Error("no cache directory still named a file")
	}
}
