package api

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/fatih/color"
	"github.com/getkin/kin-openapi/openapi3"
)

const (
	schemaTypeObject  = "object"
	separatorWidth    = 60
	indentIncrement   = 4
	maxSchemaIndent   = 10
	baseResponseDepth = 4
)

// DescribeOptions holds options for the describe command.
type DescribeOptions struct {
	Out       io.Writer
	specCache *openapi.Cache
	Endpoint  string
	Method    string
	Refresh   bool
	Verbose   bool
}

// runDescribe executes the describe command.
func runDescribe(opts *DescribeOptions) error {
	if opts.specCache == nil {
		return fmt.Errorf("API specification not initialized. Ensure you are logged in and try again")
	}

	if opts.Verbose {
		fmt.Fprintf(opts.Out, "Spec URL: %s\n\n", opts.specCache.GetSpecURL())
	}

	// Load OpenAPI spec
	if err := opts.specCache.Load(opts.Refresh); err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	spec := opts.specCache.GetSpec()
	endpoints := opts.specCache.GetEndpoints()
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints found in API specification")
	}

	// Find the endpoint
	var matches []openapi.Endpoint

	// First, try to find by operation ID
	for i := range endpoints {
		if strings.EqualFold(endpoints[i].OperationID, opts.Endpoint) {
			matches = append(matches, endpoints[i])
		}
	}

	// If no match by operation ID, try by path
	if len(matches) == 0 {
		path := opts.Endpoint
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		for i := range endpoints {
			if endpoints[i].Path == path {
				matches = append(matches, endpoints[i])
			}
		}
	}

	// Filter by method if specified
	if opts.Method != "" && len(matches) > 0 {
		method := strings.ToUpper(opts.Method)
		var filtered []openapi.Endpoint
		for i := range matches {
			if matches[i].Method == method {
				filtered = append(filtered, matches[i])
			}
		}
		matches = filtered
	}

	if len(matches) == 0 {
		return fmt.Errorf("no endpoint found matching '%s'", opts.Endpoint)
	}

	// If multiple matches and no method specified, show all
	resolver := openapi.NewSchemaResolver(spec)

	for i := range matches {
		if i > 0 {
			fmt.Fprintln(opts.Out, "\n"+strings.Repeat("─", separatorWidth)+"\n")
		}
		printEndpointDetails(opts.Out, &matches[i], resolver)
	}

	return nil
}

// printEndpointDetails prints detailed information about an endpoint.
func printEndpointDetails(out io.Writer, ep *openapi.Endpoint, resolver *openapi.SchemaResolver) {
	// Header
	method := colorizeMethod(ep.Method)
	fmt.Fprintf(out, "%s %s\n", method, ep.Path)

	if ep.Deprecated {
		fmt.Fprintf(out, "%s\n", color.YellowString("⚠ DEPRECATED"))
	}

	if ep.OperationID != "" {
		fmt.Fprintf(out, "Operation ID: %s\n", color.CyanString(ep.OperationID))
	}

	if ep.Summary != "" {
		fmt.Fprintf(out, "\n%s\n", ep.Summary)
	}

	if ep.Description != "" && ep.Description != ep.Summary {
		fmt.Fprintf(out, "\n%s\n", ep.Description)
	}

	if len(ep.Tags) > 0 {
		fmt.Fprintf(out, "\nTags: %s\n", strings.Join(ep.Tags, ", "))
	}

	// Parameters
	printParameters(out, ep.Parameters)

	// Request Body
	if ep.RequestBody != nil && ep.RequestBody.Value != nil {
		printRequestBody(out, ep.RequestBody, resolver)
	}

	// Responses
	printResponses(out, ep.Responses, resolver)
}

