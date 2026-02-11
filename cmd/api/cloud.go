package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/spf13/cobra"
)

// CloudOptions holds all options for the cloud api command.
type CloudOptions struct {
	RequestOptions
}

// NewCloudCmd creates the 'astro api cloud' command.
//
//nolint:dupl
func NewCloudCmd(out io.Writer) *cobra.Command {
	opts := &CloudOptions{
		RequestOptions: RequestOptions{
			Out:    out,
			ErrOut: os.Stderr,
			// specCache is initialized lazily when the domain is known
		},
	}

	cmd := &cobra.Command{
		Use:   "cloud <endpoint | operation-id>",
		Short: "Make authenticated requests to the Astro Cloud API",
		Long: `Make authenticated HTTP requests to the Astro Cloud API (api.astronomer.io).

The argument can be either:
  - A path of an Astro Cloud API endpoint (e.g., /organizations/{organizationId})
  - An operation ID from the API spec (e.g., GetDeployment, ListOrganizations)

Placeholder values {organizationId} and {workspaceId} will be replaced
with values from the current context. Other path parameters can be provided
using the -p/--path-param flag.

The default HTTP request method is GET normally and POST if any parameters
were added. Override the method with --method. When using an operation ID,
the method is auto-detected from the API spec.

Pass one or more -f/--raw-field values in key=value format to add static string
parameters to the request payload. To add non-string or placeholder-determined
values, see -F/--field below.

The -F/--field flag has magic type conversion based on the format of the value:
  - literal values true, false, null, and integer numbers get converted to
    appropriate JSON types;
  - if the value starts with @, the rest of the value is interpreted as a
    filename to read the value from. Pass - to read from standard input.

To pass nested parameters in the request payload, use key[subkey]=value syntax.
To pass nested values as arrays, declare multiple fields with key[]=value1.`,
		Example: `  # List all Cloud API endpoints
  astro api cloud ls

  # Get organization details (auto-injects organizationId)
  astro api cloud /organizations/{organizationId}

  # Use operation ID with path parameters
  astro api cloud GetDeployment -p deploymentId=abc123

  # Use operation ID (organizationId auto-injected from context)
  astro api cloud ListDeployments

  # List deployments with jq filter
  astro api cloud /organizations/{organizationId}/deployments --jq '.[].name'

  # Create a resource with typed fields
  astro api cloud -X POST /organizations/{organizationId}/workspaces \
    -F name=my-workspace \
    -F description="My new workspace"

  # Use Go template for output
  astro api cloud /organizations/{organizationId}/deployments \
    --template '{{range .}}{{.name}} ({{.status}}){{"\n"}}{{end}}'

  # Generate curl command instead of executing
  astro api cloud /organizations/{organizationId}/deployments --generate

  # Include response headers
  astro api cloud /organizations/{organizationId} -i

  # Verbose mode showing full request/response
  astro api cloud /organizations/{organizationId} --verbose`,
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestMethodPassed = cmd.Flags().Changed("method")
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// Interactive mode - show endpoint selection
				return runCloudInteractive(opts)
			}
			opts.RequestPath = args[0]
			return runCloud(opts)
		},
	}

	// Request flags
	cmd.Flags().StringVarP(&opts.RequestMethod, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a typed parameter in key=value format")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in key=value format")
	cmd.Flags().StringArrayVarP(&opts.RequestHeaders, "header", "H", nil, "Add a HTTP request header in key:value format")
	cmd.Flags().StringVar(&opts.RequestInputFile, "input", "", "The file to use as body for the HTTP request (use \"-\" for stdin)")
	cmd.Flags().StringArrayVarP(&opts.PathParams, "path-param", "p", nil, "Path parameter in key=value format (for use with operation IDs)")

	// Output flags
	cmd.Flags().BoolVarP(&opts.ShowResponseHeaders, "include", "i", false, "Include HTTP response status line and headers in the output")
	cmd.Flags().BoolVar(&opts.Paginate, "paginate", false, "Make additional HTTP requests to fetch all pages of results")
	cmd.Flags().BoolVar(&opts.Slurp, "slurp", false, "Use with --paginate to return an array of all pages")
	cmd.Flags().BoolVar(&opts.Silent, "silent", false, "Do not print the response body")
	cmd.Flags().StringVarP(&opts.Template, "template", "t", "", "Format JSON output using a Go template")
	cmd.Flags().StringVarP(&opts.FilterOutput, "jq", "q", "", "Query to select values from the response using jq syntax")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "Include full HTTP request and response in the output")

	// Other flags
	cmd.Flags().BoolVar(&opts.GenerateCurl, "generate", false, "Output a curl command instead of executing the request")

	// Add list and describe subcommands (cloud-specific to support lazy cache init)
	cmd.AddCommand(NewCloudListCmd(out, opts))
	cmd.AddCommand(NewCloudDescribeCmd(out, opts))

	return cmd
}

