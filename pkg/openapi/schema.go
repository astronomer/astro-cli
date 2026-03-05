package openapi

import (
	"strings"
)

// SchemaResolver provides helpers for working with OpenAPI schema references.
type SchemaResolver struct{}

// NewSchemaResolver creates a new schema resolver.
func NewSchemaResolver() *SchemaResolver {
	return &SchemaResolver{}
}

// ResolveSchema extracts the resolved schema and ref name from a SchemaRef.
func (r *SchemaResolver) ResolveSchema(ref *SchemaRef) (resolved *Schema, refName string) {
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
