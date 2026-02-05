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

// --- NewDescribeCmd ----------------------------------------------------------

func TestNewDescribeCmd_Cloud(t *testing.T) {
	out := new(bytes.Buffer)
	cache := openapi.NewCache()
	cmd := NewDescribeCmd(out, cache, "cloud")

	assert.Equal(t, "describe <endpoint>", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("method"))
	assert.NotNil(t, cmd.Flags().Lookup("refresh"))
	assert.Contains(t, cmd.Example, "astro api cloud")
	assert.Contains(t, cmd.Example, "CreateDeployment")
}

func TestNewDescribeCmd_Airflow(t *testing.T) {
	out := new(bytes.Buffer)
	cache := openapi.NewCache()
	cmd := NewDescribeCmd(out, cache, "airflow")

	assert.Contains(t, cmd.Example, "astro api airflow")
	assert.Contains(t, cmd.Example, "get_dag")
}

// --- getTypeString -----------------------------------------------------------

func TestGetTypeString(t *testing.T) {
	tests := []struct {
		name     string
		schema   *openapi.Schema
		refName  string
		expected string
	}{
		{"nil schema", nil, "", "any"},
		{"with ref name", &openapi.Schema{Type: "object"}, "MyModel", "MyModel"},
		{"simple string", &openapi.Schema{Type: "string"}, "", "string"},
		{"string with format", &openapi.Schema{Type: "string", Format: "date-time"}, "", "string (date-time)"},
		{"integer", &openapi.Schema{Type: "integer"}, "", "integer"},
		{"empty type defaults to object", &openapi.Schema{}, "", "object"},
		{"array of strings", &openapi.Schema{Type: "array", Items: &openapi.Schema{Type: "string"}}, "", "array of string"},
		{"array with ref items", &openapi.Schema{Type: "array", Items: &openapi.Schema{Ref: "#/components/schemas/DAG"}}, "", "array of DAG"},
		{"array with empty items", &openapi.Schema{Type: "array", Items: &openapi.Schema{}}, "", "array of object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getTypeString(tt.schema, tt.refName))
		})
	}
}

// --- filterParams ------------------------------------------------------------

func TestFilterParams(t *testing.T) {
	params := []openapi.Parameter{
		{Name: "dag_id", In: "path"},
		{Name: "limit", In: "query"},
		{Name: "offset", In: "query"},
		{Name: "X-Auth", In: "header"},
	}

	path := filterParams(params, "path")
	assert.Len(t, path, 1)
	assert.Equal(t, "dag_id", path[0].Name)

	query := filterParams(params, "query")
	assert.Len(t, query, 2)

	header := filterParams(params, "header")
	assert.Len(t, header, 1)

	cookie := filterParams(params, "cookie")
	assert.Empty(t, cookie)
}

// --- printParam --------------------------------------------------------------

func TestPrintParam(t *testing.T) {
	t.Run("basic param", func(t *testing.T) {
		var buf bytes.Buffer
		p := openapi.Parameter{Name: "dag_id", In: "path", Required: true, Schema: &openapi.Schema{Type: "string"}}
		printParam(&buf, p)
		output := buf.String()
		assert.Contains(t, output, "dag_id")
		assert.Contains(t, output, "required")
		assert.Contains(t, output, "string")
	})

	t.Run("optional with description", func(t *testing.T) {
		var buf bytes.Buffer
		p := openapi.Parameter{Name: "limit", In: "query", Description: "Maximum items"}
		printParam(&buf, p)
		output := buf.String()
		assert.Contains(t, output, "limit")
		assert.Contains(t, output, "Maximum items")
		assert.NotContains(t, output, "required")
	})

	t.Run("with format", func(t *testing.T) {
		var buf bytes.Buffer
		p := openapi.Parameter{Name: "created_at", Schema: &openapi.Schema{Type: "string", Format: "date-time"}}
		printParam(&buf, p)
		assert.Contains(t, buf.String(), "date-time")
	})

	t.Run("array param", func(t *testing.T) {
		var buf bytes.Buffer
		p := openapi.Parameter{Name: "tags", Schema: &openapi.Schema{Type: "array", Items: &openapi.Schema{Type: "string"}}}
		printParam(&buf, p)
		assert.Contains(t, buf.String(), "array of string")
	})

	t.Run("with default and enum", func(t *testing.T) {
		var buf bytes.Buffer
		limit := 100
		p := openapi.Parameter{
			Name: "limit",
			Schema: &openapi.Schema{
				Type:    "integer",
				Default: limit,
				Enum:    []any{10, 50, 100},
			},
		}
		printParam(&buf, p)
		output := buf.String()
		assert.Contains(t, output, "Default:")
		assert.Contains(t, output, "Enum:")
	})

	t.Run("no schema defaults to string", func(t *testing.T) {
		var buf bytes.Buffer
		p := openapi.Parameter{Name: "q"}
		printParam(&buf, p)
		assert.Contains(t, buf.String(), "string")
	})
}

