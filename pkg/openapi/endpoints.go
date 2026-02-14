package openapi

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const defaultMethodOrder = 99

// ExtractEndpoints extracts all endpoints from an OpenAPI spec.
func ExtractEndpoints(spec *openapi3.T) []Endpoint {
	if spec == nil || spec.Paths == nil {
		return nil
	}

	var endpoints []Endpoint

	for path, pathItem := range spec.Paths.Map() {
		if pathItem.Get != nil {
			endpoints = append(endpoints, newEndpoint("GET", path, pathItem.Get))
		}
		if pathItem.Post != nil {
			endpoints = append(endpoints, newEndpoint("POST", path, pathItem.Post))
		}
		if pathItem.Put != nil {
			endpoints = append(endpoints, newEndpoint("PUT", path, pathItem.Put))
		}
		if pathItem.Patch != nil {
			endpoints = append(endpoints, newEndpoint("PATCH", path, pathItem.Patch))
		}
		if pathItem.Delete != nil {
			endpoints = append(endpoints, newEndpoint("DELETE", path, pathItem.Delete))
		}
		if pathItem.Options != nil {
			endpoints = append(endpoints, newEndpoint("OPTIONS", path, pathItem.Options))
		}
		if pathItem.Head != nil {
			endpoints = append(endpoints, newEndpoint("HEAD", path, pathItem.Head))
		}
	}

	// Sort endpoints by path, then method
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return methodOrder(endpoints[i].Method) < methodOrder(endpoints[j].Method)
	})

	return endpoints
}

// newEndpoint creates an Endpoint from an Operation.
func newEndpoint(method, path string, op *openapi3.Operation) Endpoint {
	return Endpoint{
		Method:      method,
		Path:        path,
		OperationID: op.OperationID,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        op.Tags,
		Parameters:  op.Parameters,
		RequestBody: op.RequestBody,
		Responses:   op.Responses,
		Deprecated:  op.Deprecated,
	}
}

// methodOrder returns an ordering value for HTTP methods.
func methodOrder(method string) int {
	order := map[string]int{
		"GET":     1,
		"POST":    2,
		"PUT":     3,
		"PATCH":   4,
		"DELETE":  5,
		"OPTIONS": 6,
		"HEAD":    7,
	}
	if o, ok := order[method]; ok {
		return o
	}
	return defaultMethodOrder
}

// FilterEndpoints returns endpoints matching the given filter string.
// The filter is matched against the path, method, operation ID, summary, and tags.
func FilterEndpoints(endpoints []Endpoint, filter string) []Endpoint {
	if filter == "" {
		return endpoints
	}

	filter = strings.ToLower(filter)
	var filtered []Endpoint

	for i := range endpoints {
		if matchesFilter(&endpoints[i], filter) {
			filtered = append(filtered, endpoints[i])
		}
	}

	return filtered
}

// matchesFilter checks if an endpoint matches the filter string.
func matchesFilter(ep *Endpoint, filter string) bool {
	// Check path
	if strings.Contains(strings.ToLower(ep.Path), filter) {
		return true
	}

	// Check method
	if strings.Contains(strings.ToLower(ep.Method), filter) {
		return true
	}

	// Check operation ID
	if strings.Contains(strings.ToLower(ep.OperationID), filter) {
		return true
	}

	// Check summary
	if strings.Contains(strings.ToLower(ep.Summary), filter) {
		return true
	}

	// Check tags
	for _, tag := range ep.Tags {
		if strings.Contains(strings.ToLower(tag), filter) {
			return true
		}
	}

	return false
}

// FindEndpoint finds an endpoint by method and path.
func FindEndpoint(endpoints []Endpoint, method, path string) *Endpoint {
	method = strings.ToUpper(method)
	for i := range endpoints {
		if endpoints[i].Method == method && endpoints[i].Path == path {
			return &endpoints[i]
		}
	}
	return nil
}

// FindEndpointByPath finds endpoints matching the given path (any method).
func FindEndpointByPath(endpoints []Endpoint, path string) []Endpoint {
	var matches []Endpoint
	for i := range endpoints {
		if endpoints[i].Path == path {
			matches = append(matches, endpoints[i])
		}
	}
	return matches
}

// FindEndpointByOperationID finds an endpoint by its operation ID (case-insensitive).
func FindEndpointByOperationID(endpoints []Endpoint, operationID string) *Endpoint {
	for i := range endpoints {
		if strings.EqualFold(endpoints[i].OperationID, operationID) {
			return &endpoints[i]
		}
	}
	return nil
}

// GetPathParameters extracts path parameters from an endpoint path.
// For example, "/organizations/{organizationId}/deployments" returns ["organizationId"].
func GetPathParameters(path string) []string {
	var params []string
	start := -1

	for i, c := range path {
		if c == '{' {
			start = i + 1
		} else if c == '}' && start >= 0 {
			params = append(params, path[start:i])
			start = -1
		}
	}

	return params
}
