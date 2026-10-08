package apirequest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/openapi"
)

// --- ListFilter --------------------------------------------------------------

func TestListFilterTakesEitherSpelling(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		flag string
		want string
	}{
		{"neither", nil, "", ""},
		{"positional, astro's spelling", []string{"dags"}, "", "dags"},
		{"--filter, af's spelling", nil, "dags", "dags"},
		{"both, agreeing", []string{"dags"}, "dags", "dags"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ListFilter(tc.args, tc.flag)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestListFilterRefusesTwoDifferentFilters(t *testing.T) {
	_, err := ListFilter([]string{"dags"}, "pools")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disagree")
}

// --- SpecJSON ----------------------------------------------------------------

// Airflow 3 serves its spec as JSON; the key order it wrote is kept.
func TestSpecJSONIndentsAJSONDocumentInItsOwnOrder(t *testing.T) {
	got, err := SpecJSON([]byte(`{"openapi":"3.1.0","info":{"title":"Airflow"},"paths":{}}`))
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"openapi\": \"3.1.0\",\n  \"info\": {\n    \"title\": \"Airflow\"\n  },\n  \"paths\": {}\n}\n", string(got))
}

// Airflow 2 serves YAML, which still comes out as JSON a pipe into jq can read.
func TestSpecJSONConvertsYAML(t *testing.T) {
	got, err := SpecJSON([]byte("openapi: 3.0.3\npaths:\n  /dags:\n    get:\n      operationId: get_dags\n"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(got, &doc))
	assert.Equal(t, "3.0.3", doc["openapi"])
	assert.Contains(t, doc["paths"], "/dags")
	assert.True(t, strings.HasSuffix(string(got), "}\n"))
}

func TestSpecJSONRefusesSomethingThatIsNeither(t *testing.T) {
	_, err := SpecJSON([]byte("<html>login</html>"))
	require.Error(t, err)
}

// --- Rows --------------------------------------------------------------------

func TestRowsAreNeverNil(t *testing.T) {
	got, err := json.Marshal(NewEndpointList(Rows(nil)))
	require.NoError(t, err)
	assert.JSONEq(t, `{"endpoints":[],"count":0}`, string(got))
}

// --- colorizeMethod ----------------------------------------------------------

func TestColorizeMethod(t *testing.T) {
	// Each HTTP method should return a non-empty string containing the method name
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result := ColorizeMethod(m)
			assert.Contains(t, result, m)
		})
	}

	// Unknown method returns plain text
	assert.Equal(t, "TRACE", ColorizeMethod("TRACE"))
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
