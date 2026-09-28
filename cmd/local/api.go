package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/internal/apirequest"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/openapi"
)

// newAPICmd builds `astro local api`: one request to the Airflow running on
// this machine, printed as it came back.
//
// It is the escape hatch under the typed commands — every endpoint Airflow
// serves, including the ones this surface has no verb for — and it is the raw
// path of the same client, over the same transport, with the same minted
// credentials. There is nothing to configure and nothing to log into: the
// engine started this Airflow, so the CLI already knows how to talk to it.
//
// What goes after the endpoint is the standalone af's `af api`, flag for flag,
// because that is what the skills and every af user already type: -F and -f
// fields, -H headers, -i, `ls --filter`, `spec`. The parsing and rendering
// are apirequest's, the same code `astro api airflow` runs, so the two raw-API
// commands cannot drift apart on what a flag means.
//
// The endpoint listing comes from the Airflow itself — its own /openapi.json,
// or /api/v1/openapi.yaml on Airflow 2 — rather than from a published spec, so
// `ls` and `spec` need no network beyond localhost and describe exactly the
// Airflow that is running, plugins and providers included. The deployment-side
// equivalent is `astro api airflow`, which works from the published spec for
// the version it detects so it can also resolve operation ids.
func newAPICmd(c *cli) *cobra.Command {
	var opts apiOptions
	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make one request to this machine's Airflow API",
		Long: "Send a request to the Airflow running on this machine and print what it sent back, unchanged.\n\n" +
			"The endpoint is a path relative to the API base, so `/dags` (or `dags`) reaches /api/v2/dags on " +
			"Airflow 3 and /api/v1/dags on Airflow 2 — the generation is detected, not assumed. A query string " +
			"typed onto the path is sent as one. --root (or --raw) addresses the server below the version prefix, " +
			"for the few paths Airflow serves unversioned.\n\n" +
			"-F/--field adds a typed field: true, false and null are themselves, a number is a number, and @file " +
			"reads a file (@- reads stdin). --raw-field adds a field that is always a string (af spells it -f, which is --follow elsewhere in this tree). key[sub]=v nests " +
			"an object and key[]=v builds an array. Fields go in the query string on a GET, or when --body or --input carries " +
			"the payload, and become the JSON body otherwise. The method stays GET unless -X says otherwise.\n\n" +
			"The body is printed whatever the status, and a non-2xx status also fails the command, so a script can " +
			"branch on the exit code without parsing anything. -i puts the status line and headers in front of it; " +
			"with --output json it prints one {status_code, headers, body} object instead. Otherwise the output is " +
			"Airflow's own, and --output governs how a failure is reported.\n\n" +
			"`ls` lists the endpoints this Airflow serves and `spec` prints its OpenAPI document, both read from " +
			"the Airflow itself. For a deployment, `astro api airflow` is the same idea with the published API spec " +
			"behind it.",
		Example: "  # Every DAG, as Airflow itself reports them\n" +
			"  astro local api /dags\n\n" +
			"  # Typed fields become the query string on a GET\n" +
			"  astro local api dags -F limit=10 -F only_active=true\n\n" +
			"  # Create a variable: fields become the body on a POST\n" +
			"  astro local api variables -X POST -F key=my_var --raw-field value=8080\n\n" +
			"  # Trigger a run with a whole body\n" +
			"  astro local api /dags/etl/dagRuns -X POST --body '{\"logical_date\": null}'\n\n" +
			"  # The same, with the body in a file\n" +
			"  astro local api /dags/etl/dagRuns -X POST --input run.json\n\n" +
			"  # The status line and headers too\n" +
			"  astro local api dags -i\n\n" +
			"  # A path below the version prefix\n" +
			"  astro local api --root /health\n\n" +
			"  # Which endpoints mention variables, and the whole spec\n" +
			"  astro local api ls --filter variable\n" +
			"  astro local api spec | jq '.paths | keys'",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runAPI(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().StringVarP(&opts.method, "method", "X", http.MethodGet, "HTTP method")
	cmd.Flags().StringVar(&opts.body, "body", "", "JSON request body, or - to read it from stdin")
	cmd.Flags().StringVar(&opts.input, "input", "", "File holding the JSON request body, or - to read it from stdin")
	cmd.MarkFlagsMutuallyExclusive("body", "input")
	cmd.Flags().BoolVar(&opts.root, "root", false, "Address the server root, below the API version prefix (af spells it --raw)")
	cmd.Flags().StringArrayVarP(&opts.fields, "field", "F", nil, "Add a typed field in key=value format: numbers, true, false, null, @file")
	// No -f: across the v2 tree -f is --follow (`astro local logs -f`), and one
	// shorthand means one thing there (TestShorthandsMeanOneThingEachAcrossTheV2Tree).
	// af's `-f key=value` is spelled out as --raw-field here.
	cmd.Flags().StringArrayVar(&opts.rawFields, "raw-field", nil, "Add a string field in key=value format")
	cmd.Flags().StringArrayVarP(&opts.headers, "header", "H", nil, "Add a request header in key:value format")
	cmd.Flags().BoolVarP(&opts.include, "include", "i", false, "Include the response status line and headers in the output")
	// af's spelling of --root, taken as a synonym rather than a second flag so
	// the two can never disagree.
	cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "raw" {
			name = "root"
		}
		return pflag.NormalizedName(name)
	})
	cmd.AddCommand(newAPIListCmd(c), newAPISpecCmd(c))
	return cmd
}

