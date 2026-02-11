package api

import (
	"bytes"
	stdctx "context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/spf13/cobra"
)

const (
	airflowAPIDefaultURL  = "http://localhost:8080/api/v2"
	defaultAirflowVersion = "3.0.3" // Fallback version when detection fails
)

// AirflowOptions holds all options for the airflow api command.
type AirflowOptions struct {
	RequestOptions

	// Airflow-specific options
	APIURL         string
	DeploymentID   string
	OrganizationID string
	WorkspaceID    string
	Username       string
	Password       string
	AirflowVersion string // Manual override for Airflow version

	// Internal
	detectedVersion     string // The Airflow version being used (detected or overridden)
	CredentialsExplicit bool   // true when --username or --password was explicitly passed
}

// NewAirflowCmd creates the 'astro api airflow' command.
//
//nolint:dupl
func NewAirflowCmd(out io.Writer) *cobra.Command {
	opts := &AirflowOptions{
		RequestOptions: RequestOptions{
			Out:    out,
			ErrOut: os.Stderr,
			// specCache is initialized lazily when we know the Airflow version
		},
	}

	cmd := &cobra.Command{
		Use:   "airflow <endpoint | operation-id>",
		Short: "Make requests to the Airflow REST API",
		Long: `Make HTTP requests to the Airflow REST API.

The argument can be either:
  - A path of an Airflow API endpoint (e.g., /dags, /dags/my_dag)
  - An operation ID from the API spec (e.g., get_dags, get_dag)

By default, requests are made to localhost:8080/api/v2. You can override
this with --api-url, or provide --deployment-id to use the Airflow API
URL from an Astro Cloud deployment.

The Airflow version is auto-detected from the target instance to load the
correct API specification. Use --airflow-version to override if the instance
is unreachable.

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
		Example: `  # List Airflow API endpoints
  astro api airflow ls

  # Get all DAGs from local Airflow (default: localhost:8080)
  astro api airflow /dags

  # Get a specific DAG by path
  astro api airflow /dags/example_dag

  # Use operation ID (path params supplied via -p)
  astro api airflow get_dag -p dag_id=example_dag

  # Pause a DAG via operation ID
  astro api airflow patch_dag -p dag_id=example_dag -F is_paused=true

  # Use jq filter on response
  astro api airflow /dags --jq '.dags[].dag_id'

  # Use custom API URL
  astro api airflow --api-url http://airflow.example.com:8080/api/v2 /dags

  # Use Airflow from a specific Astro Cloud deployment
  astro api airflow --deployment-id clxyz123 /dags

  # Generate curl command instead of executing
  astro api airflow /dags --generate

  # Use a specific Airflow version (skip auto-detection)
  astro api airflow --airflow-version 2.10.0 /dags`,
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestMethodPassed = cmd.Flags().Changed("method")
			opts.CredentialsExplicit = cmd.Flags().Changed("username") || cmd.Flags().Changed("password")
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runAirflowInteractive(opts)
			}
			opts.RequestPath = args[0]
			return runAirflow(opts)
		},
	}

	// Airflow-specific flags (persistent so they're inherited by subcommands)
	cmd.PersistentFlags().StringVar(&opts.APIURL, "api-url", "", "Override the Airflow API base URL (default: localhost:8080/api/v2)")
	cmd.PersistentFlags().StringVarP(&opts.DeploymentID, "deployment-id", "d", "", "Use Airflow URL from this Astro Cloud deployment")
	cmd.PersistentFlags().StringVarP(&opts.OrganizationID, "organization-id", "O", "", "Override organization ID for deployment lookup")
	cmd.PersistentFlags().StringVarP(&opts.WorkspaceID, "workspace-id", "W", "", "Override workspace ID for deployment lookup")
	cmd.PersistentFlags().StringVarP(&opts.Username, "username", "u", "admin", "Username for Airflow API authentication (local only)")
	cmd.PersistentFlags().StringVar(&opts.Password, "password", "admin", "Password for Airflow API authentication (local only)")
	cmd.PersistentFlags().StringVar(&opts.AirflowVersion, "airflow-version", "", "Override Airflow version for API spec (auto-detected by default)")

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
	cmd.Flags().DurationVar(&opts.CacheTTL, "cache", 0, "Cache the response, e.g. \"3600s\", \"60m\", \"1h\"")

	// Other flags
	cmd.Flags().BoolVar(&opts.GenerateCurl, "generate", false, "Output a curl command instead of executing the request")

	// Add list and describe subcommands
	// Note: These need to create their own spec cache since version is determined at runtime
	cmd.AddCommand(NewAirflowListCmd(out, opts))
	cmd.AddCommand(NewAirflowDescribeCmd(out, opts))

	return cmd
}

// runAirflow executes the airflow API request.
func runAirflow(opts *AirflowOptions) error {
	// Resolve the API base URL
	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	if err != nil {
		return err
	}

	// Initialize the spec cache if needed (for operation ID resolution).
	// Preserve the original baseURL in case init fails — initAirflowSpecCache
	// returns "" on error and we still need the URL to make raw-path requests.
	if correctedURL, err := initAirflowSpecCache(opts, baseURL, authToken); err != nil {
		// Only fail if we need the spec (for operation ID resolution)
		if isOperationID(opts.RequestPath) {
			return err
		}
		// Otherwise, continue without the spec — baseURL keeps its original value.
	} else {
		baseURL = correctedURL
	}

	// Resolve operation ID to path if needed
	requestPath := opts.RequestPath
	method := opts.RequestMethod
	methodFromSpec := false

	if isOperationID(requestPath) {
		endpoint, err := resolveOperationID(opts.specCache, requestPath, "airflow")
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
		method = http.MethodPost
	}

	// Build the full URL
	url := buildURL(baseURL, requestPath)

	// Generate curl command if requested
	if opts.GenerateCurl {
		return generateCurl(opts.Out, method, url, authToken, opts.RequestHeaders, params, opts.RequestInputFile)
	}

	// Build and execute the request
	err = executeRequest(&opts.RequestOptions, method, url, authToken, params)
	if isConnectionError(err) {
		return airflowConnectionError(url)
	}
	return err
}

// resolveAirflowAPIURL determines the Airflow API base URL based on options.
func resolveAirflowAPIURL(opts *AirflowOptions) (baseURL, authToken string, err error) {
	// If deployment ID is provided, fetch the deployment's Airflow URL
	if opts.DeploymentID != "" {
		return resolveDeploymentAirflowURL(opts)
	}

	// Determine base URL
	if opts.APIURL != "" {
		baseURL = opts.APIURL
	} else {
		baseURL = airflowAPIDefaultURL
	}

	// Check if user already provided an Authorization header
	for _, h := range opts.RequestHeaders {
		if strings.HasPrefix(strings.ToLower(h), "authorization:") {
			// User provided their own auth, don't fetch token
			return baseURL, "", nil
		}
	}

	// Fetch token from Airflow's auth endpoint
	token, err := fetchAirflowToken(opts.GetHTTPClient(), baseURL, opts.Username, opts.Password)
	if err != nil {
		// If the user explicitly passed credentials, treat failure as an error.
		if opts.CredentialsExplicit {
			return "", "", fmt.Errorf("authentication failed: %w", err)
		}
		// Suppress the warning for connection errors — the actual request will
		// report a clear, actionable error if the host is truly unreachable.
		if !isConnectionError(err) {
			fmt.Fprintf(opts.GetErrOut(), "Warning: could not fetch auth token (%v), continuing without authentication\n", err)
		}
		return baseURL, "", nil
	}

	return baseURL, token, nil
}

// fetchAirflowToken retrieves an auth token from a local Airflow instance.
// It tries the Airflow 3.x /auth/token endpoint first, then falls back to
// basic auth encoding for Airflow 2.x.
func fetchAirflowToken(client *http.Client, baseURL, username, password string) (string, error) {
	root := airflowHostRoot(baseURL)
	tokenURL := root + "/auth/token"

	// Create request body
	reqBody := map[string]string{
		"username": username,
		"password": password,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling auth request: %w", err)
	}

	// Make request with timeout context
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching auth token: %w", err)
	}
	defer resp.Body.Close()

	// If the /auth/token endpoint doesn't exist (Airflow 2.x), fall back to basic auth
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return "Basic " + basicAuth(username, password), nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("auth endpoint returned status %d", resp.StatusCode)
	}

	// Parse response
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding auth response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access_token in auth response")
	}

	return "Bearer " + tokenResp.AccessToken, nil
}

// basicAuth returns the base64-encoded "username:password" string.
func basicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// airflowHostRoot strips any /api/v1 or /api/v2 suffix to get the bare host URL.
func airflowHostRoot(baseURL string) string {
	u := strings.TrimSuffix(baseURL, "/")
	u = strings.TrimSuffix(u, "/api/v2")
	u = strings.TrimSuffix(u, "/api/v1")
	return u
}

// airflowConnectionError returns a user-friendly error when Airflow is unreachable.
func airflowConnectionError(requestURL string) error {
	host := airflowHostRoot(requestURL)
	if isLocalhostURL(host) {
		return fmt.Errorf("could not connect to Airflow at %s\n\n"+
			"Is Airflow running? Try one of:\n"+
			"  astro dev start              Start a local Airflow environment\n"+
			"  --api-url <url>              Use a different Airflow instance\n"+
			"  --deployment-id <id>         Connect to an Astro Cloud deployment", host)
	}
	return fmt.Errorf("could not connect to Airflow at %s\n\n"+
		"Check that the URL is correct and the server is running", host)
}

// FetchAirflowVersion detects the Airflow version from a running instance.
// It tries /api/v2/version first (Airflow 3.x) then /api/v1/version (Airflow 2.x).
// If authToken is provided, it will be included in the Authorization header.
func FetchAirflowVersion(client *http.Client, baseURL, authToken string) (string, error) {
	root := airflowHostRoot(baseURL)

	// Try v2 first (Airflow 3.x), then v1 (Airflow 2.x)
	versionURLs := []string{
		root + "/api/v2/version",
		root + "/api/v1/version",
	}

	var lastErr error
	for _, versionURL := range versionURLs {
		version, err := fetchVersionFromURL(client, versionURL, authToken)
		if err != nil {
			lastErr = err
			continue
		}
		return version, nil
	}

	return "", fmt.Errorf("could not detect version from any endpoint: %w", lastErr)
}

// fetchVersionFromURL tries to fetch the Airflow version from a single URL.
func fetchVersionFromURL(client *http.Client, versionURL, authToken string) (string, error) {
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating version request: %w", err)
	}

	// Add auth header if token provided (token may already include "Bearer " prefix)
	if authToken != "" {
		req.Header.Set("Authorization", authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching Airflow version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version endpoint returned status %d", resp.StatusCode)
	}

	var versionResp struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versionResp); err != nil {
		return "", fmt.Errorf("decoding version response: %w", err)
	}

	if versionResp.Version == "" {
		return "", fmt.Errorf("no version in response")
	}

	return versionResp.Version, nil
}

// apiPrefixForVersion returns "/api/v1" for Airflow 2.x, "/api/v2" for 3.x+.
func apiPrefixForVersion(version string) string {
	normalized := openapi.NormalizeAirflowVersion(version)
	if strings.HasPrefix(normalized, "2.") {
		return "/api/v1"
	}
	return "/api/v2"
}

// resolveDeploymentAirflowURL fetches the Airflow API URL from an Astro Cloud deployment.
func resolveDeploymentAirflowURL(opts *AirflowOptions) (baseURL, authToken string, err error) {
	// Check if we're in a cloud context
	if !context.IsCloudContext() {
		return "", "", fmt.Errorf("--deployment-id requires cloud context. Run 'astro login' to connect to Astro Cloud")
	}

	// Get current context for auth
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return "", "", fmt.Errorf("getting current context: %w", err)
	}

	// Check for token
	if ctx.Token == "" {
		return "", "", fmt.Errorf("not authenticated. Run 'astro login' to authenticate")
	}

	// Use organization from flag or context
	orgID := opts.OrganizationID
	if orgID == "" {
		orgID = ctx.Organization
	}
	if orgID == "" {
		return "", "", fmt.Errorf("organization ID not set. Use --organization-id or run 'astro organization switch'")
	}

	// Create platform client
	platformCoreClient := astroplatformcore.NewPlatformCoreClient(httputil.NewHTTPClient())

	// Fetch deployment
	dep, err := deployment.CoreGetDeployment(orgID, opts.DeploymentID, platformCoreClient)
	if err != nil {
		return "", "", fmt.Errorf("fetching deployment: %w", err)
	}

	// Get the Airflow API URL
	if dep.WebServerAirflowApiUrl == "" {
		return "", "", fmt.Errorf("deployment %s does not have an Airflow API URL configured", opts.DeploymentID)
	}

	// Ensure URL has a scheme
	airflowURL := dep.WebServerAirflowApiUrl
	if !strings.HasPrefix(airflowURL, "http://") && !strings.HasPrefix(airflowURL, "https://") {
		airflowURL = "https://" + airflowURL
	}

	return airflowURL, ctx.Token, nil
}

// runAirflowInteractive runs the airflow API command in interactive mode.
func runAirflowInteractive(opts *AirflowOptions) error {
	// Resolve the API base URL (handles deployment ID if provided)
	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	if err != nil {
		return err
	}

	// Initialize the spec cache
	if _, err = initAirflowSpecCache(opts, baseURL, authToken); err != nil {
		return err
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
	fmt.Fprintf(opts.Out, "\nFound %d endpoints. Use '%s' to list them.\n", len(endpoints), ansi.Bold("astro api airflow ls"))
	fmt.Fprintf(opts.Out, "Run '%s' to make a request.\n\n", ansi.Bold("astro api airflow <endpoint>"))

	return nil
}

// initAirflowSpecCache initializes the spec cache for the given Airflow instance.
// It detects the Airflow version (unless overridden) and creates the appropriate cache.
// It returns the (possibly corrected) base URL — e.g. /api/v2 → /api/v1 for Airflow 2.x.
func initAirflowSpecCache(opts *AirflowOptions, baseURL, authToken string) (string, error) {
	// Skip if already initialized
	if opts.specCache != nil {
		return baseURL, nil
	}

	// Determine the Airflow version
	version := opts.AirflowVersion
	if version == "" {
		// Try to detect version from the running instance
		detectedVersion, err := FetchAirflowVersion(opts.GetHTTPClient(), baseURL, authToken)
		if err != nil {
			// When targeting a remote deployment, detection failure is an error
			// since the instance should be reachable.
			if opts.DeploymentID != "" {
				return "", fmt.Errorf("could not detect Airflow version from deployment %s: %w. Use --airflow-version to specify manually", opts.DeploymentID, err)
			}
			// For local instances, warn and fall back — the instance may not be running.
			// Suppress the warning for connection errors since the actual request
			// will report a clear, actionable error.
			if !isConnectionError(err) {
				fmt.Fprintf(opts.RequestOptions.GetErrOut(), "Warning: Could not detect Airflow version (%v), using default %s. Use --airflow-version to override.\n", err, defaultAirflowVersion)
			}
			version = defaultAirflowVersion
		} else {
			version = detectedVersion
		}
	}

	// Correct the base URL to match the detected API version
	baseURL = airflowHostRoot(baseURL) + apiPrefixForVersion(version)

	// Create the spec cache for this version
	cache, err := openapi.NewAirflowCacheForVersion(version)
	if err != nil {
		return "", fmt.Errorf("creating spec cache for Airflow %s: %w", version, err)
	}

	cache.SetHTTPClient(opts.GetHTTPClient())
	opts.specCache = cache
	opts.detectedVersion = openapi.NormalizeAirflowVersion(version)
	return baseURL, nil
}

// NewAirflowListCmd creates the 'astro api airflow ls' command.
func NewAirflowListCmd(out io.Writer, parentOpts *AirflowOptions) *cobra.Command {
	var filter string
	var verbose bool
	var refresh bool

	cmd := &cobra.Command{
		Use:     "ls [filter]",
		Aliases: []string{"list"},
		Short:   "List available Airflow API endpoints",
		Long: `List all available endpoints from the Airflow API.

You can optionally provide a filter to search for specific endpoints.
The filter matches against endpoint paths, methods, operation IDs, summaries, and tags.`,
		Example: `  # List all endpoints
  astro api airflow ls

  # Filter endpoints
  astro api airflow ls dags

  # List POST endpoints
  astro api airflow ls POST

  # Show verbose output with descriptions
  astro api airflow ls --verbose`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				filter = args[0]
			}

			// Resolve the API base URL (handles deployment ID if provided)
			baseURL, authToken, err := resolveAirflowAPIURL(parentOpts)
			if err != nil {
				return err
			}

			// Initialize the spec cache
			if _, err := initAirflowSpecCache(parentOpts, baseURL, authToken); err != nil {
				return err
			}

			fmt.Fprintf(out, "Airflow version: %s\n\n", parentOpts.detectedVersion)

			// Create list options
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

// NewAirflowDescribeCmd creates the 'astro api airflow describe' command.
func NewAirflowDescribeCmd(out io.Writer, parentOpts *AirflowOptions) *cobra.Command {
	var method string
	var refresh bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "describe <endpoint>",
		Short: "Describe an Airflow API endpoint's request and response schema",
		Long: `Show detailed information about an Airflow API endpoint, including:
- Path and query parameters
- Request body schema (for POST/PUT/PATCH)
- Response schema

The endpoint can be specified as a path or as an operation ID.`,
		Example: `  # Describe an endpoint by path
  astro api airflow describe /dags/{dag_id}

  # Describe a POST endpoint specifically
  astro api airflow describe /dags/{dag_id} -X POST

  # Describe by operation ID
  astro api airflow describe get_dag`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve the API base URL (handles deployment ID if provided)
			baseURL, authToken, err := resolveAirflowAPIURL(parentOpts)
			if err != nil {
				return err
			}

			// Initialize the spec cache
			if _, err := initAirflowSpecCache(parentOpts, baseURL, authToken); err != nil {
				return err
			}

			// Create describe options
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
