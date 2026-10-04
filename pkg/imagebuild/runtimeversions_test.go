package imagebuild

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// releasesJSON is a trimmed copy of the version service's answer: two Airflow
// releases served by several runtimes each, plus a yanked one that must never
// be chosen.
const releasesJSON = `{
  "runtimeVersions": {
    "9.1.0":   {"metadata": {"airflowVersion": "2.7.1", "channel": "deprecated"}},
    "11.10.0": {"metadata": {"airflowVersion": "2.9.3", "channel": "deprecated"}},
    "11.12.0": {"metadata": {"airflowVersion": "2.9.3", "channel": "deprecated"}},
    "11.9.0":  {"metadata": {"airflowVersion": "2.9.2", "channel": "deprecated"}},
    "13.9.0":  {"metadata": {"airflowVersion": "2.11.2", "channel": "stable"}},
    "13.99.0": {"metadata": {"airflowVersion": "2.9.3", "channel": "stable", "yanked": true}}
  },
  "runtimeVersionsV3": {
    "3.1-2": {"metadata": {"airflowVersion": "3.1.0", "channel": "stable"}}
  }
}`

// service is a fake runtime catalog, reached through the catalog's URL
// override, so no test touches the network or the user's home.
type service struct {
	mu        sync.Mutex
	body      string
	fail      bool
	calls     int
	userAgent string
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.userAgent = r.UserAgent()
	if s.fail {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte(s.body))
}

func (s *service) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *service) setFail() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = true
}

// withService serves body as the catalog and hands back an empty cache
// directory to resolve against. The service counts how often it was asked.
func withService(t *testing.T, body string, fail bool) (svc *service, cacheDir string) {
	t.Helper()
	svc = &service{body: body, fail: fail}
	srv := httptest.NewServer(svc)
	t.Cleanup(srv.Close)
	t.Setenv(runtimeversions.URLEnv, srv.URL)
	return svc, t.TempDir()
}