// apiOptions is what one passthrough request takes beyond its path.
type apiOptions struct {
	method    string
	body      string
	input     string
	root      bool
	fields    []string
	rawFields []string
	headers   []string
	include   bool
}

func (c *cli) runAPI(ctx context.Context, endpoint string, opts apiOptions) error {
	// The format is validated even though the response is printed as it came: a
	// misspelled --output should fail here rather than quietly change how the
	// failure below is reported.
	r, err := c.renderer()
	if err != nil {
		return err
	}
	req, err := opts.request(endpoint, c.d.Stdin)
	if err != nil {
		return err
	}
	client, err := c.machineClient(ctx)
	if err != nil {
		return err
	}
	// Do hangs the path under the generation it detected; DoRoot goes below the
	// prefix. Both hand back whatever status came, which is what a passthrough
	// is for.
	send := client.Do
	if opts.root {
		send = client.DoRoot
	}
	resp, err := send(ctx, req)
	if err != nil {
		return err
	}
	if opts.include {
		err = writeExchange(r, resp)
	} else {
		err = writeBody(r.Out, resp.Body)
	}
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The client's own error type, so the wording matches every typed
		// command and errors.Is still answers for 404, 401, and 403. Body stays
		// empty: it is already on stdout, and repeating it here would print
		// Airflow's detail twice.
		return &airflowapi.StatusError{Method: req.Method, Path: endpoint, StatusCode: resp.StatusCode}
	}
	return nil
}

// request builds the one call the flags describe.
func (o apiOptions) request(endpoint string, stdin io.Reader) (airflowapi.Request, error) {
	body, err := o.payload(stdin)
	if err != nil {
		return airflowapi.Request{}, err
	}
	params, err := apirequest.Fields{Magic: o.fields, Raw: o.rawFields, Numbers: apirequest.AnyNumber, Stdin: stdin}.Parse()
	if err != nil {
		return airflowapi.Request{}, fmt.Errorf("parsing fields: %w", err)
	}
	header, err := apirequest.ParseHeaders(o.headers)
	if err != nil {
		return airflowapi.Request{}, err
	}
	path, query, err := splitQuery(endpoint)
	if err != nil {
		return airflowapi.Request{}, err
	}
	req := airflowapi.Request{
		Method: strings.ToUpper(o.method),
		Path:   path,
		Query:  query,
		Body:   body,
		Header: header,
	}
	placeFields(&req, params)
	return req, nil
}

