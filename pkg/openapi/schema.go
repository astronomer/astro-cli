package openapi

import (
	"strings"
)

// SchemaResolver provides helpers for working with OpenAPI schema references.
// When constructed with a registry of named component schemas, it can resolve
// $ref references lazily — returning the referenced schema's value even though
// the SchemaRef itself only carries the ref name.
type SchemaResolver struct {
	// registry maps component schema names (e.g. "CreateDeploymentRequest") to
	// their resolved schema. Nested $refs within these schemas are left as ref
	// names, so callers must guard against cycles when walking recursively.
	registry map[string]*Schema
}

// NewSchemaResolver creates a new schema resolver without a registry. Refs are
// reported by name but not resolved to their value.
func NewSchemaResolver() *SchemaResolver {
	return &SchemaResolver{}
}

// NewSchemaResolverWithSchemas creates a resolver backed by a registry of named
// component schemas, enabling lazy resolution of $ref references.
func NewSchemaResolverWithSchemas(registry map[string]*Schema) *SchemaResolver {
	return &SchemaResolver{registry: registry}
}

// ResolveSchema extracts the resolved schema and ref name from a SchemaRef.
// If the ref has no inline value but names a schema present in the registry,
// the registered schema is returned so callers can walk down the ref stack.
func (r *SchemaResolver) ResolveSchema(ref *SchemaRef) (resolved *Schema, refName string) {
	if ref == nil {
		return nil, ""
	}
	if ref.Ref != "" {
		refName = extractRefName(ref.Ref)
	}
	if ref.Value != nil {
		return ref.Value, refName
	}
	if refName != "" && r.registry != nil {
		return r.registry[refName], refName
	}
	return nil, refName
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