func TestLocalRuntimeImageAirflow3IsUnchanged(t *testing.T) {
	calls, cache := withService(t, releasesJSON, false)

	ref, err := LocalRuntimeImageWith(t.Context(), "3.1", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1", ref)
	assert.Zero(t, calls.count(), "Airflow 3 resolves from the pin alone, with no lookup")

	// Docker-mode local start takes the series rule with deploy: a patch pin
	// builds FROM its series and a bare major is refused.
	ref, err = LocalRuntimeImageWith(t.Context(), "3.1.2", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1", ref)
	_, err = LocalRuntimeImageWith(t.Context(), "3", "", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "Airflow pin")
	assert.Zero(t, calls.count())
}

func TestLocalRuntimeImageAirflow2(t *testing.T) {
	_, cache := withService(t, releasesJSON, false)
	ctx := t.Context()

	// The newest runtime carrying the pinned Airflow wins, and 11.12.0 beats
	// 11.10.0 by segment, not by string order.
	ref, err := LocalRuntimeImageWith(ctx, "2.9.3", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)

	// A partial pin matches every release under it, so 2.9 takes 2.9.3 over
	// 2.9.2 — and 13.99.0 is yanked, so it never wins.
	ref, err = LocalRuntimeImageWith(ctx, "2.9", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)

	// A bare major pin takes the newest of the whole line.
	ref, err = LocalRuntimeImageWith(ctx, "2", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:13.9.0", ref)
}

// An exact Airflow 2 pin selects a runtime carrying exactly that release. The
// manifest reads ==2.9 as 2.9.0, as pip and uv do, so it no longer means the
// newest 2.9; with no runtime for 2.9.0 the error says how to pin the series.
// ==2 is 2.0.0, below the Docker-mode floor, and gets the same hint.
func TestLocalRuntimeImageAirflow2ExactPins(t *testing.T) {
	_, cache := withService(t, releasesJSON, false)
	ctx := t.Context()

	ref, err := LocalRuntimeImageWith(ctx, "2.9.2", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.9.0", ref, "an exact pin takes the runtime for that release, not the newest 2.9")

	_, err = LocalRuntimeImageWith(ctx, "2.9.0", "", "", runtimeversions.Options{CacheDir: cache})
	require.ErrorIs(t, err, ErrNoRuntimeForAirflow)
	assert.ErrorContains(t, err, "for the newest Airflow 2.9, pin apache-airflow==2.9.*")

	_, err = LocalRuntimeImageWith(ctx, "2.0.0", "", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "Airflow 2.7 or later")
	assert.ErrorContains(t, err, "for the newest Airflow 2, pin apache-airflow==2.*")

	// A patch that is not .0 was written as a patch, and gets no hint.
	_, err = LocalRuntimeImageWith(ctx, "2.8.4", "", "", runtimeversions.Options{CacheDir: cache})
	require.ErrorIs(t, err, ErrNoRuntimeForAirflow)
	assert.NotContains(t, err.Error(), "for the newest")
}

// Docker mode's database service runs `airflow db migrate`, which arrived in
// Airflow 2.7. Both sides of that floor: 2.7 resolves, 2.6 is refused with a
// sentence instead of a container that exits 2, and a pin naming no minor is
// not refused at all — it means the newest Airflow 2.
func TestLocalRuntimeImageAirflow2Floor(t *testing.T) {
	calls, cache := withService(t, releasesJSON, false)
	ctx := t.Context()

	ref, err := LocalRuntimeImageWith(ctx, "2.7.1", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:9.1.0", ref)

	_, err = LocalRuntimeImageWith(ctx, "2.6.3", "", "", runtimeversions.Options{CacheDir: cache})
	require.Error(t, err)
	assert.ErrorContains(t, err, "Airflow 2.7 or later")
	assert.ErrorContains(t, err, "2.6.3", "the message must name the pin the project carries")

	// A minor below the floor is refused however few segments follow it.
	_, err = LocalRuntimeImageWith(ctx, "2.6", "", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "Airflow 2.7 or later")

	before := calls.count()
	_, err = LocalRuntimeImageWith(ctx, "2.0.2", "", "", runtimeversions.Options{CacheDir: cache})
	require.Error(t, err)
	assert.Equal(t, before, calls.count(), "a pin below the floor is refused without a lookup")

	// A bare major means the newest Airflow 2, which is above the floor.
	ref, err = LocalRuntimeImageWith(ctx, "2", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:13.9.0", ref)
}

func TestLocalRuntimeImageUnknownAirflow(t *testing.T) {
	_, cache := withService(t, releasesJSON, false)

	// Above the 2.7 floor, so this reaches the lookup and finds nothing.
	_, err := LocalRuntimeImageWith(t.Context(), "2.8.4", "", "", runtimeversions.Options{CacheDir: cache})
	require.ErrorIs(t, err, ErrNoRuntimeForAirflow)
	assert.ErrorContains(t, err, "2.8.4")
}

func TestLocalRuntimeImageRejectsOtherMajors(t *testing.T) {
	_, cache := withService(t, releasesJSON, false)

	_, err := LocalRuntimeImageWith(t.Context(), "1.10.15", "", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "Airflow 2 or Airflow 3")

	_, err = LocalRuntimeImageWith(t.Context(), "", "", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "no Airflow version")
}

func TestLocalRuntimeImageCachesTheAnswer(t *testing.T) {
	calls, cache := withService(t, releasesJSON, false)
	ctx := t.Context()

	_, err := LocalRuntimeImageWith(ctx, "2.9.3", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	_, err = LocalRuntimeImageWith(ctx, "2.11.2", "", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, 1, calls.count(), "the second lookup should read the cache")
}

// A caller that names no cache directory gets no cache: every lookup asks the
// service, and nothing is written anywhere.
func TestLocalRuntimeImageWithoutACache(t *testing.T) {
	calls, _ := withService(t, releasesJSON, false)
	ctx := t.Context()

	ref, err := LocalRuntimeImageWith(ctx, "2.9.3", "", "", runtimeversions.Options{CacheDir: ""})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)

	_, err = LocalRuntimeImageWith(ctx, "2.9.3", "", "", runtimeversions.Options{CacheDir: ""})
	require.NoError(t, err)
	assert.Equal(t, 2, calls.count(), "with no cache directory every lookup asks the service")
}

func TestLocalRuntimeImageFallsBackToAStaleCache(t *testing.T) {
	// A good answer lands in the cache first.
	svc, dir := withService(t, releasesJSON, false)
	_, err := LocalRuntimeImageWith(t.Context(), "2.9.3", "", "", runtimeversions.Options{CacheDir: dir})
	require.NoError(t, err)

	// Age it past the TTL, then take the network away. An old copy still names
	// every runtime that existed when it was written, so the start proceeds.
	path := runtimeversions.CachePath(dir)
	old := time.Now().Add(-2 * runtimeversions.CacheTTL)
	require.NoError(t, os.Chtimes(path, old, old))
	svc.setFail()

	ref, err := LocalRuntimeImageWith(t.Context(), "2.9.3", "", "", runtimeversions.Options{CacheDir: dir})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)
}

func TestLocalRuntimeImageReportsAFailedLookup(t *testing.T) {
	_, cache := withService(t, "", true)

	_, err := LocalRuntimeImageWith(t.Context(), "2.9.3", "", "", runtimeversions.Options{CacheDir: cache})
	require.Error(t, err)
	assert.ErrorContains(t, err, "Astro Runtime versions")
	assert.ErrorContains(t, err, "HTTP 503")
}

func TestLocalRuntimeImageRejectsAnEmptyServiceAnswer(t *testing.T) {
	_, cache := withService(t, `{"runtimeVersions": {}}`, false)

	_, err := LocalRuntimeImageWith(t.Context(), "2.9.3", "", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "listed no runtimes")
}

// The caller's User-Agent reaches the catalog, so Astronomer can tell which
// client read it.
func TestLocalRuntimeImageWithSendsTheUserAgent(t *testing.T) {
	svc, cache := withService(t, releasesJSON, false)

	_, err := LocalRuntimeImageWith(t.Context(), "2.9.3", "", "", runtimeversions.Options{CacheDir: cache, UserAgent: "astro-cli/9.9.9"})
	require.NoError(t, err)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	assert.Equal(t, "astro-cli/9.9.9", svc.userAgent)
}

// A [tool.astro] runtime names the base outright, for either generation, and
// nothing is looked up: an Airflow 2 build is already a runtime version.
func TestLocalRuntimeImageUsesTheRuntimeBuild(t *testing.T) {
	calls, cache := withService(t, releasesJSON, false)
	ctx := t.Context()

	ref, err := LocalRuntimeImageWith(ctx, "3.3", "3.3-8", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.3-8", ref)

	ref, err = LocalRuntimeImageWith(ctx, "2.9", "11.12.0", "", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)
	assert.Zero(t, calls.count(), "a named build needs no lookup")

	// The pin's own rules still hold with a build named.
	_, err = LocalRuntimeImageWith(ctx, "2.6", "8.0.0", "", runtimeversions.Options{CacheDir: cache})
	assert.ErrorContains(t, err, "Airflow 2.7 or later")
}
