package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func typesPtr(t string) *openapi3.Types { return &openapi3.Types{t} }

// --- getTypeString -----------------------------------------------------------

func TestGetTypeString(t *testing.T) {
	tests := []struct {
		name     string
		schema   *openapi3.Schema
		refName  string
		expected string
	}{
		{"nil schema", nil, "", "any"},
		{"with ref name", &openapi3.Schema{Type: typesPtr("object")}, "MyModel", "MyModel"},
		{"simple string", &openapi3.Schema{Type: typesPtr("string")}, "", "string"},
		{"string with format", &openapi3.Schema{Type: typesPtr("string"), Format: "date-time"}, "", "string (date-time)"},
		{"integer", &openapi3.Schema{Type: typesPtr("integer")}, "", "integer"},
		{"empty type defaults to object", &openapi3.Schema{}, "", "object"},
		{"array of strings", &openapi3.Schema{Type: typesPtr("array"), Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: typesPtr("string")}}}, "", "array of string"},
		{"array with ref items", &openapi3.Schema{Type: typesPtr("array"), Items: &openapi3.SchemaRef{Ref: "#/components/schemas/DAG"}}, "", "array of DAG"},
		{"array with empty items", &openapi3.Schema{Type: typesPtr("array"), Items: &openapi3.SchemaRef{Value: &openapi3.Schema{}}}, "", "array of object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getTypeString(tt.schema, tt.refName))
		})
	}
}

// --- filterParams ------------------------------------------------------------

