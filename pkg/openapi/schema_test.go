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
	spec := &OpenAPISpec{Info: Info{Title: "Test"}}
	r := NewSchemaResolver(spec)
	assert.NotNil(t, r)
	assert.Equal(t, spec, r.spec)
}

// --- ResolveSchema -----------------------------------------------------------

func TestResolveSchema_Nil(t *testing.T) {
	r := NewSchemaResolver(&OpenAPISpec{})
	resolved, refName := r.ResolveSchema(nil)
	assert.Nil(t, resolved)
	assert.Empty(t, refName)
}

func TestResolveSchema_NoRef(t *testing.T) {
	r := NewSchemaResolver(&OpenAPISpec{})
	schema := &Schema{Type: "string"}
	resolved, refName := r.ResolveSchema(schema)
	assert.Equal(t, schema, resolved)
	assert.Empty(t, refName)
}

func TestResolveSchema_WithRef_Found(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]Schema{
				"Pet": {Type: "object", Properties: map[string]Schema{
					"name": {Type: "string"},
				}},
			},
		},
	}
	r := NewSchemaResolver(spec)

	schema := &Schema{Ref: "#/components/schemas/Pet"}
	resolved, refName := r.ResolveSchema(schema)
	assert.Equal(t, "Pet", refName)
	assert.Equal(t, "object", resolved.Type)
	assert.Contains(t, resolved.Properties, "name")
}

func TestResolveSchema_WithRef_NotFound(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]Schema{},
		},
	}
	r := NewSchemaResolver(spec)

	schema := &Schema{Ref: "#/components/schemas/Missing"}
	resolved, refName := r.ResolveSchema(schema)
	// Falls through to return original schema when ref not found
	assert.Equal(t, schema, resolved)
	assert.Empty(t, refName)
}

// --- lookupSchema ------------------------------------------------------------

func TestLookupSchema_NilSpec(t *testing.T) {
	r := &SchemaResolver{spec: nil}
	assert.Nil(t, r.lookupSchema("Anything"))
}

func TestLookupSchema_NilComponents(t *testing.T) {
	r := NewSchemaResolver(&OpenAPISpec{})
	assert.Nil(t, r.lookupSchema("Anything"))
}

func TestLookupSchema_NilSchemas(t *testing.T) {
	r := NewSchemaResolver(&OpenAPISpec{Components: &Components{}})
	assert.Nil(t, r.lookupSchema("Anything"))
}

func TestLookupSchema_Found(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]Schema{
				"User": {Type: "object"},
			},
		},
	}
	r := NewSchemaResolver(spec)
	s := r.lookupSchema("User")
	assert.NotNil(t, s)
	assert.Equal(t, "object", s.Type)
}

func TestLookupSchema_NotFound(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]Schema{
				"User": {Type: "object"},
			},
		},
	}
	r := NewSchemaResolver(spec)
	assert.Nil(t, r.lookupSchema("Missing"))
}

// --- GetSchemaByRef ----------------------------------------------------------

func TestGetSchemaByRef(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]Schema{
				"Deployment": {Type: "object"},
			},
		},
	}
	r := NewSchemaResolver(spec)

	t.Run("found", func(t *testing.T) {
		s := r.GetSchemaByRef("#/components/schemas/Deployment")
		assert.NotNil(t, s)
		assert.Equal(t, "object", s.Type)
	})

	t.Run("not found", func(t *testing.T) {
		s := r.GetSchemaByRef("#/components/schemas/Missing")
		assert.Nil(t, s)
	})
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
