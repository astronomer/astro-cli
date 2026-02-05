package openapi

import (
	"strings"
)

// SchemaResolver resolves $ref references in an OpenAPI spec.
type SchemaResolver struct {
	spec *OpenAPISpec
}

// NewSchemaResolver creates a new schema resolver.
func NewSchemaResolver(spec *OpenAPISpec) *SchemaResolver {
	return &SchemaResolver{spec: spec}
}

// ResolveSchema resolves a schema, following $ref if present.
// Returns the resolved schema and the reference name (if it was a $ref).
func (r *SchemaResolver) ResolveSchema(schema *Schema) (resolved *Schema, refName string) {
	if schema == nil {
		return nil, ""
	}

	if schema.Ref != "" {
		refName := extractRefName(schema.Ref)
		resolved := r.lookupSchema(refName)
		if resolved != nil {
			return resolved, refName
		}
	}

	return schema, ""
}

// lookupSchema looks up a schema by name in components/schemas.
func (r *SchemaResolver) lookupSchema(name string) *Schema {
	if r.spec == nil || r.spec.Components == nil || r.spec.Components.Schemas == nil {
		return nil
	}

	if schema, ok := r.spec.Components.Schemas[name]; ok {
		return &schema
	}
	return nil
}

// GetSchemaByRef looks up a schema by its full $ref path.
func (r *SchemaResolver) GetSchemaByRef(ref string) *Schema {
	return r.lookupSchema(extractRefName(ref))
}

// extractRefName extracts the schema name from a $ref string.
// e.g., "#/components/schemas/CreateDeploymentRequest" -> "CreateDeploymentRequest"
func extractRefName(ref string) string {
	const prefix = "#/components/schemas/"
	if strings.HasPrefix(ref, prefix) {
		return ref[len(prefix):]
	}
	// Handle other ref formats if needed
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ref
}

// IsRequired checks if a property name is in the required list.
func IsRequired(name string, required []string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}
