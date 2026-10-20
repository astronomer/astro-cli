package runtimecatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/astronomer/astro-cli/pkg/testing" // refuses any host but this machine
)

const sampleFreeze = "# pip freeze\n" +
	"apache-airflow==3.3.2+astro.2\n" +
	"PyYAML==6.0.3\n" +
	"psycopg2_binary==2.9.11\n" +
	"\n" +
	"openlineage-python @ https://example.invalid/openlineage.whl\n"

// freezeServer serves files by path and counts requests, with the cache root
// and the CDN address pointed at test-owned places.
func freezeServer(t *testing.T, files map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := freezeBaseURL
	freezeBaseURL = srv.URL + "/runtime-freeze"
	t.Cleanup(func() { freezeBaseURL = old })
	return srv, &hits
}

func TestRuntimeFreezeReadsNormalizedNames(t *testing.T) {
	freezeServer(t, map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.12.txt": sampleFreeze})
	got := RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12"})
	assert.Equal(t, []string{"apache-airflow", "pyyaml", "psycopg2-binary", "openlineage-python"}, got)
}

// The catalog names no Python for an Airflow 2 build, so the caller lists
// guesses; a 404 moves on to the next.
func TestRuntimeFreezeTriesTheNextPythonOnA404(t *testing.T) {
	_, hits := freezeServer(t, map[string]string{"/runtime-freeze/runtime-12.6.0-python-3.11.txt": "pandas==2.2.3\n"})
	got := RuntimeFreeze(context.Background(), "12.6.0", []string{"3.12", "3.11"})
	assert.Equal(t, []string{"pandas"}, got)
	assert.Equal(t, int32(2), hits.Load())
}

// A build's freeze never changes, so the second read is the cached copy, even
// with the CDN gone.
func TestRuntimeFreezeKeepsWhatItRead(t *testing.T) {
	srv, hits := freezeServer(t, map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.12.txt": sampleFreeze})
	require.NotEmpty(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12"}))
	srv.Close()
	assert.NotEmpty(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12"}))
	assert.Equal(t, int32(1), hits.Load())
	root := os.Getenv("XDG_CACHE_HOME")
	assert.FileExists(t, filepath.Join(root, "astro", freezeCacheDir, "runtime-3.3-9-python-3.12.txt"))
}

// Online, the expected Python is read from the CDN before a cached copy of a
// fallback answers, so the fallback never stands in for it.
func TestRuntimeFreezePrefersTheExpectedPythonOverACachedFallback(t *testing.T) {
	freezeServer(t, map[string]string{"/runtime-freeze/runtime-12.6.0-python-3.12.txt": "numpy==2.1.0\n"})
	dir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", freezeCacheDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "runtime-12.6.0-python-3.11.txt"), []byte("pandas==2.2.3\n"), 0o600))
	assert.Equal(t, []string{"numpy"}, RuntimeFreeze(context.Background(), "12.6.0", []string{"3.12", "3.11"}))
}

// Offline, a copy kept for a later Python still answers once the request for
// the first has failed.
func TestRuntimeFreezeFallsBackToACachedLaterPythonOffline(t *testing.T) {
	srv, hits := freezeServer(t, nil)
	dir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", freezeCacheDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "runtime-12.6.0-python-3.11.txt"), []byte("pandas==2.2.3\n"), 0o600))
	srv.Close()
	assert.Equal(t, []string{"pandas"}, RuntimeFreeze(context.Background(), "12.6.0", []string{"3.12", "3.11"}))
	assert.Equal(t, int32(0), hits.Load())
}

// A file past the size cap is refused, not read short, and not kept.
func TestRuntimeFreezeRefusesAFileTooLargeToReadWhole(t *testing.T) {
	big := strings.Repeat("pandas==2.2.3\n", freezeMaxBytes/14+1)
	freezeServer(t, map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.12.txt": big})
	assert.Nil(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12"}))
	assert.NoFileExists(t, filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", freezeCacheDir, "runtime-3.3-9-python-3.12.txt"))
}

func TestParseFreezeReportsALineTooLongToRead(t *testing.T) {
	_, err := parseFreeze([]byte("pandas==2.2.3\n" + strings.Repeat("x", freezeMaxBytes+1) + "\n"))
	assert.Error(t, err)
}

// Offline, the search stops at the first failure that is not a 404: one
// timeout, not one per Python.
func TestRuntimeFreezeStopsAtAFailureThatIsNotA404(t *testing.T) {
	srv, _ := freezeServer(t, nil)
	srv.Close()
	assert.Nil(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12", "3.11"}))
}

// A 200 that lists no package is an error page, and is neither used nor kept.
func TestRuntimeFreezeRefusesAPageWithNoPackages(t *testing.T) {
	freezeServer(t, map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.12.txt": "<html>\n<body>Sign in</body>\n</html>\n"})
	assert.Nil(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12"}))
	root := os.Getenv("XDG_CACHE_HOME")
	assert.NoFileExists(t, filepath.Join(root, "astro", freezeCacheDir, "runtime-3.3-9-python-3.12.txt"))
}

// RuntimePins reads the same file, keeping the versions, and says why it has
// none: the runtime pins come out for a build with no file, and stay when the
// file cannot be read.
func TestRuntimePinsKeepsTheVersions(t *testing.T) {
	t.Setenv("UV_EXCLUDE_NEWER", "")
	srv, _ := freezeServer(t, map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.12.txt": sampleFreeze})
	got, err := RuntimePins(context.Background(), "3.3-9", "3.12")
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow==3.3.2+astro.2", "PyYAML==6.0.3", "psycopg2_binary==2.9.11"}, got,
		"a direct reference names no version to hold a package to")

	_, err = RuntimePins(context.Background(), "10.5.0", "3.11")
	assert.ErrorIs(t, err, ErrFreezeNotPublished)

	srv.Close()
	_, err = RuntimePins(context.Background(), "3.3-10", "3.12")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrFreezeNotPublished, "not reaching the CDN says nothing about whether the build has a file")
	got, err = RuntimePins(context.Background(), "3.3-9", "3.12")
	require.NoError(t, err, "the copy the cache kept serves offline")
	assert.Len(t, got, 3)
}

// A freeze is of the present, so under a UV_EXCLUDE_NEWER cutoff, a date or a
// duration, none is read.
func TestRuntimePinsUnderAPinnedResolution(t *testing.T) {
	_, hits := freezeServer(t, map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.12.txt": sampleFreeze})
	for _, cutoff := range []string{"2026-09-01", "1 week"} {
		t.Setenv("UV_EXCLUDE_NEWER", cutoff)
		_, err := RuntimePins(context.Background(), "3.3-9", "3.12")
		assert.ErrorIs(t, err, ErrPinnedResolution, cutoff)
	}
	assert.Equal(t, int32(0), hits.Load())
}

// A 404 is remembered for a day, so a project on a runtime with no freeze does
// not ask on every start; after that it asks again.
func TestA404IsRememberedForADay(t *testing.T) {
	t.Setenv("UV_EXCLUDE_NEWER", "")
	_, hits := freezeServer(t, nil)
	for range 2 {
		_, err := RuntimePins(context.Background(), "10.5.0", "3.11")
		assert.ErrorIs(t, err, ErrFreezeNotPublished)
	}
	assert.Equal(t, int32(1), hits.Load(), "the second read is answered from the cache")

	marker := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", freezeCacheDir, "runtime-10.5.0-python-3.11.txt"+freezeMissingSuffix)
	old := time.Now().Add(-freezeMissingTTL - time.Minute)
	require.NoError(t, os.Chtimes(marker, old, old))
	_, err := RuntimePins(context.Background(), "10.5.0", "3.11")
	assert.ErrorIs(t, err, ErrFreezeNotPublished)
	assert.Equal(t, int32(2), hits.Load(), "a day on, it asks again")
}

// A build or Python becomes part of a URL and a file name, so one that could
// leave either is refused before a request.
func TestRuntimeFreezeRefusesANameThatIsNotAVersion(t *testing.T) {
	_, hits := freezeServer(t, nil)
	assert.Nil(t, RuntimeFreeze(context.Background(), "../../etc", []string{"3.12"}))
	assert.Nil(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12/../x"}))
	assert.Equal(t, int32(0), hits.Load())
}