// runCloud executes the cloud API request.
func runCloud(opts *CloudOptions) error {
	// Check if we're in a cloud context
	if !context.IsCloudContext() {
		return fmt.Errorf("the 'astro api cloud' command is only available in cloud context. Run 'astro login' to connect to Astro Cloud")
	}

	// Get current context for auth and placeholders
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("getting current context: %w", err)
	}

	// Check for token
	if ctx.Token == "" {
		return fmt.Errorf("not authenticated. Run 'astro login' to authenticate")
	}

	// Initialize the spec cache for this domain
	if err := initCloudSpecCache(opts, &ctx); err != nil {
		return fmt.Errorf("initializing API spec: %w", err)
	}

	// Resolve operation ID to path if needed
	requestPath := opts.RequestPath
	method := opts.RequestMethod
	methodFromSpec := false

	if isOperationID(requestPath) {
		endpoint, err := resolveOperationID(opts.specCache, requestPath, "cloud")
		if err != nil {
			return err
		}
		requestPath = endpoint.Path
		if !opts.RequestMethodPassed {
			method = endpoint.Method
			methodFromSpec = true
		}
	}

	// Apply path params from flags
	requestPath, err = applyPathParams(requestPath, opts.PathParams)
	if err != nil {
		return fmt.Errorf("applying path params: %w", err)
	}

	// Fill context placeholders in the path
	requestPath, err = fillPlaceholders(requestPath, &ctx)
	if err != nil {
		return fmt.Errorf("filling placeholders: %w", err)
	}

	// Check for any remaining unfilled path parameters
	if missing := findMissingPathParams(requestPath); len(missing) > 0 {
		return fmt.Errorf("missing path parameter(s): %s. Use -p/--path-param to provide them (e.g., -p %s=value)",
			strings.Join(missing, ", "), missing[0])
	}

	// Parse fields into request body
	params, err := parseFields(opts.MagicFields, opts.RawFields)
	if err != nil {
		return fmt.Errorf("parsing fields: %w", err)
	}

	// Determine HTTP method (only override if not from spec and not explicitly passed)
	if !methodFromSpec && !opts.RequestMethodPassed && (len(params) > 0 || opts.RequestInputFile != "") {
		method = "POST"
	}

	// Build the full URL using the domain-derived base URL
	baseURL := ctx.GetPublicRESTAPIURL("v1")
	url := buildURL(baseURL, requestPath)

	// Generate curl command if requested
	if opts.GenerateCurl {
		return generateCurl(opts.Out, method, url, ctx.Token, opts.RequestHeaders, params, opts.RequestInputFile)
	}

	// Build and execute the request
	return executeRequest(&opts.RequestOptions, method, url, ctx.Token, params)
}

// isOperationID checks if the input looks like an operation ID rather than a path.
// Operation IDs don't contain "/" and typically are CamelCase or camelCase.
func isOperationID(input string) bool {
	return !strings.Contains(input, "/")
}

