package openapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalSwagger20JSON returns a minimal valid Swagger 2.0 spec as JSON bytes.
func minimalSwagger20JSON(basePath string, paths map[string]any) []byte {
	spec := map[string]any{
		"swagger":  "2.0",
		"info":     map[string]any{"title": "Test", "version": "1.0"},
		"basePath": basePath,
		"paths":    paths,
	}
	data, _ := json.Marshal(spec)
	return data
}

func TestExtractEndpointsV2(t *testing.T) {
	specJSON := minimalSwagger20JSON("/api/v1", map[string]any{
		"/organizations": map[string]any{
			"get": map[string]any{
				"operationId": "ListOrganizations",
				"summary":     "List all organizations",
				"tags":        []string{"Organization"},
				"responses": map[string]any{
					"200": map[string]any{"description": "OK"},
				},
			},
			"post": map[string]any{
				"operationId": "CreateOrganization",
				"summary":     "Create an organization",
				"tags":        []string{"Organization"},
				"responses": map[string]any{
					"200": map[string]any{"description": "Created"},
				},
			},
		},
		"/organizations/{organizationId}": map[string]any{
			"get": map[string]any{
				"operationId": "GetOrganization",
				"summary":     "Get organization",
				"parameters": []any{
					map[string]any{
						"name":     "organizationId",
						"in":       "path",
						"type":     "string",
						"required": true,
					},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "OK"},
				},
			},
		},
	})

	_, v2doc, err := parseSpec(specJSON)
	require.NoError(t, err)
	require.NotNil(t, v2doc)

	endpoints := ExtractEndpointsV2(v2doc)
	require.Len(t, endpoints, 3)

	// Sorted by path then method
	assert.Equal(t, "/organizations", endpoints[0].Path)
	assert.Equal(t, "GET", endpoints[0].Method)
	assert.Equal(t, "ListOrganizations", endpoints[0].OperationID)
	assert.Equal(t, []string{"Organization"}, endpoints[0].Tags)

	assert.Equal(t, "/organizations", endpoints[1].Path)
	assert.Equal(t, "POST", endpoints[1].Method)

	assert.Equal(t, "/organizations/{organizationId}", endpoints[2].Path)
	assert.Equal(t, "GET", endpoints[2].Method)
	require.Len(t, endpoints[2].Parameters, 1)
	assert.Equal(t, "organizationId", endpoints[2].Parameters[0].Name)
	assert.Equal(t, "path", endpoints[2].Parameters[0].In)
	assert.True(t, endpoints[2].Parameters[0].Required)
}

func TestExtractEndpointsV2_BodyParam(t *testing.T) {
	specJSON := minimalSwagger20JSON("/api/v1", map[string]any{
		"/resources": map[string]any{
			"post": map[string]any{
				"operationId": "CreateResource",
				"parameters": []any{
					map[string]any{
						"name":     "body",
						"in":       "body",
						"required": true,
						"schema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{"type": "string"},
							},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Created"},
				},
			},
		},
	})

	_, v2doc, err := parseSpec(specJSON)
	require.NoError(t, err)
	require.NotNil(t, v2doc)

	endpoints := ExtractEndpointsV2(v2doc)
	require.Len(t, endpoints, 1)

	ep := endpoints[0]
	assert.Equal(t, "CreateResource", ep.OperationID)
	// Body param should be converted to RequestBody, not added to Parameters
	assert.Empty(t, ep.Parameters)
	require.NotNil(t, ep.RequestBody)
	assert.True(t, ep.RequestBody.Required)
	require.Contains(t, ep.RequestBody.Content, "application/json")
	require.NotNil(t, ep.RequestBody.Content["application/json"].Schema)
}

func TestExtractEndpointsV2_Responses(t *testing.T) {
	specJSON := minimalSwagger20JSON("/api/v1", map[string]any{
		"/things": map[string]any{
			"get": map[string]any{
				"operationId": "ListThings",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Success",
						"schema": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"id": map[string]any{"type": "string"},
								},
							},
						},
					},
					"default": map[string]any{
						"description": "Error",
						"schema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	})

	_, v2doc, err := parseSpec(specJSON)
	require.NoError(t, err)
	require.NotNil(t, v2doc)

	endpoints := ExtractEndpointsV2(v2doc)
	require.Len(t, endpoints, 1)

	ep := endpoints[0]
	require.NotNil(t, ep.Responses)
	require.Len(t, ep.Responses.Codes, 2)

	// Check 200 response
	assert.Equal(t, "200", ep.Responses.Codes[0].Code)
	assert.Equal(t, "Success", ep.Responses.Codes[0].Description)
	require.Contains(t, ep.Responses.Codes[0].Content, "application/json")
	schema := ep.Responses.Codes[0].Content["application/json"].Schema.Value
	assert.Equal(t, "array", schema.Type)

	// Check default response
	assert.Equal(t, "default", ep.Responses.Codes[1].Code)
	assert.Equal(t, "Error", ep.Responses.Codes[1].Description)
	require.Contains(t, ep.Responses.Codes[1].Content, "application/json")
}

func TestExtractEndpointsV2_NilDoc(t *testing.T) {
	endpoints := ExtractEndpointsV2(nil)
	assert.Nil(t, endpoints)
}

func TestExtractEndpointsV2_InlineParamType(t *testing.T) {
	specJSON := minimalSwagger20JSON("/v1", map[string]any{
		"/search": map[string]any{
			"get": map[string]any{
				"operationId": "Search",
				"parameters": []any{
					map[string]any{
						"name": "q",
						"in":   "query",
						"type": "string",
					},
					map[string]any{
						"name":   "limit",
						"in":     "query",
						"type":   "integer",
						"format": "int32",
					},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "OK"},
				},
			},
		},
	})

	_, v2doc, err := parseSpec(specJSON)
	require.NoError(t, err)

	endpoints := ExtractEndpointsV2(v2doc)
	require.Len(t, endpoints, 1)
	require.Len(t, endpoints[0].Parameters, 2)

	// Query params with inline type/format should have Schema set
	q := endpoints[0].Parameters[0]
	assert.Equal(t, "q", q.Name)
	require.NotNil(t, q.Schema)
	assert.Equal(t, "string", q.Schema.Value.Type)

	limit := endpoints[0].Parameters[1]
	assert.Equal(t, "limit", limit.Name)
	require.NotNil(t, limit.Schema)
	assert.Equal(t, "integer", limit.Schema.Value.Type)
	assert.Equal(t, "int32", limit.Schema.Value.Format)
}
