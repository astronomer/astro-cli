// Package openapi provides utilities for working with OpenAPI specifications.
package openapi

// OpenAPISpec represents a minimal OpenAPI 3.0 specification structure
// containing only the fields needed for endpoint discovery.
type OpenAPISpec struct {
	OpenAPI    string              `json:"openapi" yaml:"openapi"`
	Info       Info                `json:"info" yaml:"info"`
	Servers    []Server            `json:"servers,omitempty" yaml:"servers,omitempty"`
	Paths      map[string]PathItem `json:"paths" yaml:"paths"`
	Components *Components         `json:"components,omitempty" yaml:"components,omitempty"`
}

// Components holds reusable schema definitions.
type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty" yaml:"schemas,omitempty"`
}

// Info contains API metadata.
type Info struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Version     string `json:"version" yaml:"version"`
}

// Server represents an API server.
type Server struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// PathItem represents the operations available on a single path.
type PathItem struct {
	Get     *Operation `json:"get,omitempty" yaml:"get,omitempty"`
	Post    *Operation `json:"post,omitempty" yaml:"post,omitempty"`
	Put     *Operation `json:"put,omitempty" yaml:"put,omitempty"`
	Patch   *Operation `json:"patch,omitempty" yaml:"patch,omitempty"`
	Delete  *Operation `json:"delete,omitempty" yaml:"delete,omitempty"`
	Options *Operation `json:"options,omitempty" yaml:"options,omitempty"`
	Head    *Operation `json:"head,omitempty" yaml:"head,omitempty"`
}

// Operation represents a single API operation on a path.
type Operation struct {
	OperationID string              `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Summary     string              `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string              `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty" yaml:"tags,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses,omitempty" yaml:"responses,omitempty"`
	Deprecated  bool                `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// Parameter represents an operation parameter.
type Parameter struct {
	Name        string  `json:"name" yaml:"name"`
	In          string  `json:"in" yaml:"in"` // query, header, path, cookie
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool    `json:"required,omitempty" yaml:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// RequestBody represents a request body.
type RequestBody struct {
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool                 `json:"required,omitempty" yaml:"required,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

// Response represents an API response.
type Response struct {
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

// MediaType represents a media type object.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// Schema represents a JSON Schema object.
type Schema struct {
	Type        string            `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string            `json:"format,omitempty" yaml:"format,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema           `json:"items,omitempty" yaml:"items,omitempty"`
	Ref         string            `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	Required    []string          `json:"required,omitempty" yaml:"required,omitempty"`
	Enum        []any             `json:"enum,omitempty" yaml:"enum,omitempty"`
	Example     any               `json:"example,omitempty" yaml:"example,omitempty"`
	Default     any               `json:"default,omitempty" yaml:"default,omitempty"`
	Minimum     *float64          `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	MinLength   *int              `json:"minLength,omitempty" yaml:"minLength,omitempty"`
	MaxLength   *int              `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	MinItems    *int              `json:"minItems,omitempty" yaml:"minItems,omitempty"`
	MaxItems    *int              `json:"maxItems,omitempty" yaml:"maxItems,omitempty"`
	Pattern     string            `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	ReadOnly    bool              `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	WriteOnly   bool              `json:"writeOnly,omitempty" yaml:"writeOnly,omitempty"`
	OneOf       []Schema          `json:"oneOf,omitempty" yaml:"oneOf,omitempty"`
	AnyOf       []Schema          `json:"anyOf,omitempty" yaml:"anyOf,omitempty"`
	AllOf       []Schema          `json:"allOf,omitempty" yaml:"allOf,omitempty"`
}

// Endpoint represents a single API endpoint for display and selection.
type Endpoint struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string
	Tags        []string
	Parameters  []Parameter
	RequestBody *RequestBody
	Responses   map[string]Response
	Deprecated  bool
}
