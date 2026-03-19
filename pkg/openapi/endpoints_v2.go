package openapi

import (
	"strings"

	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
)

// ExtractEndpointsV2 extracts all endpoints from a Swagger 2.0 document.
func ExtractEndpointsV2(doc *v2high.Swagger) []Endpoint {
	if doc == nil || doc.Paths == nil || doc.Paths.PathItems == nil {
		return nil
	}

	var endpoints []Endpoint

	for path, pathItem := range doc.Paths.PathItems.FromOldest() {
		for _, mo := range v2MethodOps(pathItem) {
			if mo.op != nil {
				endpoints = append(endpoints, newEndpointV2(mo.method, path, mo.op))
			}
		}
	}

	sortEndpoints(endpoints)
	return endpoints
}

type v2MethodOp struct {
	method string
	op     *v2high.Operation
}

func v2MethodOps(pi *v2high.PathItem) []v2MethodOp {
	return []v2MethodOp{
		{"GET", pi.Get},
		{"POST", pi.Post},
		{"PUT", pi.Put},
		{"PATCH", pi.Patch},
		{"DELETE", pi.Delete},
		{"OPTIONS", pi.Options},
		{"HEAD", pi.Head},
	}
}

// newEndpointV2 creates an Endpoint from a Swagger 2.0 Operation.
func newEndpointV2(method, path string, op *v2high.Operation) Endpoint {
	ep := Endpoint{
		Method:      method,
		Path:        path,
		OperationID: op.OperationId,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        op.Tags,
		Deprecated:  op.Deprecated,
	}

	// Convert parameters — separate body params from regular params.
	for _, p := range op.Parameters {
		if strings.EqualFold(p.In, "body") {
			ep.RequestBody = convertV2BodyParam(p)
		} else {
			ep.Parameters = append(ep.Parameters, convertV2Parameter(p))
		}
	}

	// Convert responses
	if op.Responses != nil {
		ep.Responses = convertV2Responses(op.Responses)
	}

	return ep
}

// convertV2Parameter converts a Swagger 2.0 Parameter (non-body) to our Parameter type.
func convertV2Parameter(p *v2high.Parameter) *Parameter {
	param := &Parameter{
		Name:        p.Name,
		In:          p.In,
		Description: p.Description,
		Required:    p.Required != nil && *p.Required,
	}
	if p.Schema != nil {
		param.Schema = convertSchemaProxy(p.Schema)
	} else if p.Type != "" {
		// v2 non-body params define type/format inline rather than via Schema
		param.Schema = &SchemaRef{
			Value: &Schema{
				Type:   p.Type,
				Format: p.Format,
			},
		}
	}
	return param
}

// convertV2BodyParam converts a Swagger 2.0 body parameter to our RequestBody type.
func convertV2BodyParam(p *v2high.Parameter) *RequestBody {
	body := &RequestBody{
		Description: p.Description,
		Required:    p.Required != nil && *p.Required,
	}
	if p.Schema != nil {
		body.Content = map[string]*MediaType{
			"application/json": {
				Schema: convertSchemaProxy(p.Schema),
			},
		}
	}
	return body
}

// convertV2Responses converts Swagger 2.0 Responses to our Responses type.
func convertV2Responses(r *v2high.Responses) *Responses {
	resp := &Responses{}

	if r.Codes != nil {
		for code, response := range r.Codes.FromOldest() {
			entry := ResponseEntry{
				Code:        code,
				Description: response.Description,
			}
			if response.Schema != nil {
				entry.Content = map[string]*MediaType{
					"application/json": {
						Schema: convertSchemaProxy(response.Schema),
					},
				}
			}
			resp.Codes = append(resp.Codes, entry)
		}
	}

	if r.Default != nil {
		entry := ResponseEntry{
			Code:        "default",
			Description: r.Default.Description,
		}
		if r.Default.Schema != nil {
			entry.Content = map[string]*MediaType{
				"application/json": {
					Schema: convertSchemaProxy(r.Default.Schema),
				},
			}
		}
		resp.Codes = append(resp.Codes, entry)
	}

	return resp
}
