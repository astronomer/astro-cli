package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Constructors ------------------------------------------------------------

func TestNewCache(t *testing.T) {
	cache := NewCache()
	assert.Equal(t, SpecURL, cache.specURL)
	assert.NotEmpty(t, cache.cachePath)
}

func TestNewCacheWithOptions(t *testing.T) {
	cache := NewCacheWithOptions("https://example.com/spec", "/tmp/cache.json")
	assert.Equal(t, "https://example.com/spec", cache.specURL)
	assert.Equal(t, "/tmp/cache.json", cache.cachePath)
}

func TestNewAirflowCacheForVersion(t *testing.T) {
	t.Run("Airflow 3.x", func(t *testing.T) {
		cache, err := NewAirflowCacheForVersion("3.0.3")
		require.NoError(t, err)
		assert.Contains(t, cache.specURL, "3.0.3")
		assert.Contains(t, cache.cachePath, "3.0.3")
		assert.Equal(t, "/api/v2", cache.stripPrefix)
	})

	t.Run("Airflow 2.x", func(t *testing.T) {
		cache, err := NewAirflowCacheForVersion("2.10.0")
		require.NoError(t, err)
		assert.Equal(t, "/api/v1", cache.stripPrefix)
	})

	t.Run("invalid version", func(t *testing.T) {
		_, err := NewAirflowCacheForVersion("not-a-version")
		require.Error(t, err)
	})

	t.Run("with build metadata", func(t *testing.T) {
		cache, err := NewAirflowCacheForVersion("3.1.7+astro.1")
		require.NoError(t, err)
		fileName := filepath.Base(cache.cachePath)
		assert.Contains(t, fileName, "3.1.7")
		assert.NotContains(t, fileName, "astro")
	})
}

// --- getHTTPClient / SetHTTPClient -------------------------------------------

func TestGetHTTPClient_Default(t *testing.T) {
	cache := &Cache{}
	assert.Equal(t, http.DefaultClient, cache.getHTTPClient())
}

func TestGetHTTPClient_Custom(t *testing.T) {
	custom := &http.Client{Timeout: 5 * time.Second}
	cache := &Cache{}
	cache.SetHTTPClient(custom)
	assert.Equal(t, custom, cache.getHTTPClient())
}

// --- GetSpec / IsLoaded ------------------------------------------------------

func TestGetSpec_Nil(t *testing.T) {
	cache := &Cache{}
	assert.Nil(t, cache.GetSpec())
	assert.False(t, cache.IsLoaded())
}

func TestGetSpec_Loaded(t *testing.T) {
	spec := &OpenAPISpec{Info: Info{Title: "Test"}}
	cache := &Cache{spec: spec}
	assert.Equal(t, spec, cache.GetSpec())
	assert.True(t, cache.IsLoaded())
}

// --- GetEndpoints ------------------------------------------------------------

func TestGetEndpoints_NilSpec(t *testing.T) {
	cache := &Cache{}
	assert.Nil(t, cache.GetEndpoints())
}

func TestGetEndpoints_NoPrefix(t *testing.T) {
	cache := &Cache{
		spec: &OpenAPISpec{
			Paths: map[string]PathItem{
				"/dags": {Get: &Operation{OperationID: "get_dags"}},
			},
		},
	}
	endpoints := cache.GetEndpoints()
	assert.Len(t, endpoints, 1)
	assert.Equal(t, "/dags", endpoints[0].Path)
}

func TestGetEndpoints_WithPrefixStripping(t *testing.T) {
	cache := &Cache{
		spec: &OpenAPISpec{
			Paths: map[string]PathItem{
				"/api/v2/dags":    {Get: &Operation{OperationID: "get_dags"}},
				"/api/v2/version": {Get: &Operation{OperationID: "version"}},
			},
		},
		stripPrefix: "/api/v2",
	}
	endpoints := cache.GetEndpoints()
	assert.Len(t, endpoints, 2)
	for _, ep := range endpoints {
		assert.False(t, strings.HasPrefix(ep.Path, "/api/v2"), "prefix should be stripped from %s", ep.Path)
	}
}

// --- isExpired ---------------------------------------------------------------

func TestIsExpired(t *testing.T) {
	t.Run("fresh cache", func(t *testing.T) {
		cache := &Cache{fetchedAt: time.Now()}
		assert.False(t, cache.isExpired())
	})

	t.Run("expired cache", func(t *testing.T) {
		cache := &Cache{fetchedAt: time.Now().Add(-25 * time.Hour)}
		assert.True(t, cache.isExpired())
	})

	t.Run("zero time is expired", func(t *testing.T) {
		cache := &Cache{}
		assert.True(t, cache.isExpired())
	})
}

// --- readCache / saveCache round-trip ----------------------------------------

func TestCacheReadWriteRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    Info{Title: "Test API", Version: "1.0"},
		Paths: map[string]PathItem{
			"/test": {Get: &Operation{OperationID: "getTest"}},
		},
	}

	// Write
	writer := &Cache{
		cachePath: cachePath,
		spec:      spec,
		fetchedAt: time.Now(),
	}
	err := writer.saveCache()
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(cachePath)
	require.NoError(t, err)

	// Read
	reader := &Cache{cachePath: cachePath}
	err = reader.readCache()
	require.NoError(t, err)
	assert.Equal(t, "Test API", reader.spec.Info.Title)
	assert.False(t, reader.fetchedAt.IsZero())
}

