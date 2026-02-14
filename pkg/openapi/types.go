// Package openapi provides utilities for working with OpenAPI specifications.
package openapi

import (
	"github.com/getkin/kin-openapi/openapi3"
)

// Endpoint represents a single API endpoint for display and selection.
type Endpoint struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string
	Tags        []string
	Parameters  openapi3.Parameters
	RequestBody *openapi3.RequestBodyRef
	Responses   *openapi3.Responses
	Deprecated  bool
}
