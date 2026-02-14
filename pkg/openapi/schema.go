package openapi

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// SchemaResolver provides helpers for working with OpenAPI schema references.
// Since kin-openapi resolves $ref during loading, this is primarily used for
// extracting ref names for display purposes.
type SchemaResolver struct {
	spec *openapi3.T
}

// NewSchemaResolver creates a new schema resolver.
func NewSchemaResolver(spec *openapi3.T) *SchemaResolver {
	return &SchemaResolver{spec: spec}
}

// ResolveSchema extracts the resolved schema and ref name from a SchemaRef.
// Since kin-openapi resolves references during loading, Value is already populated.
func (r *SchemaResolver) ResolveSchema(ref *openapi3.SchemaRef) (resolved *openapi3.Schema, refName string) {
	if ref == nil {
		return nil, ""
	}
	if ref.Ref != "" {
		refName = extractRefName(ref.Ref)
	}
	return ref.Value, refName
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
