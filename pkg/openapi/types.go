// Package openapi provides utilities for working with OpenAPI specifications.
package openapi

// Endpoint represents a single API endpoint for display and selection.
type Endpoint struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string
	Tags        []string
	Parameters  []*Parameter
	RequestBody *RequestBody
	Responses   *Responses
	Deprecated  bool
}

// Parameter represents an API parameter.
type Parameter struct {
	Name        string
	In          string
	Description string
	Required    bool
	Schema      *SchemaRef
}

// RequestBody represents an API request body.
type RequestBody struct {
	Description string
	Required    bool
	Content     map[string]*MediaType
}

// MediaType represents a media type with its schema.
type MediaType struct {
	Schema *SchemaRef
}

// Responses represents a set of API responses keyed by status code.
type Responses struct {
	Codes []ResponseEntry
}

// ResponseEntry represents a single response code and its details.
type ResponseEntry struct {
	Code        string
	Description string
	Content     map[string]*MediaType
}

// Len returns the number of response entries.
func (r *Responses) Len() int {
	if r == nil {
		return 0
	}
	return len(r.Codes)
}

// SchemaRef wraps a schema with an optional $ref name.
type SchemaRef struct {
	Ref   string
	Value *Schema
}

// Schema represents an OpenAPI schema.
type Schema struct {
	Type        string
	Format      string
	Description string
	Required    []string
	Properties  []SchemaProperty
	Items       *SchemaRef
	OneOf       []*SchemaRef
	AnyOf       []*SchemaRef
	AllOf       []*SchemaRef
	Enum        []any
	Default     any
	Example     any
	ReadOnly    bool
	Deprecated  bool
}

// SchemaProperty represents a named schema property in order.
type SchemaProperty struct {
	Name   string
	Schema *SchemaRef
}
