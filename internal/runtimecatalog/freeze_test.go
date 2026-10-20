package runtimecatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

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

// A build or Python becomes part of a URL and a file name, so one that could
// leave either is refused before a request.
func TestRuntimeFreezeRefusesANameThatIsNotAVersion(t *testing.T) {
	_, hits := freezeServer(t, nil)
	assert.Nil(t, RuntimeFreeze(context.Background(), "../../etc", []string{"3.12"}))
	assert.Nil(t, RuntimeFreeze(context.Background(), "3.3-9", []string{"3.12/../x"}))
	assert.Equal(t, int32(0), hits.Load())
}
