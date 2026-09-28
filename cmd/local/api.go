package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
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
// The deployment-side equivalent is `astro api airflow`, which loads the
// OpenAPI spec so it can resolve operation ids and describe endpoints. This one
// deliberately does not: a raw path against the Airflow on your own machine
// should work with no spec to download and no network beyond localhost.
func newAPICmd(c *cli) *cobra.Command {
	var opts apiOptions
	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make one request to this machine's Airflow API",
		Long: "Send a request to the Airflow running on this machine and print what it sent back, unchanged.\n\n" +
			"The endpoint is a path relative to the API base, so `/dags` reaches /api/v2/dags on Airflow 3 and " +
			"/api/v1/dags on Airflow 2 — the generation is detected, not assumed. A query string typed onto the " +
			"path is sent as one. --root addresses the server below the version prefix, for the few paths Airflow " +
			"serves unversioned.\n\n" +
			"The body is printed whatever the status, and a non-2xx status also fails the command, so a script can " +
			"branch on the exit code without parsing anything. Output is Airflow's own, so --output governs how a " +
			"failure is reported rather than how the response is rendered.\n\n" +
			"For a deployment, `astro api airflow` is the same idea with the API spec behind it.",
		Example: "  # Every DAG, as Airflow itself reports them\n" +
			"  astro local api /dags\n\n" +
			"  # The configuration this Airflow runs with\n" +
			"  astro local api /config\n\n" +
			"  # Trigger a run\n" +
			"  astro local api /dags/etl/dagRuns -X POST --body '{\"logical_date\": null}'\n\n" +
			"  # The same, with the body in a file\n" +
			"  astro local api /dags/etl/dagRuns -X POST --input run.json\n\n" +
			"  # A path below the version prefix\n" +
			"  astro local api --root /health",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runAPI(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().StringVarP(&opts.method, "method", "X", http.MethodGet, "HTTP method")
	cmd.Flags().StringVar(&opts.body, "body", "", "JSON request body, or - to read it from stdin")
	cmd.Flags().StringVar(&opts.input, "input", "", "File holding the JSON request body, or - to read it from stdin")
	cmd.MarkFlagsMutuallyExclusive("body", "input")
	cmd.Flags().BoolVar(&opts.root, "root", false, "Address the server root, below the API version prefix")
	return cmd
}

// apiOptions is what one passthrough request takes beyond its path.
type apiOptions struct {
	method string
	body   string
	input  string
	root   bool
}

func (c *cli) runAPI(ctx context.Context, endpoint string, opts apiOptions) error {
	// The format is validated even though the response is printed as it came: a
	// misspelled --output should fail here rather than quietly change how the
	// failure below is reported.
	r, err := c.renderer()
	if err != nil {
		return err
	}
	body, err := opts.payload(c.d.Stdin)
	if err != nil {
		return err
	}
	path, query, err := splitQuery(endpoint)
	if err != nil {
		return err
	}
	client, err := c.machineClient(ctx)
	if err != nil {
		return err
	}
	req := airflowapi.Request{
		Method: strings.ToUpper(opts.method),
		Path:   path,
		Query:  query,
		Body:   body,
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
	if err := writeBody(r.Out, resp.Body); err != nil {
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
