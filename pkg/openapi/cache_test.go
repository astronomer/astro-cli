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

// minimalSpecJSON returns a minimal valid OpenAPI 3.0 spec as JSON bytes.
func minimalSpecJSON(title string, paths map[string]map[string]map[string]any) []byte {
	spec := map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": title, "version": "1.0"},
		"paths":   paths,
	}
	data, _ := json.Marshal(spec)
	return data
}

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

func TestCloudCacheFileNameForDomain(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		expected string
	}{
		{"default domain", "astronomer.io", CloudCacheFileName},
		{"empty domain defaults", "", CloudCacheFileName},
		{"dev domain", "astronomer-dev.io", "openapi-cache-astronomer-dev_io.json"},
		{"stage domain", "astronomer-stage.io", "openapi-cache-astronomer-stage_io.json"},
		{"perf domain", "astronomer-perf.io", "openapi-cache-astronomer-perf_io.json"},
		{"PR preview domain", "pr1234.astronomer-dev.io", "openapi-cache-pr1234_astronomer-dev_io.json"},
		{"localhost", "localhost", "openapi-cache-localhost.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, CloudCacheFileNameForDomain(tt.domain))
		})
	}
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

// --- GetDoc / IsLoaded -------------------------------------------------------

func TestGetDoc_Nil(t *testing.T) {
	cache := &Cache{}
	assert.Nil(t, cache.GetDoc())
	assert.False(t, cache.IsLoaded())
}

func TestGetDoc_Loaded(t *testing.T) {
	body := minimalSpecJSON("Test", map[string]map[string]map[string]any{})
	doc, _, err := parseSpec(body)
	require.NoError(t, err)
	cache := &Cache{doc: doc}
	assert.NotNil(t, cache.GetDoc())
	assert.True(t, cache.IsLoaded())
}

// --- GetEndpoints ------------------------------------------------------------

func TestGetEndpoints_NilSpec(t *testing.T) {
	cache := &Cache{}
	assert.Nil(t, cache.GetEndpoints())
}

func TestGetEndpoints_NoPrefix(t *testing.T) {
	body := minimalSpecJSON("Test", map[string]map[string]map[string]any{
		"/dags": {"get": {"operationId": "get_dags"}},
	})
	doc, _, err := parseSpec(body)
	require.NoError(t, err)

	cache := &Cache{doc: doc}
	endpoints := cache.GetEndpoints()
	assert.Len(t, endpoints, 1)
	assert.Equal(t, "/dags", endpoints[0].Path)
}