// placeFields puts the -F/-f fields where af puts them: in the query string on
// a GET, and as the JSON body on anything else. A request whose body --body
// already supplied takes them in the query too, the rule `astro api airflow`
// has for --input, rather than dropping them the way af does.
func placeFields(req *airflowapi.Request, params map[string]any) {
	if len(params) == 0 {
		return
	}
	if req.Method != http.MethodGet && req.Body == nil {
		req.Body = params
		return
	}
	if req.Query == nil {
		req.Query = url.Values{}
	}
	for key, values := range apirequest.Query(params) {
		req.Query[key] = append(req.Query[key], values...)
	}
}

// apiExchange is `-i --output json`: the answer as one object, in the shape the
// standalone af prints for `af api -i` — the status, the headers keyed in lower
// case with repeated values joined, and the body decoded when it is JSON.
type apiExchange struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	// Body is the decoded JSON body, the text of any other, or null for none.
	Body any `json:"body"`
}

// writeExchange prints the answer with its status and headers: the block an
// HTTP response starts with, then the body, or the one object json mode takes.
func writeExchange(r Renderer, resp airflowapi.Response) error {
	exchange := apiExchange{StatusCode: resp.StatusCode, Headers: map[string]string{}}
	for key, values := range resp.Header {
		exchange.Headers[strings.ToLower(key)] = strings.Join(values, ", ")
	}
	if body := bytes.TrimSpace(resp.Body); len(body) > 0 {
		if json.Valid(body) {
			exchange.Body = json.RawMessage(body)
		} else {
			exchange.Body = string(resp.Body)
		}
	}
	return r.Emit(exchange, func(w io.Writer) error {
		status := fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		apirequest.WriteHead(w, strings.TrimSpace(resp.Proto+" "+status), resp.Header, false)
		return writeBody(w, resp.Body)
	})
}

// newAPIListCmd builds `astro local api ls`: the endpoints this Airflow serves,
// from its own spec.
func newAPIListCmd(c *cli) *cobra.Command {
	var filterFlag string
	var verbose bool
	cmd := &cobra.Command{
		Use:     "ls [filter]",
		Aliases: []string{"list"},
		Short:   "List the endpoints this machine's Airflow serves",
		Long: "List the API endpoints the Airflow running on this machine serves, read from its own OpenAPI " +
			"specification.\n\n" +
			"A filter, as an argument or with --filter, keeps the endpoints whose path, method, operation id, " +
			"summary or tag contains it, ignoring case. Paths are shown relative to the API base, the form " +
			"`astro local api` takes them in. --output json prints one object per endpoint.",
		Example: "  astro local api ls\n" +
			"  astro local api ls --filter variable\n" +
			"  astro local api ls dagRun --verbose",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := apirequest.ListFilter(args, filterFlag)
			if err != nil {
				return err
			}
			r, err := c.renderer()
			if err != nil {
				return err
			}
			endpoints, err := c.machineEndpoints(cmd.Context())
			if err != nil {
				return err
			}
			endpoints = openapi.FilterEndpoints(endpoints, filter)
			return emitRows(r, apirequest.Rows(endpoints), func(w io.Writer, _ []apirequest.EndpointRow) error {
				apirequest.WriteEndpoints(w, endpoints, filter, verbose)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&filterFlag, "filter", "", "Only list endpoints matching this, the same as the positional filter")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show each endpoint's operation id, summary, tags and path parameters")
	return cmd
}

// newAPISpecCmd builds `astro local api spec`: this Airflow's OpenAPI
// document, as JSON.
func newAPISpecCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:   "spec",
		Short: "Print the OpenAPI specification this machine's Airflow serves",
		Long: "Print the OpenAPI specification the Airflow running on this machine serves, as JSON.\n\n" +
			"Airflow 3 serves it as JSON at /openapi.json; Airflow 2 serves YAML at /api/v1/openapi.yaml, which " +
			"is printed as JSON all the same, so a script piping it into jq does not have to know which it reached.",
		Example: "  astro local api spec\n" +
			"  astro local api spec | jq '.paths | keys'",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := c.renderer()
			if err != nil {
				return err
			}
			raw, _, err := c.machineSpec(cmd.Context())
			if err != nil {
				return err
			}
			doc, err := apirequest.SpecJSON(raw)
			if err != nil {
				return err
			}
			_, err = r.Out.Write(doc)
			return err
		},
	}
}