// printParameters prints parameter information.
func printParameters(out io.Writer, params openapi3.Parameters) {
	if len(params) == 0 {
		return
	}

	// Group by location
	pathParams := filterParams(params, "path")
	queryParams := filterParams(params, "query")
	headerParams := filterParams(params, "header")

	if len(pathParams) > 0 {
		fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Path Parameters:"))
		for _, pRef := range pathParams {
			if pRef.Value != nil {
				printParam(out, pRef.Value)
			}
		}
	}

	if len(queryParams) > 0 {
		fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Query Parameters:"))
		for _, pRef := range queryParams {
			if pRef.Value != nil {
				printParam(out, pRef.Value)
			}
		}
	}

	if len(headerParams) > 0 {
		fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Header Parameters:"))
		for _, pRef := range headerParams {
			if pRef.Value != nil {
				printParam(out, pRef.Value)
			}
		}
	}
}

// filterParams filters parameters by location.
func filterParams(params openapi3.Parameters, in string) openapi3.Parameters {
	var result openapi3.Parameters
	for _, pRef := range params {
		if pRef.Value != nil && pRef.Value.In == in {
			result = append(result, pRef)
		}
	}
	return result
}

// printParam prints a single parameter.
func printParam(out io.Writer, p *openapi3.Parameter) {
	required := ""
	if p.Required {
		required = color.RedString(" (required)")
	}

	typeStr := "string"
	if p.Schema != nil && p.Schema.Value != nil {
		s := p.Schema.Value
		sType := schemaType(s)
		if sType != "" {
			typeStr = sType
			if s.Format != "" {
				typeStr += " (" + s.Format + ")"
			}
			if sType == "array" && s.Items != nil && s.Items.Value != nil {
				typeStr = "array of " + schemaType(s.Items.Value)
			}
		}
	}

	fmt.Fprintf(out, "  %s%s  %s\n", color.GreenString(p.Name), required, color.HiBlackString(typeStr))
	if p.Description != "" {
		fmt.Fprintf(out, "      %s\n", p.Description)
	}
	if p.Schema != nil && p.Schema.Value != nil {
		s := p.Schema.Value
		if s.Default != nil {
			fmt.Fprintf(out, "      Default: %v\n", s.Default)
		}
		if len(s.Enum) > 0 {
			fmt.Fprintf(out, "      Enum: %v\n", s.Enum)
		}
	}
}

