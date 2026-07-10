package api

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/pkg/openapi"
)

// The types below are the machine-readable form of `describe`, emitted with the
// --json flag. Schemas are resolved down the $ref stack so an agent gets the
// actual fields without needing the spec; genuine cycles are cut and marked
// with "circular": true rather than recursing forever.
//
// Matched endpoints are always emitted as an array so callers can rely on a
// stable top-level shape.

// maxJSONSchemaDepth bounds resolveSchemaJSON's recursion. Cycles are already
// cut via the ancestry set; this is a backstop against pathologically deep or
// wide (diamond) acyclic schemas so output stays bounded. Real specs nest far
// shallower than this.
const maxJSONSchemaDepth = 40

type endpointJSON struct {
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	OperationID string           `json:"operationId,omitempty"`
	Summary     string           `json:"summary,omitempty"`
	Description string           `json:"description,omitempty"`
	Deprecated  bool             `json:"deprecated,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Parameters  []parameterJSON  `json:"parameters,omitempty"`
	RequestBody *requestBodyJSON `json:"requestBody,omitempty"`
	Responses   []responseJSON   `json:"responses,omitempty"`
}

type parameterJSON struct {
	Name        string      `json:"name"`
	In          string      `json:"in,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Schema      *schemaJSON `json:"schema,omitempty"`
}

type requestBodyJSON struct {
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Schema      *schemaJSON `json:"schema,omitempty"`
}

type responseJSON struct {
	Code        string      `json:"code"`
	Description string      `json:"description,omitempty"`
	Schema      *schemaJSON `json:"schema,omitempty"`
}

type schemaJSON struct {
	Ref         string         `json:"ref,omitempty"`
	Circular    bool           `json:"circular,omitempty"`
	Truncated   bool           `json:"truncated,omitempty"`
	Type        string         `json:"type,omitempty"`
	Format      string         `json:"format,omitempty"`
	Description string         `json:"description,omitempty"`
	Required    []string       `json:"required,omitempty"`
	ReadOnly    bool           `json:"readOnly,omitempty"`
	Deprecated  bool           `json:"deprecated,omitempty"`
	Properties  []propertyJSON `json:"properties,omitempty"`
	Items       *schemaJSON    `json:"items,omitempty"`
	OneOf       []*schemaJSON  `json:"oneOf,omitempty"`
	AnyOf       []*schemaJSON  `json:"anyOf,omitempty"`
	AllOf       []*schemaJSON  `json:"allOf,omitempty"`
	Enum        []any          `json:"enum,omitempty"`
	Default     any            `json:"default,omitempty"`
	Example     any            `json:"example,omitempty"`
}

type propertyJSON struct {
	Name   string      `json:"name"`
	Schema *schemaJSON `json:"schema"`
}

