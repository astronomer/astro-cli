package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- extractRefName ----------------------------------------------------------

func TestExtractRefName(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{
			name:     "standard components/schemas prefix",
			ref:      "#/components/schemas/CreateDeploymentRequest",
			expected: "CreateDeploymentRequest",
		},
		{
			name:     "other ref format",
			ref:      "#/definitions/SomeType",
			expected: "SomeType",
		},
		{
			name:     "bare name",
			ref:      "MySchema",
			expected: "MySchema",
		},
		{
			name:     "deep path",
			ref:      "#/some/deeply/nested/SchemaName",
			expected: "SchemaName",
		},
		{
			name:     "empty string",
			ref:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractRefName(tt.ref))
		})
	}
}

// --- NewSchemaResolver -------------------------------------------------------

func TestNewSchemaResolver(t *testing.T) {
	r := NewSchemaResolver()
	assert.NotNil(t, r)
}

// --- ResolveSchema -----------------------------------------------------------

func TestResolveSchema_Nil(t *testing.T) {
	r := NewSchemaResolver()
	resolved, refName := r.ResolveSchema(nil)
	assert.Nil(t, resolved)
	assert.Empty(t, refName)
}

func TestResolveSchema_NoRef(t *testing.T) {
	r := NewSchemaResolver()
	schema := &Schema{Type: "string"}
	ref := &SchemaRef{Value: schema}
	resolved, refName := r.ResolveSchema(ref)
	assert.Equal(t, schema, resolved)
	assert.Empty(t, refName)
}

func TestResolveSchema_WithRef(t *testing.T) {
	r := NewSchemaResolver()
	resolvedSchema := &Schema{
		Type: "object",
		Properties: []SchemaProperty{
			{Name: "name", Schema: &SchemaRef{Value: &Schema{Type: "string"}}},
		},
	}
	ref := &SchemaRef{
		Ref:   "#/components/schemas/Pet",
		Value: resolvedSchema,
	}
	resolved, refName := r.ResolveSchema(ref)
	assert.Equal(t, "Pet", refName)
	assert.Equal(t, resolvedSchema, resolved)
	assert.Equal(t, "object", resolved.Type)
	assert.Len(t, resolved.Properties, 1)
	assert.Equal(t, "name", resolved.Properties[0].Name)
}

// --- IsRequired --------------------------------------------------------------

func TestIsRequired(t *testing.T) {
	required := []string{"name", "email"}

	assert.True(t, IsRequired("name", required))
	assert.True(t, IsRequired("email", required))
	assert.False(t, IsRequired("age", required))
	assert.False(t, IsRequired("name", nil))
	assert.False(t, IsRequired("name", []string{}))
}
