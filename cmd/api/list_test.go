package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewListCmd --------------------------------------------------------------

func TestNewListCmd_Cloud(t *testing.T) {
	out := new(bytes.Buffer)
	cache := openapi.NewCache()
	cmd := NewListCmd(out, cache, "cloud")

	assert.Equal(t, "ls [filter]", cmd.Use)
	assert.Contains(t, cmd.Aliases, "list")
	assert.Contains(t, cmd.Short, "Astro Cloud API")
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("refresh"))
	assert.Contains(t, cmd.Example, "astro api cloud")
}

func TestNewListCmd_Airflow(t *testing.T) {
	out := new(bytes.Buffer)
	cache := openapi.NewCache()
	cmd := NewListCmd(out, cache, "airflow")

	assert.Contains(t, cmd.Short, "Airflow API")
	assert.Contains(t, cmd.Example, "astro api airflow")
	assert.Contains(t, cmd.Example, "dags")
}

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

func newTestSpecServer(t *testing.T, spec *openapi.OpenAPISpec) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(spec)
	require.NoError(t, err)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func TestRunList(t *testing.T) {
	spec := &openapi.OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    openapi.Info{Title: "Test", Version: "1.0"},
		Paths: map[string]openapi.PathItem{
			"/dags": {
				Get: &openapi.Operation{OperationID: "get_dags", Summary: "List DAGs", Tags: []string{"DAGs"}},
			},
			"/health": {
				Get: &openapi.Operation{OperationID: "health", Summary: "Health check"},
			},
		},
	}
	ts := newTestSpecServer(t, spec)
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
	spec := &openapi.OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    openapi.Info{Title: "Test", Version: "1.0"},
		Paths:   map[string]openapi.PathItem{},
	}
	ts := newTestSpecServer(t, spec)
	defer ts.Close()

	var buf bytes.Buffer
	cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
	opts := &ListOptions{Out: &buf, specCache: cache}

	err := runList(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoints found")
}
