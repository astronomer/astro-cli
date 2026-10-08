package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/openapi"
)

// --- runList -----------------------------------------------------------------

func newTestSpecServer(t *testing.T, specJSON []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(specJSON)
	}))
}

func TestRunList(t *testing.T) {
	spec := map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "Test", "version": "1.0"},
		"paths": map[string]any{
			"/dags": map[string]any{
				"get": map[string]any{"operationId": "get_dags", "summary": "List DAGs", "tags": []string{"DAGs"}},
			},
			"/health": map[string]any{
				"get": map[string]any{"operationId": "health", "summary": "Health check"},
			},
		},
	}
	body, _ := json.Marshal(spec)
	ts := newTestSpecServer(t, body)
	defer ts.Close()

	t.Run("lists all endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, Format: cliout.FormatText, specCache: cache}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "/dags")
		assert.Contains(t, buf.String(), "/health")
		assert.Contains(t, buf.String(), "2 endpoints")
	})

	t.Run("filters endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, Format: cliout.FormatText, specCache: cache, Filter: "dags"}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "/dags")
		assert.Contains(t, buf.String(), "1 endpoint")
		assert.NotContains(t, buf.String(), "1 endpoints") // singular
	})

	t.Run("filter no matches", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, Format: cliout.FormatText, specCache: cache, Filter: "nonexistent"}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No endpoints found")
	})

	t.Run("verbose mode", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, Format: cliout.FormatText, specCache: cache, Verbose: true}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List DAGs")
		assert.Contains(t, buf.String(), "Health check")
	})

	t.Run("json output", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, specCache: cache, Format: cliout.FormatJSON}

		require.NoError(t, runList(opts))

		items := decodeEndpoints(t, buf.Bytes())
		require.Len(t, items, 2)

		byPath := map[string]map[string]any{}
		for _, it := range items {
			byPath[it["path"].(string)] = it
		}
		assert.Equal(t, "get_dags", byPath["/dags"]["operation_id"])
		assert.Equal(t, "GET", byPath["/dags"]["method"])
		assert.Equal(t, []any{"DAGs"}, byPath["/dags"]["tags"])
		// No human-readable trailer leaked into JSON output.
		assert.NotContains(t, buf.String(), "Found")
	})

	t.Run("json empty array when filter matches nothing", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, specCache: cache, Filter: "nonexistent", Format: cliout.FormatJSON}

		require.NoError(t, runList(opts))

		items := decodeEndpoints(t, buf.Bytes())
		assert.NotNil(t, items, `no match is "endpoints": [], not null`)
		assert.Empty(t, items)
	})
}

func TestRunList_EmptySpec(t *testing.T) {
	spec := map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "Test", "version": "1.0"},
		"paths":   map[string]any{},
	}
	body, _ := json.Marshal(spec)
	ts := newTestSpecServer(t, body)
	defer ts.Close()

	var buf bytes.Buffer
	cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
	opts := &ListOptions{Out: &buf, Format: cliout.FormatText, specCache: cache}

	err := runList(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoints found")
}

// decodeEndpoints reads what ls -o json and describe -o json print: the
// endpoints under "endpoints", with a count that agrees with them.
func decodeEndpoints(t *testing.T, out []byte) []map[string]any {
	t.Helper()
	var doc struct {
		Endpoints []map[string]any `json:"endpoints"`
		Count     *int             `json:"count"`
	}
	require.NoError(t, json.Unmarshal(out, &doc), "output must be a JSON object: %s", out)
	require.NotNil(t, doc.Count, "output has no count: %s", out)
	require.Len(t, doc.Endpoints, *doc.Count)
	return doc.Endpoints
}