func TestFilterParams(t *testing.T) {
	params := openapi3.Parameters{
		{Value: &openapi3.Parameter{Name: "dag_id", In: "path"}},
		{Value: &openapi3.Parameter{Name: "limit", In: "query"}},
		{Value: &openapi3.Parameter{Name: "offset", In: "query"}},
		{Value: &openapi3.Parameter{Name: "X-Auth", In: "header"}},
	}

	path := filterParams(params, "path")
	assert.Len(t, path, 1)
	assert.Equal(t, "dag_id", path[0].Value.Name)

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
		p := &openapi3.Parameter{Name: "dag_id", In: "path", Required: true, Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: typesPtr("string")}}}
		printParam(&buf, p)
		output := buf.String()
		assert.Contains(t, output, "dag_id")
		assert.Contains(t, output, "required")
		assert.Contains(t, output, "string")
	})

	t.Run("optional with description", func(t *testing.T) {
		var buf bytes.Buffer
		p := &openapi3.Parameter{Name: "limit", In: "query", Description: "Maximum items"}
		printParam(&buf, p)
		output := buf.String()
		assert.Contains(t, output, "limit")
		assert.Contains(t, output, "Maximum items")
		assert.NotContains(t, output, "required")
	})

	t.Run("with format", func(t *testing.T) {
		var buf bytes.Buffer
		p := &openapi3.Parameter{Name: "created_at", Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: typesPtr("string"), Format: "date-time"}}}
		printParam(&buf, p)
		assert.Contains(t, buf.String(), "date-time")
	})

	t.Run("array param", func(t *testing.T) {
		var buf bytes.Buffer
		p := &openapi3.Parameter{Name: "tags", Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: typesPtr("array"), Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: typesPtr("string")}}}}}
		printParam(&buf, p)
		assert.Contains(t, buf.String(), "array of string")
	})

	t.Run("with default and enum", func(t *testing.T) {
		var buf bytes.Buffer
		limit := 100
		p := &openapi3.Parameter{
			Name: "limit",
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
				Type:    typesPtr("integer"),
				Default: limit,
				Enum:    []any{10, 50, 100},
			}},
		}
		printParam(&buf, p)
		output := buf.String()
		assert.Contains(t, output, "Default:")
		assert.Contains(t, output, "Enum:")
	})

	t.Run("no schema defaults to string", func(t *testing.T) {
		var buf bytes.Buffer
		p := &openapi3.Parameter{Name: "q"}
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
		params := openapi3.Parameters{
			{Value: &openapi3.Parameter{Name: "id", In: "path"}},
			{Value: &openapi3.Parameter{Name: "limit", In: "query"}},
			{Value: &openapi3.Parameter{Name: "X-Custom", In: "header"}},
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
		bodyRef := &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Required:    true,
				Description: "The DAG to create",
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
							Type: typesPtr("object"),
							Properties: map[string]*openapi3.SchemaRef{
								"name": {Value: &openapi3.Schema{Type: typesPtr("string")}},
							},
						}},
					},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printRequestBody(&buf, bodyRef, resolver)
		output := buf.String()
		assert.Contains(t, output, "Request Body")
		assert.Contains(t, output, "required")
		assert.Contains(t, output, "The DAG to create")
		assert.Contains(t, output, "name")
	})

	t.Run("with ref schema", func(t *testing.T) {
		var buf bytes.Buffer
		dagSchema := &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"dag_id": {Value: &openapi3.Schema{Type: typesPtr("string")}},
			},
		}
		spec := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{"DAG": {Ref: "#/components/schemas/DAG", Value: dagSchema}},
			},
		}
		bodyRef := &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/DAG", Value: dagSchema},
					},
				},
			},
		}
		resolver := openapi.NewSchemaResolver(spec)
		printRequestBody(&buf, bodyRef, resolver)
		output := buf.String()
		assert.Contains(t, output, "DAG")
		assert.Contains(t, output, "dag_id")
	})

	t.Run("no content", func(t *testing.T) {
		var buf bytes.Buffer
		bodyRef := &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printRequestBody(&buf, bodyRef, resolver)
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
		responses := openapi3.NewResponsesWithCapacity(3)
		responses.Set("200", &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: strPtr("Successful response"),
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
							Type: typesPtr("object"),
							Properties: map[string]*openapi3.SchemaRef{
								"id": {Value: &openapi3.Schema{Type: typesPtr("string")}},
							},
						}},
					},
				},
			},
		})
		responses.Set("404", &openapi3.ResponseRef{Value: &openapi3.Response{Description: strPtr("Not found")}})
		responses.Set("500", &openapi3.ResponseRef{Value: &openapi3.Response{Description: strPtr("Server error")}})
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printResponses(&buf, responses, resolver)
		output := buf.String()
		assert.Contains(t, output, "Responses")
		assert.Contains(t, output, "200")
		assert.Contains(t, output, "Successful response")
		assert.Contains(t, output, "404")
		assert.Contains(t, output, "500")
		assert.Contains(t, output, "id")
	})

	t.Run("redirect status", func(t *testing.T) {
		var buf bytes.Buffer
		responses := openapi3.NewResponsesWithCapacity(1)
		responses.Set("301", &openapi3.ResponseRef{Value: &openapi3.Response{Description: strPtr("Moved permanently")}})
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printResponses(&buf, responses, resolver)
		assert.Contains(t, buf.String(), "301")
	})

	t.Run("non-numeric code", func(t *testing.T) {
		var buf bytes.Buffer
		responses := openapi3.NewResponsesWithCapacity(1)
		responses.Set("default", &openapi3.ResponseRef{Value: &openapi3.Response{Description: strPtr("Default error")}})
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
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
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:     typesPtr("object"),
			Required: []string{"name"},
			Properties: map[string]*openapi3.SchemaRef{
				"name":        {Value: &openapi3.Schema{Type: typesPtr("string"), Description: "The name"}},
				"description": {Value: &openapi3.Schema{Type: typesPtr("string")}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "name")
		assert.Contains(t, output, "The name")
		assert.Contains(t, output, "description")
	})

	t.Run("skips read-only in request opts", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"id":   {Value: &openapi3.Schema{Type: typesPtr("string"), ReadOnly: true}},
				"name": {Value: &openapi3.Schema{Type: typesPtr("string")}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.NotContains(t, output, "  id")
		assert.Contains(t, output, "name")
	})

	t.Run("shows read-only in response opts", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"id":   {Value: &openapi3.Schema{Type: typesPtr("string"), ReadOnly: true}},
				"name": {Value: &openapi3.Schema{Type: typesPtr("string")}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), responseSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "id")
		assert.Contains(t, output, "name")
	})

	t.Run("shows example and enum", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"status": {Value: &openapi3.Schema{
					Type:    typesPtr("string"),
					Example: "active",
					Enum:    []any{"active", "inactive"},
				}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "Example: active")
		assert.Contains(t, output, "Enum:")
	})

	t.Run("shows default in request opts", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"limit": {Value: &openapi3.Schema{Type: typesPtr("integer"), Default: 100}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		assert.Contains(t, buf.String(), "Default: 100")
	})

	t.Run("hides default in response opts", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"limit": {Value: &openapi3.Schema{Type: typesPtr("integer"), Default: 100}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), responseSchemaPrintOpts())
		assert.NotContains(t, buf.String(), "Default:")
	})

	t.Run("oneOf composition", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			OneOf: openapi3.SchemaRefs{
				{Value: &openapi3.Schema{Type: typesPtr("string")}},
				{Value: &openapi3.Schema{Type: typesPtr("integer")}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "One of the following")
		assert.Contains(t, output, "Option 1")
		assert.Contains(t, output, "Option 2")
	})

	t.Run("anyOf composition", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			AnyOf: openapi3.SchemaRefs{
				{Value: &openapi3.Schema{Type: typesPtr("string")}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		assert.Contains(t, buf.String(), "Any of the following")
	})

	t.Run("allOf composition", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			AllOf: openapi3.SchemaRefs{
				{Value: &openapi3.Schema{
					Type: typesPtr("object"),
					Properties: map[string]*openapi3.SchemaRef{
						"base": {Value: &openapi3.Schema{Type: typesPtr("string")}},
					},
				}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "All of the following")
		assert.Contains(t, output, "base")
	})

	t.Run("ref cycle detection via composition", func(t *testing.T) {
		var buf bytes.Buffer
		nodeSchema := &openapi3.Schema{}
		selfRef := &openapi3.SchemaRef{Ref: "#/components/schemas/Node"}
		nodeSchema.OneOf = openapi3.SchemaRefs{
			selfRef,
			{Value: &openapi3.Schema{Type: typesPtr("string")}},
		}
		selfRef.Value = nodeSchema

		entryRef := &openapi3.SchemaRef{Ref: "#/components/schemas/Node", Value: nodeSchema}
		spec := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{"Node": {Ref: "#/components/schemas/Node", Value: nodeSchema}},
			},
		}
		resolver := openapi.NewSchemaResolver(spec)
		printSchema(&buf, entryRef, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "see Node above")
	})

	t.Run("direct ref already visited", func(t *testing.T) {
		var buf bytes.Buffer
		thingSchema := &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"id": {Value: &openapi3.Schema{Type: typesPtr("string")}},
			},
		}
		spec := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{"Thing": {Ref: "#/components/schemas/Thing", Value: thingSchema}},
			},
		}
		schemaRef := &openapi3.SchemaRef{Ref: "#/components/schemas/Thing", Value: thingSchema}
		resolver := openapi.NewSchemaResolver(spec)
		visited := map[string]bool{"Thing": true}
		printSchema(&buf, schemaRef, resolver, 2, visited, requestSchemaPrintOpts())
		assert.Contains(t, buf.String(), "see Thing above")
	})

	t.Run("max indent stops recursion", func(t *testing.T) {
		var buf bytes.Buffer
		schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: typesPtr("object"),
			Properties: map[string]*openapi3.SchemaRef{
				"nested": {Value: &openapi3.Schema{
					Type: typesPtr("object"),
					Properties: map[string]*openapi3.SchemaRef{
						"deep": {Value: &openapi3.Schema{Type: typesPtr("string")}},
					},
				}},
			},
		}}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printSchema(&buf, schemaRef, resolver, maxSchemaIndent, make(map[string]bool), requestSchemaPrintOpts())
		output := buf.String()
		assert.Contains(t, output, "nested")
		assert.NotContains(t, output, "deep")
	})
}

