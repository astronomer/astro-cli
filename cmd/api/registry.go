package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/openapi"
)

const (
	defaultRegistryURL      = "https://airflow.apache.org/registry"
	registryURLEnv          = "ASTRO_REGISTRY_URL"
	registrySpecPath        = "/api/openapi.json"
	registryCacheFileName   = "openapi-registry-cache.json"
	registryStripPathPrefix = "/api"
)

// RegistryOptions holds all options for the registry api command.
type RegistryOptions struct {
	RequestOptions

	RegistryURL string
}

// NewRegistryCmd creates the 'astro api registry' command.
//
//nolint:dupl
func NewRegistryCmd(out io.Writer) *cobra.Command {
	opts := &RegistryOptions{
		RequestOptions: RequestOptions{
			Out:    out,
			ErrOut: os.Stderr,
		},
	}

	cmd := &cobra.Command{
		Use:   "registry <endpoint | operation-id>",
		Short: "Make requests to the Airflow Provider Registry API",
		Long: `Make HTTP requests to the Airflow Provider Registry API.

The argument can be either:
  - A path of a registry API endpoint (e.g., /providers.json, /providers/amazon/modules.json)
  - An operation ID from the API spec (e.g., listProviders, getProviderModulesLatest)

The /api prefix is added automatically to paths. No authentication is required.

Use "astro api registry ls" to discover all available endpoints and operation IDs.`,
		Example: `  # List registry API endpoints
  astro api registry ls

  # Query by operation ID with path parameters
  astro api registry getProviderModulesLatest -p providerId=amazon
  astro api registry getProviderModulesByVersion -p providerId=amazon -p version=9.22.0

  # Query by path
  astro api registry /providers.json
  astro api registry /providers/amazon/modules.json
  astro api registry /providers/amazon/9.22.0/modules.json
  astro api registry /modules.json

  # Use jq filter on response
  astro api registry /providers.json --jq '.providers[0].id'

  # Use Go template for output
  astro api registry listProviders \
    --template '{{range .providers}}{{.id}}{{"\n"}}{{end}}'

  # Generate curl command instead of executing
  astro api registry /providers.json --generate

  # Verbose mode showing full request/response
  astro api registry /providers.json --verbose

  # Override registry URL
  astro api registry --registry-url https://custom.example.com /providers.json`,
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestMethodPassed = cmd.Flags().Changed("method")
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runRegistryInteractive(opts)
			}
			opts.RequestPath = args[0]
			return runRegistry(opts)
		},
	}

	// Registry-specific persistent flags
	cmd.PersistentFlags().StringVar(&opts.RegistryURL, "registry-url", "", "Override the registry base URL (env: ASTRO_REGISTRY_URL)")

	// Request flags
	cmd.Flags().StringVarP(&opts.RequestMethod, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a typed parameter in key=value format")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in key=value format")
	cmd.Flags().StringArrayVarP(&opts.RequestHeaders, "header", "H", nil, "Add a HTTP request header in key:value format")
	cmd.Flags().StringVar(&opts.RequestInputFile, "input", "", "The file to use as body for the HTTP request (use \"-\" for stdin)")
	cmd.Flags().StringArrayVarP(&opts.PathParams, "path-param", "p", nil, "Path parameter in key=value format (for use with operation IDs)")

	// Output flags
	cmd.Flags().BoolVarP(&opts.ShowResponseHeaders, "include", "i", false, "Include HTTP response status line and headers in the output")
	cmd.Flags().BoolVar(&opts.Silent, "silent", false, "Do not print the response body")
	cmd.Flags().StringVarP(&opts.Template, "template", "t", "", "Format JSON output using a Go template")
	cmd.Flags().StringVarP(&opts.FilterOutput, "jq", "q", "", "Query to select values from the response using jq syntax")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "Include full HTTP request and response in the output")

	// Other flags
	cmd.Flags().BoolVar(&opts.GenerateCurl, "generate", false, "Output a curl command instead of executing the request")

	// Subcommands
	cmd.AddCommand(NewRegistryListCmd(out, opts))
	cmd.AddCommand(NewRegistryDescribeCmd(out, opts))

	return cmd
}