// printRequestBody prints request body schema.
func printRequestBody(out io.Writer, bodyRef *openapi3.RequestBodyRef, resolver *openapi.SchemaResolver) {
	body := bodyRef.Value

	fmt.Fprintf(out, "\n%s", color.New(color.Bold).Sprint("Request Body"))
	if body.Required {
		fmt.Fprintf(out, " %s", color.RedString("(required)"))
	}
	fmt.Fprintln(out, ":")

	if body.Description != "" {
		fmt.Fprintf(out, "  %s\n", body.Description)
	}

	// Get the JSON schema
	if body.Content != nil {
		if mt, ok := body.Content["application/json"]; ok && mt.Schema != nil {
			_, refName := resolver.ResolveSchema(mt.Schema)
			if refName != "" {
				fmt.Fprintf(out, "  Schema: %s\n", color.CyanString(refName))
			}
			if mt.Schema.Value != nil {
				printSchema(out, mt.Schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
			}
		}
	}
}

// schemaPrintOpts controls the behavior of printSchema.
type schemaPrintOpts struct {
	// SkipReadOnly hides read-only properties (useful for request-body schemas
	// where those fields aren't accepted by the server).
	SkipReadOnly bool
	// ShowComposition enables printing of oneOf/anyOf/allOf branches.
	ShowComposition bool
	// ShowDefault enables printing of default values.
	ShowDefault bool
	// MaxIndent limits recursion depth for nested objects.
	MaxIndent int
}

// requestSchemaPrintOpts returns options appropriate for request-body schemas.
func requestSchemaPrintOpts() schemaPrintOpts {
	return schemaPrintOpts{
		SkipReadOnly:    true,
		ShowComposition: true,
		ShowDefault:     true,
		MaxIndent:       maxSchemaIndent,
	}
}

// responseSchemaPrintOpts returns options appropriate for response schemas.
func responseSchemaPrintOpts() schemaPrintOpts {
	return schemaPrintOpts{
		ShowComposition: true,
		ShowDefault:     false,
		MaxIndent:       maxSchemaIndent,
	}
}

// printResponses prints response information.
func printResponses(out io.Writer, responses *openapi3.Responses, resolver *openapi.SchemaResolver) {
	if responses == nil || responses.Len() == 0 {
		return
	}

	fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Responses:"))

	// Sort response codes
	codes := make([]string, 0, responses.Len())
	for code := range responses.Map() {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		respRef := responses.Value(code)
		if respRef == nil || respRef.Value == nil {
			continue
		}
		resp := respRef.Value

		description := ""
		if resp.Description != nil {
			description = *resp.Description
		}

		// Parse status code as integer; non-numeric keys like "default" stay green.
		codeInt, parseErr := strconv.Atoi(code)
		codeColor := color.GreenString
		if parseErr == nil {
			if codeInt >= httpStatusError {
				codeColor = color.RedString
			} else if codeInt >= httpStatusRedirect {
				codeColor = color.YellowString
			}
		}

		fmt.Fprintf(out, "  %s: %s\n", codeColor(code), description)

		// Show success response schema (2xx)
		isSuccess := parseErr == nil && codeInt >= 200 && codeInt < 300
		if isSuccess && resp.Content != nil {
			if mt, ok := resp.Content["application/json"]; ok && mt.Schema != nil {
				_, refName := resolver.ResolveSchema(mt.Schema)
				if refName != "" {
					fmt.Fprintf(out, "    Schema: %s\n", color.CyanString(refName))
				}
				if mt.Schema.Value != nil && len(mt.Schema.Value.Properties) > 0 {
					printSchema(out, mt.Schema, resolver, baseResponseDepth, make(map[string]bool), responseSchemaPrintOpts())
				}
			}
		}
	}
}

// printSchema prints a schema with proper indentation.
// The schemaPrintOpts control which features (composition, defaults, etc.) are rendered.
//
//nolint:gocognit,gocyclo // Complex but necessary for comprehensive schema display
func printSchema(out io.Writer, schemaRef *openapi3.SchemaRef, resolver *openapi.SchemaResolver, indent int, visited map[string]bool, opts schemaPrintOpts) {
	if schemaRef == nil || schemaRef.Value == nil {
		return
	}

	schema := schemaRef.Value
	prefix := strings.Repeat(" ", indent)

	// Handle $ref (cycle detection)
	if schemaRef.Ref != "" {
		_, refName := resolver.ResolveSchema(schemaRef)
		if visited[refName] {
			fmt.Fprintf(out, "%s(see %s above)\n", prefix, refName)
			return
		}
		visited[refName] = true
	}

	// Handle oneOf / anyOf / allOf when composition is enabled
	if opts.ShowComposition {
		if len(schema.OneOf) > 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, color.HiBlackString("One of the following:"))
			for i, childRef := range schema.OneOf {
				_, refName := resolver.ResolveSchema(childRef)
				if refName != "" {
					fmt.Fprintf(out, "\n%s%s %s\n", prefix, color.CyanString("Option %d:", i+1), refName)
				} else {
					fmt.Fprintf(out, "\n%s%s\n", prefix, color.CyanString("Option %d:", i+1))
				}
				if childRef.Value != nil {
					if visited[refName] {
						fmt.Fprintf(out, "%s  (see %s above)\n", prefix, refName)
					} else {
						if refName != "" {
							visited[refName] = true
						}
						printSchema(out, childRef, resolver, indent+2, visited, opts)
					}
				}
			}
			return
		}

		if len(schema.AnyOf) > 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, color.HiBlackString("Any of the following:"))
			for i, childRef := range schema.AnyOf {
				_, refName := resolver.ResolveSchema(childRef)
				if refName != "" {
					fmt.Fprintf(out, "\n%s%s %s\n", prefix, color.CyanString("Option %d:", i+1), refName)
				} else {
					fmt.Fprintf(out, "\n%s%s\n", prefix, color.CyanString("Option %d:", i+1))
				}
				if childRef.Value != nil {
					if refName != "" {
						visited[refName] = true
					}
					printSchema(out, childRef, resolver, indent+2, visited, opts)
				}
			}
			return
		}

		if len(schema.AllOf) > 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, color.HiBlackString("All of the following:"))
			for _, childRef := range schema.AllOf {
				if childRef.Value != nil {
					_, refName := resolver.ResolveSchema(childRef)
					if refName != "" {
						visited[refName] = true
					}
					printSchema(out, childRef, resolver, indent, visited, opts)
				}
			}
			return
		}
	}

	// Print properties
	if len(schema.Properties) > 0 {
		propNames := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)

		for _, name := range propNames {
			propRef := schema.Properties[name]
			if propRef == nil || propRef.Value == nil {
				continue
			}
			prop := propRef.Value

			// Skip read-only properties in request schemas (they're server-set
			// and not accepted in the request body). Response schemas keep them
			// because those are the fields the API actually returns.
			if opts.SkipReadOnly && prop.ReadOnly {
				continue
			}

			required := ""
			if openapi.IsRequired(name, schema.Required) {
				required = color.RedString("*")
			}

			// Extract ref name if this is a $ref property
			refName := ""
			if propRef.Ref != "" {
				_, refName = resolver.ResolveSchema(propRef)
			}

			typeStr := getTypeString(prop, refName)
			fmt.Fprintf(out, "%s%s%s  %s\n", prefix, color.GreenString(name), required, color.HiBlackString(typeStr))

			if prop.Description != "" {
				fmt.Fprintf(out, "%s    %s\n", prefix, prop.Description)
			}

			if prop.Example != nil {
				fmt.Fprintf(out, "%s    Example: %v\n", prefix, prop.Example)
			}

			if len(prop.Enum) > 0 {
				fmt.Fprintf(out, "%s    Enum: %v\n", prefix, prop.Enum)
			}

			if opts.ShowDefault && prop.Default != nil {
				fmt.Fprintf(out, "%s    Default: %v\n", prefix, prop.Default)
			}

			propType := schemaType(prop)

			// Show nested object properties (but limit depth)
			if indent < opts.MaxIndent && propType == schemaTypeObject && len(prop.Properties) > 0 {
				printSchema(out, propRef, resolver, indent+indentIncrement, visited, opts)
			}

			// Handle nested composition in properties
			if opts.ShowComposition && (len(prop.OneOf) > 0 || len(prop.AnyOf) > 0 || len(prop.AllOf) > 0) {
				printSchema(out, propRef, resolver, indent+indentIncrement, visited, opts)
			}
		}
	}
}

// getTypeString returns a human-readable type string for a schema.
func getTypeString(schema *openapi3.Schema, refName string) string {
	if schema == nil {
		return "any"
	}

	if refName != "" {
		return refName
	}

	typeStr := schemaType(schema)
	if typeStr == "" {
		typeStr = "object"
	}

	if schema.Format != "" {
		typeStr += " (" + schema.Format + ")"
	}

	if schemaType(schema) == "array" && schema.Items != nil {
		itemType := ""
		if schema.Items.Value != nil {
			itemType = schemaType(schema.Items.Value)
		}
		if schema.Items.Ref != "" {
			// Extract ref name
			parts := strings.Split(schema.Items.Ref, "/")
			itemType = parts[len(parts)-1]
		}
		if itemType == "" {
			itemType = "object"
		}
		typeStr = "array of " + itemType
	}

	return typeStr
}

// schemaType returns the primary type string from an OpenAPI schema.
func schemaType(schema *openapi3.Schema) string {
	if schema == nil || schema.Type == nil {
		return ""
	}
	types := schema.Type.Slice()
	if len(types) > 0 {
		return types[0]
	}
	return ""
}