// resolveOperationID looks up an operation ID in the OpenAPI spec and returns the endpoint.
// If no exact operation ID match is found, it falls back to trying the input as a path
// (with "/" prepended) to handle bare path segments like "version" or "health".
// cmdName is used only in the error message (e.g. "cloud", "airflow").
func resolveOperationID(specCache *openapi.Cache, operationID, cmdName string) (*openapi.Endpoint, error) {
	if err := specCache.Load(false); err != nil {
		return nil, fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	endpoints := specCache.GetEndpoints()

	// Try as operation ID first
	endpoint := openapi.FindEndpointByOperationID(endpoints, operationID)
	if endpoint != nil {
		return endpoint, nil
	}

	// Fall back to trying as a path (e.g., "version" -> "/version")
	pathMatches := openapi.FindEndpointByPath(endpoints, "/"+operationID)
	if len(pathMatches) > 0 {
		return &pathMatches[0], nil
	}

	return nil, fmt.Errorf("'%s' not found as operation ID or path. Use 'astro api %s ls' to see available endpoints", operationID, cmdName)
}

// applyPathParams replaces path placeholders with values from --path-param flags.
func applyPathParams(path string, pathParams []string) (string, error) {
	if len(pathParams) == 0 {
		return path, nil
	}

	// Parse path params into a map
	params := make(map[string]string)
	for _, p := range pathParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid path param format '%s', expected key=value", p)
		}
		params[parts[0]] = parts[1]
	}

	// Replace placeholders in the path
	result := placeholderRE.ReplaceAllStringFunc(path, func(match string) string {
		name := match[1 : len(match)-1]
		if val, ok := params[name]; ok {
			return val
		}
		return match
	})

	return result, nil
}

// runCloudInteractive runs the cloud API command in interactive mode.
func runCloudInteractive(opts *CloudOptions) error {
	// Check if we're in a cloud context
	if !context.IsCloudContext() {
		return fmt.Errorf("the 'astro api cloud' command is only available in cloud context. Run 'astro login' to connect to Astro Cloud")
	}

	// Get current context for cache initialization
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("getting current context: %w", err)
	}

	// Initialize the spec cache for this domain
	if err := initCloudSpecCache(opts, &ctx); err != nil {
		return fmt.Errorf("initializing API spec: %w", err)
	}

	// Load OpenAPI spec
	if err := opts.specCache.Load(false); err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	endpoints := opts.specCache.GetEndpoints()
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints found in API specification")
	}

	// Show endpoint selection
	fmt.Fprintf(opts.Out, "\nFound %d endpoints. Use '%s' to list them.\n", len(endpoints), ansi.Bold("astro api cloud ls"))
	fmt.Fprintf(opts.Out, "Run '%s' to make a request.\n\n", ansi.Bold("astro api cloud <endpoint>"))

	return nil
}

// placeholderRE matches placeholders like {organizationId}, {workspaceId}, {dag_id}, etc.
var placeholderRE = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9_]*)\}`)

// fillPlaceholders replaces placeholders with values from the context.
func fillPlaceholders(path string, ctx *config.Context) (string, error) {
	var errs []string

	result := placeholderRE.ReplaceAllStringFunc(path, func(match string) string {
		// Extract the name without braces
		name := match[1 : len(match)-1]

		switch strings.ToLower(name) {
		case "organizationid":
			if ctx.Organization == "" {
				errs = append(errs, "organizationId not set in context (run 'astro organization switch')")
				return match
			}
			return ctx.Organization
		case "workspaceid":
			if ctx.Workspace == "" {
				errs = append(errs, "workspaceId not set in context (run 'astro workspace switch')")
				return match
			}
			return ctx.Workspace
		default:
			// Keep unknown placeholders as-is (user might provide them)
			return match
		}
	})

	if len(errs) > 0 {
		return result, fmt.Errorf("placeholder error: %s", strings.Join(errs, "; "))
	}

	return result, nil
}

// findMissingPathParams returns any unfilled path parameters in the path.
func findMissingPathParams(path string) []string {
	matches := placeholderRE.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}

	missing := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			missing = append(missing, match[1])
		}
	}
	return missing
}

// initCloudSpecCache initializes the OpenAPI spec cache for the current cloud
// context's domain. If the cache is already initialized, this is a no-op.
func initCloudSpecCache(opts *CloudOptions, ctx *config.Context) error {
	if opts.specCache != nil {
		return nil
	}
	domain := domainutil.FormatDomain(ctx.Domain)
	specURL := ctx.GetPublicRESTAPIURL("spec/v1.0")
	if specURL == "" {
		return fmt.Errorf("could not determine API spec URL for domain %q. Check your login context", ctx.Domain)
	}
	cachePath := filepath.Join(config.HomeConfigPath, openapi.CloudCacheFileNameForDomain(domain))
	opts.specCache = openapi.NewCacheWithOptions(specURL, cachePath)
	return nil
}

// NewCloudListCmd creates the 'astro api cloud ls' command.
// It lazily initializes the spec cache so the correct domain-specific spec URL is used.
func NewCloudListCmd(out io.Writer, parentOpts *CloudOptions) *cobra.Command {
	var verbose bool
	var refresh bool

	cmd := &cobra.Command{
		Use:     "ls [filter]",
		Aliases: []string{"list"},
		Short:   "List available Astro Cloud API endpoints",
		Long: `List all available endpoints from the Astro Cloud API.

