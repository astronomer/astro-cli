package openapi

import (
	"sort"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"
)

const defaultMethodOrder = 99

// sortEndpoints sorts endpoints by path, then by method order.
func sortEndpoints(endpoints []Endpoint) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return methodOrder(endpoints[i].Method) < methodOrder(endpoints[j].Method)
	})
}

// ExtractEndpoints extracts all endpoints from an OpenAPI v3 document.
func ExtractEndpoints(doc *v3high.Document) []Endpoint {
	if doc == nil || doc.Paths == nil || doc.Paths.PathItems == nil {
		return nil
	}

	var endpoints []Endpoint

	for path, pathItem := range doc.Paths.PathItems.FromOldest() {
		for _, mo := range v3MethodOps(pathItem) {
			if mo.op != nil {
				endpoints = append(endpoints, newEndpoint(mo.method, path, mo.op))
			}
		}
	}

	sortEndpoints(endpoints)
	return endpoints
}

type v3MethodOp struct {
	method string
	op     *v3high.Operation
}

func v3MethodOps(pi *v3high.PathItem) []v3MethodOp {
	return []v3MethodOp{
		{"GET", pi.Get},
		{"POST", pi.Post},
		{"PUT", pi.Put},
		{"PATCH", pi.Patch},
		{"DELETE", pi.Delete},
		{"OPTIONS", pi.Options},
		{"HEAD", pi.Head},
	}
}

// newEndpoint creates an Endpoint from a libopenapi Operation.
func newEndpoint(method, path string, op *v3high.Operation) Endpoint {
	ep := Endpoint{
		Method:      method,
		Path:        path,
		OperationID: op.OperationId,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        op.Tags,
		Deprecated:  op.Deprecated != nil && *op.Deprecated,
	}

	// Convert parameters
	for _, p := range op.Parameters {
		ep.Parameters = append(ep.Parameters, convertParameter(p))
	}

	// Convert request body
	if op.RequestBody != nil {
		ep.RequestBody = convertRequestBody(op.RequestBody)
	}

	// Convert responses
	if op.Responses != nil {
		ep.Responses = convertResponses(op.Responses)
	}

	return ep
}

// convertParameter converts a libopenapi Parameter to our Parameter type.
func convertParameter(p *v3high.Parameter) *Parameter {
	param := &Parameter{
		Name:        p.Name,
		In:          p.In,
		Description: p.Description,
		Required:    p.Required != nil && *p.Required,
	}
	if p.Schema != nil {
		param.Schema = convertSchemaProxy(p.Schema)
	}
	return param
}

// convertRequestBody converts a libopenapi RequestBody to our RequestBody type.
func convertRequestBody(rb *v3high.RequestBody) *RequestBody {
	body := &RequestBody{
		Description: rb.Description,
		Required:    rb.Required != nil && *rb.Required,
	}
	if rb.Content != nil {
		body.Content = convertContentMap(rb.Content)
	}
	return body
}

// convertResponses converts libopenapi Responses to our Responses type.
func convertResponses(r *v3high.Responses) *Responses {
	resp := &Responses{}

	if r.Codes != nil {
		for code, response := range r.Codes.FromOldest() {
			entry := ResponseEntry{
				Code:        code,
				Description: response.Description,
			}
			if response.Content != nil {
				entry.Content = convertContentMap(response.Content)
			}
			resp.Codes = append(resp.Codes, entry)
		}
	}

	// Also include the default response if present
	if r.Default != nil {
		entry := ResponseEntry{
			Code:        "default",
			Description: r.Default.Description,
		}
		if r.Default.Content != nil {
			entry.Content = convertContentMap(r.Default.Content)
		}
		resp.Codes = append(resp.Codes, entry)
	}

	return resp
}

// convertContentMap converts a libopenapi content ordered map to our map type.
func convertContentMap(content *orderedmap.Map[string, *v3high.MediaType]) map[string]*MediaType {
	result := make(map[string]*MediaType)
	for key, val := range content.FromOldest() {
		mt := &MediaType{}
		if val.Schema != nil {
			mt.Schema = convertSchemaProxy(val.Schema)
		}
		result[key] = mt
	}
	return result
}

// convertSchemaProxy converts a libopenapi SchemaProxy to our SchemaRef type.
// When the proxy is a $ref, we return just the ref name without resolving it
// to avoid infinite recursion on circular references.
func convertSchemaProxy(sp *base.SchemaProxy) *SchemaRef {
	if sp == nil {
		return nil
	}
	ref := &SchemaRef{
		Ref: sp.GetReference(),
	}
	// If this is a $ref, don't resolve it — just return the ref name.
	// Resolving would cause infinite recursion on circular schemas.
	if ref.Ref != "" {
		return ref
	}
	schema, err := sp.BuildSchema()
	if err != nil || schema == nil {
		return ref
	}
	ref.Value = convertSchema(schema)
	return ref
}

// convertSchema converts a libopenapi Schema to our Schema type.
func convertSchema(s *base.Schema) *Schema {
	if s == nil {
		return nil
	}
	schema := &Schema{
		Format:      s.Format,
		Description: s.Description,
		Required:    s.Required,
		ReadOnly:    s.ReadOnly != nil && *s.ReadOnly,
		Deprecated:  s.Deprecated != nil && *s.Deprecated,
	}

	// Type
	if len(s.Type) > 0 {
		schema.Type = s.Type[0]
	}

	// Properties (ordered)
	if s.Properties != nil {
		for name, proxy := range s.Properties.FromOldest() {
			schema.Properties = append(schema.Properties, SchemaProperty{
				Name:   name,
				Schema: convertSchemaProxy(proxy),
			})
		}
	}

	// Items
	if s.Items != nil && s.Items.A != nil {
		schema.Items = convertSchemaProxy(s.Items.A)
	}

	// Composition
	for _, sp := range s.OneOf {
		schema.OneOf = append(schema.OneOf, convertSchemaProxy(sp))
	}
	for _, sp := range s.AnyOf {
		schema.AnyOf = append(schema.AnyOf, convertSchemaProxy(sp))
	}
	for _, sp := range s.AllOf {
		schema.AllOf = append(schema.AllOf, convertSchemaProxy(sp))
	}

	// Enum
	for _, node := range s.Enum {
		schema.Enum = append(schema.Enum, yamlNodeToValue(node))
	}

	// Default
	if s.Default != nil {
		schema.Default = yamlNodeToValue(s.Default)
	}

	// Example
	if s.Example != nil {
		schema.Example = yamlNodeToValue(s.Example)
	}

	return schema
}

// yamlNodeToValue converts a *yaml.Node to a Go value.
func yamlNodeToValue(node *yaml.Node) any {
	if node == nil {
		return nil
	}
	var val any
	if err := node.Decode(&val); err != nil {
		return node.Value
	}
	return val
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
	if strings.Contains(strings.ToLower(ep.Path), filter) {
		return true
	}
	if strings.Contains(strings.ToLower(ep.Method), filter) {
		return true
	}
	if strings.Contains(strings.ToLower(ep.OperationID), filter) {
		return true
	}
	if strings.Contains(strings.ToLower(ep.Summary), filter) {
		return true
	}
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