func TestReadCache_FileNotFound(t *testing.T) {
	cache := &Cache{cachePath: "/nonexistent/path/cache.json"}
	err := cache.readCache()
	assert.Error(t, err)
}

func TestReadCache_CorruptData(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")
	require.NoError(t, os.WriteFile(cachePath, []byte("not json"), 0o600))

	cache := &Cache{cachePath: cachePath}
	err := cache.readCache()
	assert.Error(t, err)
}

// --- fetchSpec ---------------------------------------------------------------

func TestFetchSpec_JSON(t *testing.T) {
	spec := OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    Info{Title: "JSON Spec", Version: "1.0"},
		Paths:   map[string]PathItem{"/test": {Get: &Operation{OperationID: "test"}}},
	}
	body, _ := json.Marshal(spec)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "JSON Spec", cache.spec.Info.Title)
	assert.False(t, cache.fetchedAt.IsZero())
}

func TestFetchSpec_YAML(t *testing.T) {
	yamlBody := `openapi: "3.0.0"
info:
  title: YAML Spec
  version: "1.0"
paths:
  /test:
    get:
      operationId: test
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(yamlBody))
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "YAML Spec", cache.spec.Info.Title)
}

func TestFetchSpec_UnknownContentType_YAML(t *testing.T) {
	// YAML body with no helpful content-type — falls through to try YAML first
	yamlBody := `openapi: "3.0.0"
info:
  title: Unknown CT
  version: "1.0"
paths: {}
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(yamlBody))
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "Unknown CT", cache.spec.Info.Title)
}

func TestFetchSpec_UnknownContentType_JSON(t *testing.T) {
	// JSON body with no helpful content-type — YAML parse succeeds (JSON is valid YAML)
	spec := OpenAPISpec{OpenAPI: "3.0.0", Info: Info{Title: "JSON via unknown", Version: "1.0"}}
	body, _ := json.Marshal(spec)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "JSON via unknown", cache.spec.Info.Title)
}

func TestFetchSpec_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 404")
}

func TestFetchSpec_InvalidBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing OpenAPI spec as JSON")
}

func TestFetchSpec_InvalidBody_UnknownCT(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("<<<not yaml or json>>>"))
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tried YAML and JSON")
}

func TestFetchSpec_AcceptHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Accept"), "application/json")
		assert.Contains(t, r.Header.Get("Accept"), "application/yaml")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openapi":"3.0.0","info":{"title":"T","version":"1"},"paths":{}}`))
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
}

// --- Load --------------------------------------------------------------------

func TestLoad_FetchAndCache(t *testing.T) {
	spec := OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    Info{Title: "Load Test", Version: "1.0"},
		Paths:   map[string]PathItem{"/test": {Get: &Operation{OperationID: "test"}}},
	}
	body, _ := json.Marshal(spec)

	fetchCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cachePath := filepath.Join(t.TempDir(), "cache.json")
	cache := NewCacheWithOptions(ts.URL, cachePath)

	// First load fetches from server
	err := cache.Load(false)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount)
	assert.Equal(t, "Load Test", cache.GetSpec().Info.Title)

	// Second load uses cache (no additional fetch)
	err = cache.Load(false)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount)
}

func TestLoad_ForceRefresh(t *testing.T) {
	spec := OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    Info{Title: "Refresh", Version: "1.0"},
		Paths:   map[string]PathItem{},
	}
	body, _ := json.Marshal(spec)

	fetchCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cachePath := filepath.Join(t.TempDir(), "cache.json")
	cache := NewCacheWithOptions(ts.URL, cachePath)

	_ = cache.Load(false)
	assert.Equal(t, 1, fetchCount)

	// Force refresh bypasses cache
	_ = cache.Load(true)
	assert.Equal(t, 2, fetchCount)
}

func TestLoad_FetchFailWithStaleCache(t *testing.T) {
	spec := OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    Info{Title: "Stale", Version: "1.0"},
		Paths:   map[string]PathItem{},
	}
	body, _ := json.Marshal(spec)

	alive := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if alive {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	cachePath := filepath.Join(t.TempDir(), "cache.json")
	cache := NewCacheWithOptions(ts.URL, cachePath)

	// Load successfully
	err := cache.Load(false)
	require.NoError(t, err)

	// Server goes down; expire the cache to force a fetch attempt
	alive = false
	cache.fetchedAt = time.Now().Add(-25 * time.Hour)

	// Load should succeed using stale cache
	err = cache.Load(false)
	require.NoError(t, err)
	assert.Equal(t, "Stale", cache.GetSpec().Info.Title)
}

func TestLoad_FetchFailNoCache(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cachePath := filepath.Join(t.TempDir(), "cache.json")
	cache := NewCacheWithOptions(ts.URL, cachePath)

	err := cache.Load(false)
	require.Error(t, err)
}

// --- ClearCache --------------------------------------------------------------

func TestClearCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")
	require.NoError(t, os.WriteFile(cachePath, []byte(`{}`), 0o600))

	cache := &Cache{cachePath: cachePath}
	err := cache.ClearCache()
	require.NoError(t, err)

	_, err = os.Stat(cachePath)
	assert.True(t, os.IsNotExist(err))
}

func TestClearCache_NotFound(t *testing.T) {
	cache := &Cache{cachePath: "/nonexistent/cache.json"}
	err := cache.ClearCache()
	assert.Error(t, err)
}