// machineEndpoints is what `ls` lists: the running Airflow's own spec, with
// its version prefix taken off every path so each reads as `astro local api`
// takes it.
func (c *cli) machineEndpoints(ctx context.Context) ([]openapi.Endpoint, error) {
	raw, generation, err := c.machineSpec(ctx)
	if err != nil {
		return nil, err
	}
	endpoints, err := openapi.ParseEndpoints(raw, generation.BasePath())
	if err != nil {
		return nil, err
	}
	if err := apirequest.NonEmpty(endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

// machineSpec fetches the OpenAPI document the running Airflow serves, from
// wherever its generation serves it: /openapi.json at the root on Airflow 3,
// /api/v1/openapi.yaml on Airflow 2. Those are the two addresses af reads.
func (c *cli) machineSpec(ctx context.Context) ([]byte, airflowapi.Generation, error) {
	client, err := c.machineClient(ctx)
	if err != nil {
		return nil, airflowapi.GenerationNone, err
	}
	generation, err := client.Generation(ctx)
	if err != nil {
		return nil, airflowapi.GenerationNone, err
	}
	var resp airflowapi.Response
	req := airflowapi.Request{Method: http.MethodGet}
	if generation == airflowapi.Airflow2 {
		req.Path = "/openapi.yaml"
		resp, err = client.Do(ctx, req)
	} else {
		req.Path = "/openapi.json"
		resp, err = client.DoRoot(ctx, req)
	}
	if err != nil {
		return nil, airflowapi.GenerationNone, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, airflowapi.GenerationNone, fmt.Errorf("reading this Airflow's API specification: %w",
			&airflowapi.StatusError{Method: req.Method, Path: req.Path, StatusCode: resp.StatusCode})
	}
	return resp.Body, generation, nil
}

// writeBody writes Airflow's answer as it arrived, ending it with the newline
// it may not carry. The bytes go straight out rather than through a string, so
// a large listing is not copied to add one character.
func writeBody(w io.Writer, body []byte) error {
	if _, err := w.Write(body); err != nil {
		return err
	}
	if len(body) == 0 || body[len(body)-1] == '\n' {
		return nil
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// payload is the request body: nothing at all unless one was given, because no
// body and an empty object are different requests.
func (o apiOptions) payload(stdin io.Reader) (any, error) {
	flag, raw := "--body", []byte(o.body)
	var err error
	switch {
	case o.input != "":
		flag = "--input"
		raw, err = readBody(o.input, stdin)
	case o.body == "-":
		raw, err = readBody(o.body, stdin)
	case o.body == "":
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s is not valid json", flag)
	}
	return json.RawMessage(raw), nil
}

// readBody reads a request body from a file, or from stdin for "-".
func readBody(source string, stdin io.Reader) ([]byte, error) {
	if source != "-" {
		read, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read the request body: %w", err)
		}
		return read, nil
	}
	read, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read the request body from stdin: %w", err)
	}
	return read, nil
}

// splitQuery separates a path from the query string typed onto it, so
// `/dags?limit=5` sends what it looks like it sends.
func splitQuery(endpoint string) (path string, query url.Values, err error) {
	path, raw, found := strings.Cut(endpoint, "?")
	if !found {
		return path, nil, nil
	}
	query, err = url.ParseQuery(raw)
	if err != nil {
		return "", nil, fmt.Errorf("the query string on %q does not parse: %w", endpoint, err)
	}
	return path, query, nil
}
