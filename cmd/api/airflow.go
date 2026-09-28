package api

import (
	stdctx "context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/openapi"
)

const defaultAirflowVersion = "3.0.3" // Fallback version when detection fails

// AirflowOptions holds all options for the airflow api command.
type AirflowOptions struct {
	RequestOptions

	// Deployment is -d/--deployment: a deployment link the manifest declares,
	// or — for a name no link declares — an Astro Deployment id.
	Deployment string
	// URL is --url: an Airflow no project declares, addressed directly.
	URL string

	// APIURL and DeploymentID are the deprecated spellings of URL and
	// Deployment, kept working for one release.
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
func NewAirflowCmd(out io.Writer) *cobra.Command {
	opts := &AirflowOptions{
		RequestOptions: RequestOptions{
			Out:             out,
			ErrOut:          os.Stderr,
			TotalCountField: "total_entries",
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

A request needs a target; there is no default. Pass -d/--deployment to talk to
a deployment your project links in pyproject.toml — an Astro Deployment, an
MWAA or Composer environment, or a plain URL, each reached with the
credentials that deployment's link calls for. A name no link declares is read
as an Astro Deployment id. Pass --url to reach an Airflow no project declares.
For this project's local Airflow, use 'astro local api' instead.

The ls and describe subcommands only read the API spec, so they need no
target: without one they read the spec for --airflow-version, or for Airflow
` + defaultAirflowVersion + `.

The API generation is detected from the instance itself, so /dags reaches
/api/v2/dags on Airflow 3 and /api/v1/dags on Airflow 2. The version it reports
also picks the API specification; use --airflow-version to override it if the
instance is unreachable.

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

  # Get all DAGs from a deployment this project links in pyproject.toml
  astro api airflow -d prod /dags

  # Get a specific DAG by path
  astro api airflow -d prod /dags/example_dag

  # Use operation ID (path params supplied via -p)
  astro api airflow -d prod get_dag -p dag_id=example_dag

  # Pause a DAG via operation ID
  astro api airflow -d prod patch_dag -p dag_id=example_dag -F is_paused=true

  # Use jq filter on response
  astro api airflow -d prod /dags --jq '.dags[].dag_id'

  # Use an Airflow no project declares
  astro api airflow --url http://airflow.example.com:8080 /dags

  # Use an Astro Deployment by id
  astro api airflow -d clxyz123 /dags

  # Generate curl command instead of executing
  astro api airflow -d prod /dags --generate

  # List the endpoints of a specific Airflow version (no target needed)
  astro api airflow ls --airflow-version 2.10.0

  # This project's local Airflow
  astro local api /dags`,
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
	cmd.PersistentFlags().StringVarP(&opts.Deployment, "deployment", "d", "", "Deployment to act on, by the name the manifest links it under, or an Astro Deployment id")
	cmd.PersistentFlags().StringVar(&opts.URL, "url", "", "Airflow base URL to act on directly, for an Airflow no project declares")
	// The two spellings this command shipped with. They still work, under the
	// names above, for one release.
	cmd.PersistentFlags().StringVar(&opts.APIURL, "api-url", "", "Override the Airflow API base URL")
	cmd.PersistentFlags().StringVar(&opts.DeploymentID, "deployment-id", "", "Use Airflow URL from this Astro Cloud deployment")
	//nolint:errcheck // both flags are defined just above; this only errors on an unknown flag name
	cmd.PersistentFlags().MarkDeprecated("api-url", "use --url")
	//nolint:errcheck // see above
	cmd.PersistentFlags().MarkDeprecated("deployment-id", "use -d/--deployment")
	cmd.PersistentFlags().StringVarP(&opts.OrganizationID, "organization-id", "O", "", "Override organization ID for deployment lookup")
	cmd.PersistentFlags().StringVarP(&opts.WorkspaceID, "workspace-id", "W", "", "Override workspace ID for deployment lookup")
	cmd.PersistentFlags().StringVarP(&opts.Username, "username", "u", airflowrt.Airflow2AdminUser, "Username for Airflow API authentication (--url only)")
	cmd.PersistentFlags().StringVar(&opts.Password, "password", airflowrt.Airflow2AdminPassword, "Password for Airflow API authentication (--url only)")
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
	ctx := stdctx.Background()
	target, err := resolveAirflowTarget(ctx, opts)
	if err != nil {
		return err
	}

	// Work out which API generation to address, and load the spec that goes with
	// it. A failure here is a failure for every argument shape, not only for an
	// operation id: the generation decides the path, so carrying on would send
	// the request somewhere this run could not work out — and report success.
	//
	// The one survivor is the fallback initAirflowSpecCache makes for a bare
	// --url: it may simply not be running, so it warns and assumes the current
	// generation. That still yields a base with a prefix on it, never the bare
	// host root.
	baseURL, err := initAirflowSpecCache(ctx, opts, target)
	if err != nil {
		// An instance that never answered is unreachable, not mysterious. Say
		// that, rather than reporting the version probe that happened to be the
		// first thing to notice.
		if isConnectionError(err) && target.isHTTP() {
			return airflowConnectionError(target.hostRoot)
		}
		return err
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

	if !target.isHTTP() {
		return runAirflowThroughTransport(ctx, opts, target, method, requestPath, params)
	}

	// Build the full URL
	requestURL := buildURL(baseURL, requestPath)

	// Generate curl command if requested
	if opts.GenerateCurl {
		return generateCurl(opts.Out, opts.GetErrOut(), method, requestURL,
			withheldAuth(target.authorization, airflowTokenEnv), opts.RequestHeaders, params, opts.RequestInputFile)
	}

	// Build and execute the request
	err = executeRequest(&opts.RequestOptions, method, requestURL, target.authorization, params)
	if isConnectionError(err) {
		return airflowConnectionError(requestURL)
	}
	return err
}

// runAirflowThroughTransport sends the request through the deployment's own
// door, for a target with no Airflow URL to build a request against: MWAA under
// InvokeRestApi, which carries the call inside a signed AWS API request.
//
// What comes back is unwrapped faithfully — the doc's open question 1, settled.
// InvokeRestApi answers with the Airflow status in RestApiStatusCode and the
// Airflow body in RestApiResponse; the transport maps both onto an ordinary
// airflowapi.Response, and this prints exactly that. So `astro api airflow
// /dags -d prod-mwaa` gives the same status and the same body as it would
// against any HTTP Airflow, and a script branching on the exit code does not
// have to know which door it went through. The AWS envelope is machinery, not
// the answer.
//
// The flags that only make sense against a URL are refused rather than ignored:
// there is no curl command for a signed AWS call, and nothing to trace on the
// wire.
func runAirflowThroughTransport(ctx stdctx.Context, opts *AirflowOptions, target *airflowTarget, method, requestPath string, params map[string]interface{}) error {
	for _, unusable := range []struct {
		flag string
		set  bool
	}{
		{"--generate", opts.GenerateCurl},
		{"--paginate", opts.Paginate},
		{"--verbose", opts.Verbose},
		{"--include", opts.ShowResponseHeaders},
		{"--input", opts.RequestInputFile != ""},
		{"--header", len(opts.RequestHeaders) > 0},
	} {
		if unusable.set {
			return fmt.Errorf("%s needs an Airflow URL, and %s is reached through the AWS API instead", unusable.flag, target.name)
		}
	}

	req := airflowapi.Request{Method: strings.ToUpper(method), Path: requestPath}
	if len(params) > 0 {
		if strings.EqualFold(method, http.MethodGet) {
			req.Query = queryFromParams(params)
		} else {
			req.Body = params
		}
	}
	resp, err := target.client().Do(ctx, req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= httpStatusError {
		if len(resp.Body) > 0 {
			_ = writeColorizedJSON(opts.Out, resp.Body, isColorEnabled(opts.Out), "  ") //nolint:errcheck // the request already failed; a write error changes nothing
		}
		return &SilentError{StatusCode: resp.StatusCode}
	}
	return outputResponseBody(&opts.RequestOptions, resp.Body)
}

// queryFromParams turns the -f/-F fields into a query string, the same encoding
// addQueryParams applies to a URL.
func queryFromParams(params map[string]interface{}) url.Values {
	query := url.Values{}
	for key, value := range params {
		addQueryParam(query, key, value)
	}
	return query
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
			"Is Airflow running? For this project's local Airflow, try one of:\n"+
			"  astro local start            Start it\n"+
			"  astro local api <endpoint>   Reach it on whatever port it runs on", host)
	}
	return fmt.Errorf("could not connect to Airflow at %s\n\n"+
		"Check that the URL is correct and the server is running", host)
}

// apiPrefixForVersion returns "/api/v1" for Airflow 2.x, "/api/v2" for 3.x+.
func apiPrefixForVersion(version string) string {
	normalized := openapi.NormalizeAirflowVersion(version)
	if strings.HasPrefix(normalized, "2.") {
		return "/api/v1"
	}
	return "/api/v2"
}

// runAirflowInteractive runs the airflow API command in interactive mode.
func runAirflowInteractive(opts *AirflowOptions) error {
	if err := loadAirflowSpec(stdctx.Background(), opts); err != nil {
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
	fmt.Fprintf(opts.Out, "Run '%s' to make a request.\n\n", ansi.Bold("astro api airflow -d <deployment> <endpoint>"))

	return nil
}

// initAirflowSpecCache settles which API generation this target speaks and
// loads the spec that goes with it. It asks the instance what version it runs
// (unless --airflow-version says) and returns the base URL with the
// generation's prefix on it — /api/v1 for Airflow 2, /api/v2 for Airflow 3 —
// or "" for a target that is not addressed by URL at all.
//
// The asking is pkg/airflowapi's: one Client.Version call over the target's own
// door, which probes both generations and reads the generation off the version
// Airflow reports rather than off whichever probe answered. That is why an
// Astronomer-patched Airflow 2 — which answers some /api/v2 paths — still gets
// /api/v1 here.
//
// An error here ends the run, whatever the caller was going to ask for. The
// generation decides the path, so a run that could not work it out has nowhere
// to send the request — and a request sent to the wrong path that reports
// success is worse than a refusal. The one target that falls back instead is a
// bare --url: it may simply not be running, so it warns and assumes the current
// generation, which still yields a prefixed base.
func initAirflowSpecCache(ctx stdctx.Context, opts *AirflowOptions, target *airflowTarget) (string, error) {
	// Skip if already initialized
	if opts.specCache != nil {
		return target.apiBase(opts.detectedVersion), nil
	}

	// Determine the Airflow version
	version := opts.AirflowVersion
	if version == "" {
		info, err := target.client().Version(ctx)
		switch {
		case err == nil:
			version = info.Version
		case target.isNamedDeployment():
			// A deployment resolves through a control plane that says it
			// exists, so failing to read its version is a failure rather than a
			// fallback.
			return "", fmt.Errorf("could not detect Airflow version from %s: %w. Use --airflow-version to specify manually", target.name, err)
		default:
			// A bare --url may simply not be running. Warn and fall back —
			// except on a connection failure, which the request itself is
			// about to report far better.
			if !isConnectionError(err) {
				fmt.Fprintf(opts.RequestOptions.GetErrOut(), "Warning: Could not detect Airflow version (%v), using default %s. Use --airflow-version to override.\n", err, defaultAirflowVersion)
			}
			version = defaultAirflowVersion
		}
	}

	if err := opts.useSpecFor(version); err != nil {
		return "", err
	}
	return target.apiBase(version), nil
}

// useSpecFor sets up the spec cache for one Airflow version.
func (o *AirflowOptions) useSpecFor(version string) error {
	cache, err := openapi.NewAirflowCacheForVersion(version)
	if err != nil {
		return fmt.Errorf("creating spec cache for Airflow %s: %w", version, err)
	}
	cache.SetHTTPClient(o.GetHTTPClient())
	o.specCache = cache
	o.detectedVersion = openapi.NormalizeAirflowVersion(version)
	return nil
}

// loadAirflowSpec sets up the spec for the commands that only read it: ls,
// describe, and the bare command with no endpoint. A named target is asked for
// its version, as a request would. With none there is nothing to ask, so the
// spec is --airflow-version's, or the default's.
func loadAirflowSpec(ctx stdctx.Context, opts *AirflowOptions) error {
	if !opts.hasTarget() {
		return opts.useSpecFor(firstNonEmpty(opts.AirflowVersion, defaultAirflowVersion))
	}
	target, err := resolveAirflowTarget(ctx, opts)
	if err != nil {
		return err
	}
	_, err = initAirflowSpecCache(ctx, opts, target)
	return err
}

// NewAirflowListCmd creates the 'astro api airflow ls' command.
func NewAirflowListCmd(out io.Writer, parentOpts *AirflowOptions) *cobra.Command {
	var filter string
	var verbose bool
	var refresh bool
	var jsonOut bool

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

			if err := loadAirflowSpec(cmd.Context(), parentOpts); err != nil {
				return err
			}

			if !jsonOut {
				fmt.Fprintf(out, "Airflow version: %s\n\n", parentOpts.detectedVersion)
			}

			// Create list options
			listOpts := &ListOptions{
				Out:       out,
				specCache: parentOpts.specCache,
				Filter:    filter,
				Verbose:   verbose,
				Refresh:   refresh,
				JSON:      jsonOut,
			}

			return runList(listOpts)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show additional details like summaries and tags")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output the endpoint list as JSON")

	return cmd
}

// NewAirflowDescribeCmd creates the 'astro api airflow describe' command.
func NewAirflowDescribeCmd(out io.Writer, parentOpts *AirflowOptions) *cobra.Command {
	var method string
	var refresh bool
	var verbose bool
	var jsonOut bool

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
			if err := loadAirflowSpec(cmd.Context(), parentOpts); err != nil {
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
				JSON:      jsonOut,
			}

			return runDescribe(descOpts)
		},
	}

	cmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method (GET, POST, PUT, PATCH, DELETE)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show spec URL and additional details")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output the endpoint schema as JSON")

	return cmd
}
