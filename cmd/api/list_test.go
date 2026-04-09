package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/openapi"
)

// --- colorizeMethod ----------------------------------------------------------

func TestColorizeMethod(t *testing.T) {
	// Each HTTP method should return a non-empty string containing the method name
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result := colorizeMethod(m)
			assert.Contains(t, result, m)
		})
	}

	// Unknown method returns plain text
	assert.Equal(t, "TRACE", colorizeMethod("TRACE"))
}

// --- groupEndpointsByTag -----------------------------------------------------

func TestGroupEndpointsByTag(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{Method: "GET", Path: "/dags", Tags: []string{"DAGs"}},
		{Method: "POST", Path: "/dags", Tags: []string{"DAGs"}},
		{Method: "GET", Path: "/health", Tags: nil},
		{Method: "GET", Path: "/version", Tags: []string{}},
	}

	groups := groupEndpointsByTag(endpoints)

	assert.Len(t, groups["DAGs"], 2)
	assert.Len(t, groups[untaggedSection], 2) // /health and /version both untagged
}

func TestGroupEndpointsByTag_UsesFirstTag(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{Method: "GET", Path: "/test", Tags: []string{"Primary", "Secondary"}},
	}

	groups := groupEndpointsByTag(endpoints)
	assert.Len(t, groups["Primary"], 1)
	assert.Empty(t, groups["Secondary"])
}

// --- printEndpointsTable -----------------------------------------------------

func TestPrintEndpointsTable(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{Method: "GET", Path: "/dags", OperationID: "get_dags", Tags: []string{"DAGs"}},
		{Method: "POST", Path: "/dags", OperationID: "create_dag", Tags: []string{"DAGs"}},
		{Method: "GET", Path: "/health", Tags: nil},
	}

	var buf bytes.Buffer
	printEndpointsTable(&buf, endpoints)
	output := buf.String()

	assert.Contains(t, output, "DAGs")
	assert.Contains(t, output, "/dags")
	assert.Contains(t, output, "get_dags")
	assert.Contains(t, output, untaggedSection)
	assert.Contains(t, output, "/health")
}

func TestPrintEndpointsTable_DeprecatedEndpoint(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{Method: "GET", Path: "/old", Deprecated: true, Tags: []string{"API"}},
	}

	var buf bytes.Buffer
	printEndpointsTable(&buf, endpoints)
	assert.Contains(t, buf.String(), "deprecated")
}

// --- printEndpointsVerbose ---------------------------------------------------

func TestPrintEndpointsVerbose(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{
			Method:      "GET",
			Path:        "/dags/{dag_id}",
			OperationID: "get_dag",
			Summary:     "Get a DAG",
			Tags:        []string{"DAGs"},
		},
		{
			Method:     "DELETE",
			Path:       "/old",
			Deprecated: true,
		},
	}

	var buf bytes.Buffer
	printEndpointsVerbose(&buf, endpoints)
	output := buf.String()

	assert.Contains(t, output, "/dags/{dag_id}")
	assert.Contains(t, output, "get_dag")
	assert.Contains(t, output, "Get a DAG")
	assert.Contains(t, output, "DAGs")
	assert.Contains(t, output, "DEPRECATED")
	assert.Contains(t, output, "dag_id") // path parameter extracted
	assert.Contains(t, output, "---")    // separator between endpoints
}

func TestPrintEndpointsVerbose_NoOptionalFields(t *testing.T) {
	// Endpoint with no operation ID, summary, tags, or deprecation
	endpoints := []openapi.Endpoint{
		{Method: "GET", Path: "/health"},
	}

	var buf bytes.Buffer
	printEndpointsVerbose(&buf, endpoints)
	output := buf.String()

	assert.Contains(t, output, "/health")
	assert.NotContains(t, output, "Operation ID")
	assert.NotContains(t, output, "Summary")
	assert.NotContains(t, output, "Tags")
	assert.NotContains(t, output, "DEPRECATED")
}

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
		opts := &ListOptions{Out: &buf, specCache: cache}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "/dags")
		assert.Contains(t, buf.String(), "/health")
		assert.Contains(t, buf.String(), "2 endpoints")
	})

	t.Run("filters endpoints", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, specCache: cache, Filter: "dags"}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "/dags")
		assert.Contains(t, buf.String(), "1 endpoint")
		assert.NotContains(t, buf.String(), "1 endpoints") // singular
	})

	t.Run("filter no matches", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, specCache: cache, Filter: "nonexistent"}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No endpoints found")
	})

	t.Run("verbose mode", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &ListOptions{Out: &buf, specCache: cache, Verbose: true}

		err := runList(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List DAGs")
		assert.Contains(t, buf.String(), "Health check")
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
	opts := &ListOptions{Out: &buf, specCache: cache}

	err := runList(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoints found")
}