func TestGetEndpoints_WithPrefixStripping(t *testing.T) {
	body := minimalSpecJSON("Test", map[string]map[string]map[string]any{
		"/api/v2/dags":    {"get": {"operationId": "get_dags"}},
		"/api/v2/version": {"get": {"operationId": "version"}},
	})
	doc, _, err := parseSpec(body)
	require.NoError(t, err)

	cache := &Cache{doc: doc, stripPrefix: "/api/v2"}
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

	rawSpec := minimalSpecJSON("Test API", map[string]map[string]map[string]any{
		"/test": {"get": {"operationId": "getTest"}},
	})
	doc, _, err := parseSpec(rawSpec)
	require.NoError(t, err)

	// Write
	writer := &Cache{
		cachePath: cachePath,
		doc:       doc,
		rawSpec:   rawSpec,
		fetchedAt: time.Now(),
	}
	err = writer.saveCache()
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(cachePath)
	require.NoError(t, err)

	// Read
	reader := &Cache{cachePath: cachePath}
	err = reader.readCache()
	require.NoError(t, err)
	assert.Equal(t, "Test API", reader.doc.Info.Title)
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
	body := minimalSpecJSON("JSON Spec", map[string]map[string]map[string]any{
		"/test": {"get": {"operationId": "test"}},
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "JSON Spec", cache.doc.Info.Title)
	assert.False(t, cache.fetchedAt.IsZero())
}

func TestFetchSpec_YAML(t *testing.T) {
	yamlBody := `openapi: "3.0.0"
info:
  title: YAML Spec
  version: "1.0"
paths: {}
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(yamlBody))
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "YAML Spec", cache.doc.Info.Title)
}

func TestFetchSpec_UnknownContentType_YAML(t *testing.T) {
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
	assert.Equal(t, "Unknown CT", cache.doc.Info.Title)
}

func TestFetchSpec_UnknownContentType_JSON(t *testing.T) {
	body := minimalSpecJSON("JSON via unknown", map[string]map[string]map[string]any{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := &Cache{specURL: ts.URL}
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "JSON via unknown", cache.doc.Info.Title)
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
	assert.Contains(t, err.Error(), "parsing OpenAPI spec")
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
	assert.Contains(t, err.Error(), "parsing OpenAPI spec")
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
	body := minimalSpecJSON("Load Test", map[string]map[string]map[string]any{
		"/test": {"get": {"operationId": "test"}},
	})

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
	assert.Equal(t, "Load Test", cache.GetDoc().Info.Title)

	// Second load uses cache (no additional fetch)
	err = cache.Load(false)
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount)
}

func TestLoad_ForceRefresh(t *testing.T) {
	body := minimalSpecJSON("Refresh", map[string]map[string]map[string]any{})

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
	body := minimalSpecJSON("Stale", map[string]map[string]map[string]any{})

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
	assert.Equal(t, "Stale", cache.GetDoc().Info.Title)
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

// --- parseSpec version detection ---------------------------------------------

func TestParseSpec_SwaggerV2(t *testing.T) {
	spec := []byte(`{"swagger":"2.0","info":{"title":"V2 Spec","version":"1.0"},"basePath":"/api/v1","paths":{"/test":{"get":{"operationId":"getTest","responses":{"200":{"description":"OK"}}}}}}`)
	v3doc, v2doc, err := parseSpec(spec)
	require.NoError(t, err)
	assert.Nil(t, v3doc)
	require.NotNil(t, v2doc)
	assert.Equal(t, "V2 Spec", v2doc.Info.Title)
	assert.Equal(t, "/api/v1", v2doc.BasePath)
}

func TestParseSpec_OpenAPIV3(t *testing.T) {
	spec := minimalSpecJSON("V3 Spec", map[string]map[string]map[string]any{})
	v3doc, v2doc, err := parseSpec(spec)
	require.NoError(t, err)
	require.NotNil(t, v3doc)
	assert.Nil(t, v2doc)
	assert.Equal(t, "V3 Spec", v3doc.Info.Title)
}

// --- GetServerPath -----------------------------------------------------------

func TestGetServerPath_V2(t *testing.T) {
	spec := []byte(`{"swagger":"2.0","info":{"title":"T","version":"1"},"basePath":"/api/v1","paths":{}}`)
	_, v2doc, err := parseSpec(spec)
	require.NoError(t, err)

	cache := &Cache{v2doc: v2doc}
	assert.Equal(t, "/api/v1", cache.GetServerPath())
}

func TestGetServerPath_V3(t *testing.T) {
	spec := []byte(`{"openapi":"3.0.0","info":{"title":"T","version":"1"},"servers":[{"url":"https://api.example.com/api/v2"}],"paths":{}}`)
	v3doc, _, err := parseSpec(spec)
	require.NoError(t, err)

	cache := &Cache{doc: v3doc}
	assert.Equal(t, "/api/v2", cache.GetServerPath())
}

func TestGetServerPath_V3_TrailingSlash(t *testing.T) {
	spec := []byte(`{"openapi":"3.0.0","info":{"title":"T","version":"1"},"servers":[{"url":"https://api.example.com/v1/"}],"paths":{}}`)
	v3doc, _, err := parseSpec(spec)
	require.NoError(t, err)

	cache := &Cache{doc: v3doc}
	assert.Equal(t, "/v1", cache.GetServerPath())
}

func TestGetServerPath_NoServers(t *testing.T) {
	cache := &Cache{}
	assert.Equal(t, "", cache.GetServerPath())
}

// --- NewCacheWithAuth --------------------------------------------------------

func TestNewCacheWithAuth(t *testing.T) {
	cache := NewCacheWithAuth("https://example.com/spec", "/tmp/cache.json", "my-token")
	assert.Equal(t, "https://example.com/spec", cache.specURL)
	assert.Equal(t, "/tmp/cache.json", cache.cachePath)
	assert.Equal(t, "my-token", cache.authToken)
}

// --- FetchSpec with auth -----------------------------------------------------

func TestFetchSpec_WithAuth(t *testing.T) {
	body := minimalSpecJSON("Auth Test", map[string]map[string]map[string]any{})

	var receivedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := NewCacheWithAuth(ts.URL, t.TempDir()+"/cache.json", "test-token-123")
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token-123", receivedAuth)
}

func TestFetchSpec_NoAuth(t *testing.T) {
	body := minimalSpecJSON("No Auth", map[string]map[string]map[string]any{})

	var receivedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
	err := cache.fetchSpec()
	require.NoError(t, err)
	assert.Equal(t, "", receivedAuth)
}

// --- SpecCacheFileName -------------------------------------------------------

func TestSpecCacheFileName(t *testing.T) {
	name1 := SpecCacheFileName("https://api.example.com/spec/v1.0")
	name2 := SpecCacheFileName("https://api.example.com/spec/v2")
	name3 := SpecCacheFileName("https://api.example.com/spec/v1.0")

	// Deterministic
	assert.Equal(t, name1, name3)
	// Different URLs produce different names
	assert.NotEqual(t, name1, name2)
	// Matches expected pattern
	assert.Regexp(t, `^openapi-cache-[0-9a-f]+\.json$`, name1)
}

// --- IsLoaded with v2 --------------------------------------------------------

func TestIsLoaded_V2(t *testing.T) {
	spec := []byte(`{"swagger":"2.0","info":{"title":"T","version":"1"},"basePath":"/","paths":{}}`)
	_, v2doc, err := parseSpec(spec)
	require.NoError(t, err)

	cache := &Cache{v2doc: v2doc}
	assert.True(t, cache.IsLoaded())
}

// --- GetEndpoints with v2 ----------------------------------------------------

func TestGetEndpoints_V2(t *testing.T) {
	spec := []byte(`{"swagger":"2.0","info":{"title":"T","version":"1"},"basePath":"/api/v1","paths":{"/orgs":{"get":{"operationId":"ListOrgs","responses":{"200":{"description":"OK"}}}}}}`)
	_, v2doc, err := parseSpec(spec)
	require.NoError(t, err)

	cache := &Cache{v2doc: v2doc}
	endpoints := cache.GetEndpoints()
	require.Len(t, endpoints, 1)
	assert.Equal(t, "/orgs", endpoints[0].Path)
	assert.Equal(t, "ListOrgs", endpoints[0].OperationID)
}