You can optionally provide a filter to search for specific endpoints.
The filter matches against endpoint paths, methods, operation IDs, summaries, and tags.`,
		Example: `  # List all endpoints
  astro api cloud ls

  # Filter endpoints
  astro api cloud ls deployments

  # List POST endpoints
  astro api cloud ls POST

  # Show verbose output with descriptions
  astro api cloud ls --verbose`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var filter string
			if len(args) > 0 {
				filter = args[0]
			}

			if !context.IsCloudContext() {
				return fmt.Errorf("the 'astro api cloud' command is only available in cloud context. Run 'astro login' to connect to Astro Cloud")
			}

			ctx, err := context.GetCurrentContext()
			if err != nil {
				return fmt.Errorf("getting current context: %w", err)
			}

			if err := initCloudSpecCache(parentOpts, &ctx); err != nil {
				return fmt.Errorf("initializing API spec: %w", err)
			}

			listOpts := &ListOptions{
				Out:       out,
				specCache: parentOpts.specCache,
				Filter:    filter,
				Verbose:   verbose,
				Refresh:   refresh,
			}
			return runList(listOpts)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show additional details like summaries and tags")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")

	return cmd
}

// NewCloudDescribeCmd creates the 'astro api cloud describe' command.
// It lazily initializes the spec cache so the correct domain-specific spec URL is used.
func NewCloudDescribeCmd(out io.Writer, parentOpts *CloudOptions) *cobra.Command {
	var method string
	var refresh bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "describe <endpoint>",
		Short: "Describe an Astro Cloud API endpoint's request and response schema",
		Long: `Show detailed information about an Astro Cloud API endpoint, including:
- Path and query parameters
- Request body schema (for POST/PUT/PATCH)
- Response schema

The endpoint can be specified as a path or as an operation ID.`,
		Example: `  # Describe an endpoint by path
  astro api cloud describe /organizations/{organizationId}/deployments

  # Describe a POST endpoint specifically
  astro api cloud describe /organizations/{organizationId}/deployments -X POST

  # Describe by operation ID
  astro api cloud describe CreateDeployment`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !context.IsCloudContext() {
				return fmt.Errorf("the 'astro api cloud' command is only available in cloud context. Run 'astro login' to connect to Astro Cloud")
			}

			ctx, err := context.GetCurrentContext()
			if err != nil {
				return fmt.Errorf("getting current context: %w", err)
			}

			if err := initCloudSpecCache(parentOpts, &ctx); err != nil {
				return fmt.Errorf("initializing API spec: %w", err)
			}

			descOpts := &DescribeOptions{
				Out:       out,
				specCache: parentOpts.specCache,
				Endpoint:  args[0],
				Method:    method,
				Refresh:   refresh,
				Verbose:   verbose,
			}
			return runDescribe(descOpts)
		},
	}

	cmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method (GET, POST, PUT, PATCH, DELETE)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show spec URL and additional details")

	return cmd
}

// buildURL constructs the full URL from base and path.
func buildURL(baseURL, path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(baseURL, "/") + path
}
