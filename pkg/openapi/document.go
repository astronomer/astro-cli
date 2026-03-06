package openapi

import "github.com/pb33f/libopenapi"

// newDocument wraps libopenapi.NewDocument. It exists as a package-level var
// so tests can override it when needed.
var newDocument = libopenapi.NewDocument
