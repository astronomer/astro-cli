package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenAPI31_NumericExclusiveMinimum verifies that libopenapi correctly
// parses OpenAPI 3.1 specs that use numeric exclusiveMinimum (the value that
// caused silent failures with kin-openapi v0.131.0).
func TestOpenAPI31_NumericExclusiveMinimum(t *testing.T) {
	spec := []byte(`openapi: "3.1.0"
info:
  title: "Airflow 3.1 Test"
  version: "3.1.7"
paths:
  /dags:
    get:
      operationId: get_dags
      summary: List DAGs
      parameters:
        - name: limit
          in: query
          schema:
            type: integer
            exclusiveMinimum: 0
      responses:
        "200":
          description: Success
`)

	doc, _, err := parseSpec(spec)
	require.NoError(t, err)
	require.NotNil(t, doc)

	endpoints := ExtractEndpoints(doc)
	require.Len(t, endpoints, 1)
	assert.Equal(t, "get_dags", endpoints[0].OperationID)
	assert.Equal(t, "/dags", endpoints[0].Path)
	require.Len(t, endpoints[0].Parameters, 1)
	assert.Equal(t, "limit", endpoints[0].Parameters[0].Name)
	assert.Equal(t, "integer", endpoints[0].Parameters[0].Schema.Value.Type)
}

// TestOpenAPI31_TypeArray verifies that OpenAPI 3.1 multi-type arrays (e.g. type: [string, null])
// are handled correctly.
func TestOpenAPI31_TypeArray(t *testing.T) {
	spec := []byte(`openapi: "3.1.0"
info:
  title: "Type Array Test"
  version: "1.0"
paths:
  /test:
    get:
      operationId: get_test
      responses:
        "200":
          description: Success
          content:
            application/json:
              schema:
                type: object
                properties:
                  name:
                    type:
                      - string
                      - "null"
`)

	doc, _, err := parseSpec(spec)
	require.NoError(t, err)
	require.NotNil(t, doc)

	endpoints := ExtractEndpoints(doc)
	require.Len(t, endpoints, 1)
	require.NotNil(t, endpoints[0].Responses)
	require.Len(t, endpoints[0].Responses.Codes, 1)

	content := endpoints[0].Responses.Codes[0].Content
	require.Contains(t, content, "application/json")
	schema := content["application/json"].Schema.Value
	require.Len(t, schema.Properties, 1)
	// First type in the array should be extracted
	assert.Equal(t, "string", schema.Properties[0].Schema.Value.Type)
}