// writeEndpointsJSON marshals the matched endpoints as a JSON array.
func writeEndpointsJSON(out io.Writer, matches []openapi.Endpoint, resolver *openapi.SchemaResolver) error {
	eps := make([]endpointJSON, 0, len(matches))
	for i := range matches {
		eps = append(eps, buildEndpointJSON(&matches[i], resolver))
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(eps); err != nil {
		return fmt.Errorf("encoding describe output as JSON: %w", err)
	}
	return nil
}

// listEndpointJSON is the machine-readable form of a single `ls` row. It is a
// lean summary — use `describe --json` for the full resolved schema.
type listEndpointJSON struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	OperationID    string   `json:"operationId,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Deprecated     bool     `json:"deprecated,omitempty"`
	PathParameters []string `json:"pathParameters,omitempty"`
}

// writeEndpointListJSON marshals the endpoint list as a JSON array (always an
// array, even for zero or one match, so consumers can rely on the shape).
func writeEndpointListJSON(out io.Writer, endpoints []openapi.Endpoint) error {
	items := make([]listEndpointJSON, 0, len(endpoints))
	for i := range endpoints {
		ep := &endpoints[i]
		items = append(items, listEndpointJSON{
			Method:         ep.Method,
			Path:           ep.Path,
			OperationID:    ep.OperationID,
			Summary:        ep.Summary,
			Tags:           ep.Tags,
			Deprecated:     ep.Deprecated,
			PathParameters: openapi.GetPathParameters(ep.Path),
		})
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(items); err != nil {
		return fmt.Errorf("encoding list output as JSON: %w", err)
	}
	return nil
}

func buildEndpointJSON(ep *openapi.Endpoint, resolver *openapi.SchemaResolver) endpointJSON {
	e := endpointJSON{
		Method:      ep.Method,
		Path:        ep.Path,
		OperationID: ep.OperationID,
		Summary:     ep.Summary,
		Description: ep.Description,
		Deprecated:  ep.Deprecated,
		Tags:        ep.Tags,
	}

	for _, p := range ep.Parameters {
		if p == nil {
			continue
		}
		e.Parameters = append(e.Parameters, parameterJSON{
			Name:        p.Name,
			In:          p.In,
			Description: p.Description,
			Required:    p.Required,
			Schema:      resolveSchemaJSON(p.Schema, resolver, map[string]bool{}, 0),
		})
	}

	if ep.RequestBody != nil {
		rb := &requestBodyJSON{
			Description: ep.RequestBody.Description,
			Required:    ep.RequestBody.Required,
		}
		if mt, ok := ep.RequestBody.Content["application/json"]; ok && mt.Schema != nil {
			rb.Schema = resolveSchemaJSON(mt.Schema, resolver, map[string]bool{}, 0)
		}
		e.RequestBody = rb
	}

	if ep.Responses != nil {
		for i := range ep.Responses.Codes {
			entry := &ep.Responses.Codes[i]
			r := responseJSON{Code: entry.Code, Description: entry.Description}
			if mt, ok := entry.Content["application/json"]; ok && mt.Schema != nil {
				r.Schema = resolveSchemaJSON(mt.Schema, resolver, map[string]bool{}, 0)
			}
			e.Responses = append(e.Responses, r)
		}
	}

	return e
}

// resolveSchemaJSON converts a SchemaRef into its resolved JSON form, following
// $ref references via the resolver. visited tracks the current ancestry so real
// cycles are cut (marked circular) while a schema reused on sibling branches is
// still expanded in full. depth bounds recursion as a backstop for deep/wide
// acyclic schemas (marked truncated).
func resolveSchemaJSON(ref *openapi.SchemaRef, resolver *openapi.SchemaResolver, visited map[string]bool, depth int) *schemaJSON {
	if ref == nil {
		return nil
	}

	schema, refName := resolver.ResolveSchema(ref)
	node := &schemaJSON{Ref: refName}

	if refName != "" && visited[refName] {
		node.Circular = true
		return node
	}
	if schema == nil {
		// Unresolved ref (unknown schema or no registry): report the name only.
		return node
	}

	if depth >= maxJSONSchemaDepth {
		node.Type = schema.Type
		node.Truncated = true
		return node
	}

	if refName != "" {
		visited[refName] = true
		defer delete(visited, refName)
	}

	node.Type = schema.Type
	node.Format = schema.Format
	node.Description = schema.Description
	node.Required = schema.Required
	node.ReadOnly = schema.ReadOnly
	node.Deprecated = schema.Deprecated
	node.Enum = schema.Enum
	node.Default = schema.Default
	node.Example = schema.Example

	for _, p := range schema.Properties {
		node.Properties = append(node.Properties, propertyJSON{
			Name:   p.Name,
			Schema: resolveSchemaJSON(p.Schema, resolver, visited, depth+1),
		})
	}
	if schema.Items != nil {
		node.Items = resolveSchemaJSON(schema.Items, resolver, visited, depth+1)
	}
	for _, c := range schema.OneOf {
		node.OneOf = append(node.OneOf, resolveSchemaJSON(c, resolver, visited, depth+1))
	}
	for _, c := range schema.AnyOf {
		node.AnyOf = append(node.AnyOf, resolveSchemaJSON(c, resolver, visited, depth+1))
	}
	for _, c := range schema.AllOf {
		node.AllOf = append(node.AllOf, resolveSchemaJSON(c, resolver, visited, depth+1))
	}

	return node
}