// --- printParameters ---------------------------------------------------------

func TestPrintParameters(t *testing.T) {
	t.Run("empty params", func(t *testing.T) {
		var buf bytes.Buffer
		printParameters(&buf, nil)
		assert.Empty(t, buf.String())
	})

	t.Run("grouped by location", func(t *testing.T) {
		var buf bytes.Buffer
		params := []openapi.Parameter{
			{Name: "id", In: "path"},
			{Name: "limit", In: "query"},
			{Name: "X-Custom", In: "header"},
		}
		printParameters(&buf, params)
		output := buf.String()
		assert.Contains(t, output, "Path Parameters")
		assert.Contains(t, output, "Query Parameters")
		assert.Contains(t, output, "Header Parameters")
	})
}

// --- printRequestBody --------------------------------------------------------

func TestPrintRequestBody(t *testing.T) {
	t.Run("required with description", func(t *testing.T) {
		var buf bytes.Buffer
		body := &openapi.RequestBody{
			Required:    true,
			Description: "The DAG to create",
			Content: map[string]openapi.MediaType{
				"application/json": {
					Schema: &openapi.Schema{
						Type: "object",
						Properties: map[string]openapi.Schema{
							"name": {Type: "string"},
						},
					},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printRequestBody(&buf, body, resolver)
		output := buf.String()
		assert.Contains(t, output, "Request Body")
		assert.Contains(t, output, "required")
		assert.Contains(t, output, "The DAG to create")
		assert.Contains(t, output, "name")
	})

	t.Run("with ref schema", func(t *testing.T) {
		var buf bytes.Buffer
		spec := &openapi.OpenAPISpec{
			Components: &openapi.Components{
				Schemas: map[string]openapi.Schema{
					"DAG": {
						Type:       "object",
						Properties: map[string]openapi.Schema{"dag_id": {Type: "string"}},
					},
				},
			},
		}
		body := &openapi.RequestBody{
			Content: map[string]openapi.MediaType{
				"application/json": {
					Schema: &openapi.Schema{Ref: "#/components/schemas/DAG"},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(spec)
		printRequestBody(&buf, body, resolver)
		output := buf.String()
		assert.Contains(t, output, "DAG")
		assert.Contains(t, output, "dag_id")
	})

	t.Run("no content", func(t *testing.T) {
		var buf bytes.Buffer
		body := &openapi.RequestBody{}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printRequestBody(&buf, body, resolver)
		assert.Contains(t, buf.String(), "Request Body")
	})
}

// --- printResponses ----------------------------------------------------------

func TestPrintResponses(t *testing.T) {
	t.Run("empty responses", func(t *testing.T) {
		var buf bytes.Buffer
		printResponses(&buf, nil, nil)
		assert.Empty(t, buf.String())
	})

	t.Run("success and error responses", func(t *testing.T) {
		var buf bytes.Buffer
		responses := map[string]openapi.Response{
			"200": {
				Description: "Successful response",
				Content: map[string]openapi.MediaType{
					"application/json": {
						Schema: &openapi.Schema{
							Type: "object",
							Properties: map[string]openapi.Schema{
								"id": {Type: "string"},
							},
						},
					},
				},
			},
			"404": {Description: "Not found"},
			"500": {Description: "Server error"},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printResponses(&buf, responses, resolver)
		output := buf.String()
		assert.Contains(t, output, "Responses")
		assert.Contains(t, output, "200")
		assert.Contains(t, output, "Successful response")
		assert.Contains(t, output, "404")
		assert.Contains(t, output, "500")
		assert.Contains(t, output, "id") // schema property from 200
	})

	t.Run("redirect status", func(t *testing.T) {
		var buf bytes.Buffer
		responses := map[string]openapi.Response{
			"301": {Description: "Moved permanently"},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printResponses(&buf, responses, resolver)
		assert.Contains(t, buf.String(), "301")
	})

	t.Run("non-numeric code", func(t *testing.T) {
		var buf bytes.Buffer
		responses := map[string]openapi.Response{
			"default": {Description: "Default error"},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printResponses(&buf, responses, resolver)
		assert.Contains(t, buf.String(), "default")
	})
}

// --- printSchema -------------------------------------------------------------

func TestPrintSchema(t *testing.T) {
	t.Run("nil schema", func(t *testing.T) {
		var buf bytes.Buffer
		printSchema(&buf, nil, nil, 0, nil, requestSchemaPrintOpts())
		assert.Empty(t, buf.String())
	})

	t.Run("simple properties", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]openapi.Schema{
				"name":        {Type: "string", Description: "The name"},
				"description": {Type: "string"},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "name")
		assert.Contains(t, output, "The name")
		assert.Contains(t, output, "description")
	})

	t.Run("skips read-only in request opts", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type: "object",
			Properties: map[string]openapi.Schema{
				"id":   {Type: "string", ReadOnly: true},
				"name": {Type: "string"},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.NotContains(t, output, "  id") // read-only skipped
		assert.Contains(t, output, "name")
	})

	t.Run("shows read-only in response opts", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type: "object",
			Properties: map[string]openapi.Schema{
				"id":   {Type: "string", ReadOnly: true},
				"name": {Type: "string"},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), responseSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "id")
		assert.Contains(t, output, "name")
	})

	t.Run("shows example and enum", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type: "object",
			Properties: map[string]openapi.Schema{
				"status": {
					Type:    "string",
					Example: "active",
					Enum:    []any{"active", "inactive"},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "Example: active")
		assert.Contains(t, output, "Enum:")
	})

	t.Run("shows default in request opts", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type: "object",
			Properties: map[string]openapi.Schema{
				"limit": {Type: "integer", Default: 100},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		assert.Contains(t, buf.String(), "Default: 100")
	})

	t.Run("hides default in response opts", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type: "object",
			Properties: map[string]openapi.Schema{
				"limit": {Type: "integer", Default: 100},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), responseSchemaPrintOpts())
		assert.NotContains(t, buf.String(), "Default:")
	})

	t.Run("oneOf composition", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			OneOf: []openapi.Schema{
				{Type: "string"},
				{Type: "integer"},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "One of the following")
		assert.Contains(t, output, "Option 1")
		assert.Contains(t, output, "Option 2")
	})

	t.Run("anyOf composition", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			AnyOf: []openapi.Schema{
				{Type: "string"},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		assert.Contains(t, buf.String(), "Any of the following")
	})

	t.Run("allOf composition", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			AllOf: []openapi.Schema{
				{
					Type:       "object",
					Properties: map[string]openapi.Schema{"base": {Type: "string"}},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "All of the following")
		assert.Contains(t, output, "base")
	})

	t.Run("ref cycle detection via composition", func(t *testing.T) {
		var buf bytes.Buffer
		spec := &openapi.OpenAPISpec{
			Components: &openapi.Components{
				Schemas: map[string]openapi.Schema{
					"Node": {
						OneOf: []openapi.Schema{
							{Ref: "#/components/schemas/Node"},
							{Type: "string"},
						},
					},
				},
			},
		}
		// Start with a $ref that resolves to Node (which has oneOf referencing itself)
		schema := &openapi.Schema{Ref: "#/components/schemas/Node"}
		resolver := openapi.NewSchemaResolver(spec)
		printSchema(&buf, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "see Node above")
	})

	t.Run("direct ref already visited", func(t *testing.T) {
		var buf bytes.Buffer
		spec := &openapi.OpenAPISpec{
			Components: &openapi.Components{
				Schemas: map[string]openapi.Schema{
					"Thing": {Type: "object", Properties: map[string]openapi.Schema{"id": {Type: "string"}}},
				},
			},
		}
		schema := &openapi.Schema{Ref: "#/components/schemas/Thing"}
		resolver := openapi.NewSchemaResolver(spec)
		// Pre-mark Thing as visited
		visited := map[string]bool{"Thing": true}
		printSchema(&buf, schema, resolver, 2, visited, requestSchemaPrintOpts())
		assert.Contains(t, buf.String(), "see Thing above")
	})

	t.Run("max indent stops recursion", func(t *testing.T) {
		var buf bytes.Buffer
		schema := &openapi.Schema{
			Type: "object",
			Properties: map[string]openapi.Schema{
				"nested": {
					Type: "object",
					Properties: map[string]openapi.Schema{
						"deep": {Type: "string"},
					},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		// Set indent at maxSchemaIndent so nested object properties are not expanded
		printSchema(&buf, schema, resolver, maxSchemaIndent, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "nested")
		assert.NotContains(t, output, "deep") // too deep
	})
}

// --- printEndpointDetails ----------------------------------------------------

func TestPrintEndpointDetails(t *testing.T) {
	t.Run("full endpoint", func(t *testing.T) {
		var buf bytes.Buffer
		ep := &openapi.Endpoint{
			Method:      "POST",
			Path:        "/dags",
			OperationID: "create_dag",
			Summary:     "Create a new DAG",
			Description: "Creates a DAG in the system",
			Tags:        []string{"DAGs"},
			Parameters: []openapi.Parameter{
				{Name: "dag_id", In: "path", Required: true, Schema: &openapi.Schema{Type: "string"}},
			},
			RequestBody: &openapi.RequestBody{
				Required: true,
				Content: map[string]openapi.MediaType{
					"application/json": {Schema: &openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{"name": {Type: "string"}}}},
				},
			},
			Responses: map[string]openapi.Response{
				"201": {Description: "Created"},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printEndpointDetails(&buf, ep, resolver)
		output := buf.String()

		assert.Contains(t, output, "/dags")
		assert.Contains(t, output, "create_dag")
		assert.Contains(t, output, "Create a new DAG")
		assert.Contains(t, output, "Creates a DAG in the system")
		assert.Contains(t, output, "DAGs")
		assert.Contains(t, output, "dag_id")
		assert.Contains(t, output, "Request Body")
		assert.Contains(t, output, "Responses")
	})

	t.Run("deprecated endpoint", func(t *testing.T) {
		var buf bytes.Buffer
		ep := &openapi.Endpoint{Method: "GET", Path: "/old", Deprecated: true}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printEndpointDetails(&buf, ep, resolver)
		assert.Contains(t, buf.String(), "DEPRECATED")
	})

	t.Run("no optional fields", func(t *testing.T) {
		var buf bytes.Buffer
		ep := &openapi.Endpoint{Method: "GET", Path: "/health"}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printEndpointDetails(&buf, ep, resolver)
		output := buf.String()
		assert.Contains(t, output, "/health")
		assert.NotContains(t, output, "Operation ID")
		assert.NotContains(t, output, "Tags")
	})

	t.Run("description same as summary is not duplicated", func(t *testing.T) {
		var buf bytes.Buffer
		ep := &openapi.Endpoint{
			Method:      "GET",
			Path:        "/test",
			Summary:     "Same text",
			Description: "Same text",
		}
		resolver := openapi.NewSchemaResolver(&openapi.OpenAPISpec{})
		printEndpointDetails(&buf, ep, resolver)
		// The string should appear exactly once for the summary, not twice
		output := buf.String()
		first := bytes.Index([]byte(output), []byte("Same text"))
		second := bytes.Index([]byte(output[first+1:]), []byte("Same text"))
		assert.Equal(t, -1, second, "description identical to summary should not be printed twice")
	})
}

// --- runDescribe -------------------------------------------------------------

func TestRunDescribe(t *testing.T) {
	spec := openapi.OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    openapi.Info{Title: "Test", Version: "1.0"},
		Paths: map[string]openapi.PathItem{
			"/dags": {
				Get:  &openapi.Operation{OperationID: "get_dags", Summary: "List DAGs"},
				Post: &openapi.Operation{OperationID: "create_dag", Summary: "Create DAG"},
			},
			"/health": {
				Get: &openapi.Operation{OperationID: "health"},
			},
		},
	}
	body, _ := json.Marshal(spec)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	t.Run("find by operation ID", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "get_dags"}

		err := runDescribe(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List DAGs")
	})

	t.Run("find by path", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "/dags"}

		err := runDescribe(opts)
		require.NoError(t, err)
		// Should show both GET and POST
		assert.Contains(t, buf.String(), "List DAGs")
		assert.Contains(t, buf.String(), "Create DAG")
	})

	t.Run("find by path without leading slash", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "health"}

		err := runDescribe(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "/health")
	})

	t.Run("filter by method", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "/dags", Method: "POST"}

		err := runDescribe(opts)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Create DAG")
		assert.NotContains(t, buf.String(), "List DAGs")
	})

	t.Run("not found", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "nonexistent"}

		err := runDescribe(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no endpoint found")
	})

	t.Run("method filter no match", func(t *testing.T) {
		var buf bytes.Buffer
		cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
		opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "/dags", Method: "DELETE"}

		err := runDescribe(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no endpoint found")
	})
}

func TestRunDescribe_EmptySpec(t *testing.T) {
	spec := openapi.OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    openapi.Info{Title: "Test", Version: "1.0"},
		Paths:   map[string]openapi.PathItem{},
	}
	body, _ := json.Marshal(spec)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	var buf bytes.Buffer
	cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")
	opts := &DescribeOptions{Out: &buf, specCache: cache, Endpoint: "/dags"}

	err := runDescribe(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoints found")
}

// --- requestSchemaPrintOpts / responseSchemaPrintOpts ------------------------

func TestSchemaPrintOpts(t *testing.T) {
	reqOpts := requestSchemaPrintOpts()
	assert.True(t, reqOpts.SkipReadOnly)
	assert.True(t, reqOpts.ShowComposition)
	assert.True(t, reqOpts.ShowDefault)
	assert.Equal(t, maxSchemaIndent, reqOpts.MaxIndent)

	respOpts := responseSchemaPrintOpts()
	assert.False(t, respOpts.SkipReadOnly)
	assert.True(t, respOpts.ShowComposition)
	assert.False(t, respOpts.ShowDefault)
	assert.Equal(t, maxSchemaIndent, respOpts.MaxIndent)
}