// runRegistry executes a registry API request.
func runRegistry(opts *RegistryOptions) error {
	baseURL := resolveRegistryURL(opts.RegistryURL)

	// Initialize the spec cache for operation ID resolution
	initRegistrySpecCache(opts)

	// Resolve operation ID to path if needed
	requestPath := opts.RequestPath
	method := opts.RequestMethod
	methodFromSpec := false

	if isOperationID(requestPath) {
		endpoint, err := resolveOperationID(opts.specCache, requestPath, "registry")
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
	var err error
	requestPath, err = applyPathParams(requestPath, opts.PathParams)
	if err != nil {
		return fmt.Errorf("applying path params: %w", err)
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

	// Build the full URL
	url := registryBuildGenericURL(baseURL, requestPath)

	// Generate curl command if requested
	if opts.GenerateCurl {
		return generateCurl(opts.Out, method, url, "", opts.RequestHeaders, params, opts.RequestInputFile)
	}

	// Build and execute the request
	err = executeRequest(&opts.RequestOptions, method, url, "", params)
	if isConnectionError(err) {
		return registryConnectionError(url)
	}
	return err
}

// runRegistryInteractive runs the registry command in interactive mode (no args).
func runRegistryInteractive(opts *RegistryOptions) error {
	initRegistrySpecCache(opts)

	if err := opts.specCache.Load(false); err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	endpoints := opts.specCache.GetEndpoints()
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints found in API specification")
	}

	fmt.Fprintf(opts.Out, "\nFound %d endpoints. Use 'astro api registry ls' to list them.\n", len(endpoints))
	fmt.Fprintf(opts.Out, "Run 'astro api registry <endpoint>' to make a request.\n\n")

	return nil
}

// NewRegistryListCmd creates the 'astro api registry ls' command.
func NewRegistryListCmd(out io.Writer, parentOpts *RegistryOptions) *cobra.Command {
	var verbose bool
	var refresh bool

	cmd := &cobra.Command{
		Use:     "ls [filter]",
		Aliases: []string{"list"},
		Short:   "List available registry API endpoints",
		Long: `List all available endpoints from the Airflow Provider Registry API.

You can optionally provide a filter to search for specific endpoints.
The filter matches against endpoint paths, methods, operation IDs, summaries, and tags.`,
		Example: `  # List all endpoints
  astro api registry ls

  # Filter endpoints
  astro api registry ls providers

  # Show verbose output with descriptions
  astro api registry ls --verbose`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var filter string
			if len(args) > 0 {
				filter = args[0]
			}

			initRegistrySpecCache(parentOpts)

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

// NewRegistryDescribeCmd creates the 'astro api registry describe' command.
func NewRegistryDescribeCmd(out io.Writer, parentOpts *RegistryOptions) *cobra.Command {
	var method string
	var refresh bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "describe <endpoint>",
		Short: "Describe a registry API endpoint's request and response schema",
		Long: `Show detailed information about a registry API endpoint, including:
- Path parameters
- Response schema

The endpoint can be specified as a path or as an operation ID.`,
		Example: `  # Describe an endpoint by path
  astro api registry describe /providers/{providerId}/modules.json

  # Describe by operation ID
  astro api registry describe getProviderModulesLatest`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			initRegistrySpecCache(parentOpts)

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

	cmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method (GET)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show spec URL and additional details")

	return cmd
}

// initRegistrySpecCache initializes the OpenAPI spec cache for the registry.
func initRegistrySpecCache(opts *RegistryOptions) {
	if opts.specCache != nil {
		return
	}
	base := resolveRegistryURL(opts.RegistryURL)
	specURL := base + registrySpecPath
	cachePath := filepath.Join(config.HomeConfigPath, registryCacheFileName)
	opts.specCache = openapi.NewCacheWithOptions(specURL, cachePath)
	opts.specCache.SetStripPrefix(registryStripPathPrefix)
}

// resolveRegistryURL returns the registry base URL using the precedence: flag > env > default.
func resolveRegistryURL(flagURL string) string {
	if flagURL != "" {
		return strings.TrimRight(flagURL, "/")
	}
	if envURL := os.Getenv(registryURLEnv); envURL != "" {
		return strings.TrimRight(envURL, "/")
	}
	return defaultRegistryURL
}

// registryBuildGenericURL constructs the full URL for a user-supplied path.
func registryBuildGenericURL(base, path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + "/api" + path
}

// registryConnectionError returns a user-friendly error when the registry is unreachable.
func registryConnectionError(requestURL string) error {
	return fmt.Errorf("could not connect to registry at %s\n\n"+
		"Check that the URL is correct. You can override it with:\n"+
		"  --registry-url <url>\n"+
		"  ASTRO_REGISTRY_URL=<url>", requestURL)
}
