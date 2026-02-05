package api

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
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
}

// NewDescribeCmd creates the 'astro api <cmdName> describe' command.
// cmdName should be "cloud" or "airflow".
func NewDescribeCmd(out io.Writer, specCache *openapi.Cache, cmdName string) *cobra.Command {
	opts := &DescribeOptions{
		Out:       out,
		specCache: specCache,
	}

	// Build examples based on command name
	pathExample := "/organizations/{organizationId}/deployments"
	opIDExample := "CreateDeployment"
	if cmdName == "airflow" {
		pathExample = "/dags/{dag_id}"
		opIDExample = "get_dag"
	}

	cmd := &cobra.Command{
		Use:   "describe <endpoint>",
		Short: "Describe an API endpoint's request and response schema",
		Long: `Show detailed information about an API endpoint, including:
- Path and query parameters
- Request body schema (for POST/PUT/PATCH)
- Response schema

The endpoint can be specified as a path or as an operation ID.`,
		Example: fmt.Sprintf(`  # Describe an endpoint by path
  astro api %s describe %s

  # Describe a POST endpoint specifically
  astro api %s describe %s -X POST

  # Describe by operation ID
  astro api %s describe %s`, cmdName, pathExample, cmdName, pathExample, cmdName, opIDExample),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Endpoint = args[0]
			return runDescribe(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "", "HTTP method (GET, POST, PUT, PATCH, DELETE)")
	cmd.Flags().BoolVar(&opts.Refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")

	return cmd
}

// runDescribe executes the describe command.
func runDescribe(opts *DescribeOptions) error {
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
	if ep.RequestBody != nil {
		printRequestBody(out, ep.RequestBody, resolver)
	}

	// Responses
	printResponses(out, ep.Responses, resolver)
}

// printParameters prints parameter information.
func printParameters(out io.Writer, params []openapi.Parameter) {
	if len(params) == 0 {
		return
	}

	// Group by location
	pathParams := filterParams(params, "path")
	queryParams := filterParams(params, "query")
	headerParams := filterParams(params, "header")

	if len(pathParams) > 0 {
		fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Path Parameters:"))
		for _, p := range pathParams {
			printParam(out, p)
		}
	}

	if len(queryParams) > 0 {
		fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Query Parameters:"))
		for _, p := range queryParams {
			printParam(out, p)
		}
	}

	if len(headerParams) > 0 {
		fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Header Parameters:"))
		for _, p := range headerParams {
			printParam(out, p)
		}
	}
}

// filterParams filters parameters by location.
func filterParams(params []openapi.Parameter, in string) []openapi.Parameter {
	var result []openapi.Parameter
	for _, p := range params {
		if p.In == in {
			result = append(result, p)
		}
	}
	return result
}

// printParam prints a single parameter.
func printParam(out io.Writer, p openapi.Parameter) {
	required := ""
	if p.Required {
		required = color.RedString(" (required)")
	}

	typeStr := "string"
	if p.Schema != nil && p.Schema.Type != "" {
		typeStr = p.Schema.Type
		if p.Schema.Format != "" {
			typeStr += " (" + p.Schema.Format + ")"
		}
		if p.Schema.Type == "array" && p.Schema.Items != nil {
			typeStr = "array of " + p.Schema.Items.Type
		}
	}

	fmt.Fprintf(out, "  %s%s  %s\n", color.GreenString(p.Name), required, color.HiBlackString(typeStr))
	if p.Description != "" {
		fmt.Fprintf(out, "      %s\n", p.Description)
	}
	if p.Schema != nil {
		if p.Schema.Default != nil {
			fmt.Fprintf(out, "      Default: %v\n", p.Schema.Default)
		}
		if len(p.Schema.Enum) > 0 {
			fmt.Fprintf(out, "      Enum: %v\n", p.Schema.Enum)
		}
	}
}

// printRequestBody prints request body schema.
func printRequestBody(out io.Writer, body *openapi.RequestBody, resolver *openapi.SchemaResolver) {
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
			schema, refName := resolver.ResolveSchema(mt.Schema)
			if refName != "" {
				fmt.Fprintf(out, "  Schema: %s\n", color.CyanString(refName))
			}
			if schema != nil {
				printSchema(out, schema, resolver, 2, make(map[string]bool), requestSchemaPrintOpts())
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
func printResponses(out io.Writer, responses map[string]openapi.Response, resolver *openapi.SchemaResolver) {
	if len(responses) == 0 {
		return
	}

	fmt.Fprintf(out, "\n%s\n", color.New(color.Bold).Sprint("Responses:"))

	// Sort response codes
	codes := make([]string, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		resp := responses[code]

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

		fmt.Fprintf(out, "  %s: %s\n", codeColor(code), resp.Description)

		// Show success response schema (2xx)
		isSuccess := parseErr == nil && codeInt >= 200 && codeInt < 300
		if isSuccess && resp.Content != nil {
			if mt, ok := resp.Content["application/json"]; ok && mt.Schema != nil {
				schema, refName := resolver.ResolveSchema(mt.Schema)
				if refName != "" {
					fmt.Fprintf(out, "    Schema: %s\n", color.CyanString(refName))
				}
				if schema != nil && len(schema.Properties) > 0 {
					printSchema(out, schema, resolver, baseResponseDepth, make(map[string]bool), responseSchemaPrintOpts())
				}
			}
		}
	}
}

// printSchema prints a schema with proper indentation.
// The schemaPrintOpts control which features (composition, defaults, etc.) are rendered.
//
//nolint:gocognit,gocyclo // Complex but necessary for comprehensive schema display
func printSchema(out io.Writer, schema *openapi.Schema, resolver *openapi.SchemaResolver, indent int, visited map[string]bool, opts schemaPrintOpts) {
	if schema == nil {
		return
	}

	prefix := strings.Repeat(" ", indent)

	// Handle $ref
	if schema.Ref != "" {
		resolved, refName := resolver.ResolveSchema(schema)
		if resolved != nil {
			if visited[refName] {
				fmt.Fprintf(out, "%s(see %s above)\n", prefix, refName)
				return
			}
			visited[refName] = true
			printSchema(out, resolved, resolver, indent, visited, opts)
		}
		return
	}

	// Handle oneOf / anyOf / allOf when composition is enabled
	if opts.ShowComposition {
		if len(schema.OneOf) > 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, color.HiBlackString("One of the following:"))
			for i := range schema.OneOf {
				resolved, refName := resolver.ResolveSchema(&schema.OneOf[i])
				if refName != "" {
					fmt.Fprintf(out, "\n%s%s %s\n", prefix, color.CyanString("Option %d:", i+1), refName)
				} else {
					fmt.Fprintf(out, "\n%s%s\n", prefix, color.CyanString("Option %d:", i+1))
				}
				if resolved != nil {
					if visited[refName] {
						fmt.Fprintf(out, "%s  (see %s above)\n", prefix, refName)
					} else {
						if refName != "" {
							visited[refName] = true
						}
						printSchema(out, resolved, resolver, indent+2, visited, opts)
					}
				}
			}
			return
		}

		if len(schema.AnyOf) > 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, color.HiBlackString("Any of the following:"))
			for i := range schema.AnyOf {
				resolved, refName := resolver.ResolveSchema(&schema.AnyOf[i])
				if refName != "" {
					fmt.Fprintf(out, "\n%s%s %s\n", prefix, color.CyanString("Option %d:", i+1), refName)
				} else {
					fmt.Fprintf(out, "\n%s%s\n", prefix, color.CyanString("Option %d:", i+1))
				}
				if resolved != nil {
					if refName != "" {
						visited[refName] = true
					}
					printSchema(out, resolved, resolver, indent+2, visited, opts)
				}
			}
			return
		}

		if len(schema.AllOf) > 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, color.HiBlackString("All of the following:"))
			for i := range schema.AllOf {
				resolved, refName := resolver.ResolveSchema(&schema.AllOf[i])
				if resolved != nil {
					if refName != "" {
						visited[refName] = true
					}
					printSchema(out, resolved, resolver, indent, visited, opts)
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
			prop := schema.Properties[name]

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

			// Resolve the property schema if it's a $ref
			propSchema := &prop
			refName := ""
			if prop.Ref != "" {
				propSchema, refName = resolver.ResolveSchema(&prop)
				if propSchema == nil {
					propSchema = &prop
				}
			}

			typeStr := getTypeString(propSchema, refName)
			fmt.Fprintf(out, "%s%s%s  %s\n", prefix, color.GreenString(name), required, color.HiBlackString(typeStr))

			if propSchema.Description != "" {
				fmt.Fprintf(out, "%s    %s\n", prefix, propSchema.Description)
			}

			if propSchema.Example != nil {
				fmt.Fprintf(out, "%s    Example: %v\n", prefix, propSchema.Example)
			}

			if len(propSchema.Enum) > 0 {
				fmt.Fprintf(out, "%s    Enum: %v\n", prefix, propSchema.Enum)
			}

			if opts.ShowDefault && propSchema.Default != nil {
				fmt.Fprintf(out, "%s    Default: %v\n", prefix, propSchema.Default)
			}

			// Show nested object properties (but limit depth)
			if indent < opts.MaxIndent && propSchema.Type == schemaTypeObject && len(propSchema.Properties) > 0 {
				printSchema(out, propSchema, resolver, indent+indentIncrement, visited, opts)
			}

			// Handle nested composition in properties
			if opts.ShowComposition && (len(propSchema.OneOf) > 0 || len(propSchema.AnyOf) > 0 || len(propSchema.AllOf) > 0) {
				printSchema(out, propSchema, resolver, indent+indentIncrement, visited, opts)
			}
		}
	}
}

// getTypeString returns a human-readable type string for a schema.
func getTypeString(schema *openapi.Schema, refName string) string {
	if schema == nil {
		return "any"
	}

	if refName != "" {
		return refName
	}

	typeStr := schema.Type
	if typeStr == "" {
		typeStr = "object"
	}

	if schema.Format != "" {
		typeStr += " (" + schema.Format + ")"
	}

	if schema.Type == "array" && schema.Items != nil {
		itemType := schema.Items.Type
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