// --- printEndpointDetails ----------------------------------------------------

func TestPrintEndpointDetails(t *testing.T) {
	t.Run("full endpoint", func(t *testing.T) {
		var buf bytes.Buffer
		responses := openapi3.NewResponsesWithCapacity(1)
		responses.Set("201", &openapi3.ResponseRef{Value: &openapi3.Response{Description: strPtr("Created")}})
		ep := &openapi.Endpoint{
			Method:      "POST",
			Path:        "/dags",
			OperationID: "create_dag",
			Summary:     "Create a new DAG",
			Description: "Creates a DAG in the system",
			Tags:        []string{"DAGs"},
			Parameters: openapi3.Parameters{
				{Value: &openapi3.Parameter{Name: "dag_id", In: "path", Required: true, Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: typesPtr("string")}}}},
			},
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
								Type: typesPtr("object"),
								Properties: map[string]*openapi3.SchemaRef{
									"name": {Value: &openapi3.Schema{Type: typesPtr("string")}},
								},
							}},
						},
					},
				},
			},
			Responses: responses,
		}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
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
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printEndpointDetails(&buf, ep, resolver)
		assert.Contains(t, buf.String(), "DEPRECATED")
	})

	t.Run("no optional fields", func(t *testing.T) {
		var buf bytes.Buffer
		ep := &openapi.Endpoint{Method: "GET", Path: "/health"}
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
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
		resolver := openapi.NewSchemaResolver(&openapi3.T{})
		printEndpointDetails(&buf, ep, resolver)
		output := buf.String()
		first := bytes.Index([]byte(output), []byte("Same text"))
		second := bytes.Index([]byte(output[first+1:]), []byte("Same text"))
		assert.Equal(t, -1, second, "description identical to summary should not be printed twice")
	})
}

// --- runDescribe -------------------------------------------------------------

func TestRunDescribe(t *testing.T) {
	paths := openapi3.NewPaths()
	paths.Set("/dags", &openapi3.PathItem{
		Get:  &openapi3.Operation{OperationID: "get_dags", Summary: "List DAGs"},
		Post: &openapi3.Operation{OperationID: "create_dag", Summary: "Create DAG"},
	})
	paths.Set("/health", &openapi3.PathItem{
		Get: &openapi3.Operation{OperationID: "health"},
	})
	spec := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Paths:   paths,
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
	spec := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0"},
		Paths:   openapi3.NewPaths(),
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
