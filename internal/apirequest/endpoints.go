package apirequest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/ghodss/yaml"

	"github.com/astronomer/astro-cli/pkg/openapi"
)

// untaggedSection heads the endpoints a spec gives no tag.
const untaggedSection = "Other"

// ListFilter settles the one filter `ls` takes, which can be spelled two ways:
// as the positional argument astro has always read, or as the --filter flag
// the standalone af spells it with. Both at once is fine when they agree.
func ListFilter(args []string, flag string) (string, error) {
	positional := ""
	if len(args) > 0 {
		positional = args[0]
	}
	if positional != "" && flag != "" && positional != flag {
		return "", fmt.Errorf("ls takes one filter: %q and --filter %q disagree", positional, flag)
	}
	if flag != "" {
		return flag, nil
	}
	return positional, nil
}

// EndpointRow is the machine-readable form of one `ls` row: a lean summary, as
// against `describe -o json`'s full resolved schema.
type EndpointRow struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	OperationID    string   `json:"operation_id,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Deprecated     bool     `json:"deprecated,omitempty"`
	PathParameters []string `json:"path_parameters,omitempty"`
}

// Rows turns endpoints into their machine-readable rows, always a non-nil
// slice so an empty listing encodes as [] rather than null.
func Rows(endpoints []openapi.Endpoint) []EndpointRow {
	rows := make([]EndpointRow, 0, len(endpoints))
	for i := range endpoints {
		ep := &endpoints[i]
		rows = append(rows, EndpointRow{
			Method:         ep.Method,
			Path:           ep.Path,
			OperationID:    ep.OperationID,
			Summary:        ep.Summary,
			Tags:           ep.Tags,
			Deprecated:     ep.Deprecated,
			PathParameters: openapi.GetPathParameters(ep.Path),
		})
	}
	return rows
}

// EndpointList is the `ls` listing, for `astro api … ls -o json` and `astro
// local api ls -o json` alike: the rows under "endpoints", [] when nothing
// matched, and their count. The key and the count are af's `api ls`.
type EndpointList struct {
	Endpoints []EndpointRow `json:"endpoints"`
	Count     int           `json:"count"`
}

// NewEndpointList wraps rows as the listing.
func NewEndpointList(rows []EndpointRow) EndpointList {
	return EndpointList{Endpoints: rows, Count: len(rows)}
}

// WriteEndpoints writes the human listing of already-filtered endpoints: a
// table grouped by tag, or every detail with verbose, then a count. filter is
// only for the message when nothing matched.
func WriteEndpoints(out io.Writer, endpoints []openapi.Endpoint, filter string, verbose bool) {
	if len(endpoints) == 0 {
		fmt.Fprintf(out, "No endpoints found matching '%s'\n", filter)
		return
	}
	if verbose {
		printEndpointsVerbose(out, endpoints)
	} else {
		printEndpointsTable(out, endpoints)
	}
	plural := "s"
	if len(endpoints) == 1 {
		plural = ""
	}
	fmt.Fprintf(out, "\nFound %d endpoint%s\n", len(endpoints), plural)
}

// errEmptySpec is a document with nothing in it to list.
var errEmptySpec = errors.New("no endpoints found in API specification")

// NonEmpty refuses a spec that lists no endpoints at all, which is a broken
// document rather than an empty answer, before any filter runs.
func NonEmpty(endpoints []openapi.Endpoint) error {
	if len(endpoints) == 0 {
		return errEmptySpec
	}
	return nil
}

// SpecJSON renders an OpenAPI document as indented JSON, whichever of JSON or
// YAML it was served as: Airflow 3 serves JSON, Airflow 2 YAML, and a script
// piping `spec` into jq should not have to know which it reached. A JSON
// document keeps its key order; a YAML one comes out with keys sorted.
func SpecJSON(raw []byte) ([]byte, error) {
	doc := raw
	if !json.Valid(raw) {
		converted, err := yaml.YAMLToJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("the API specification is neither JSON nor YAML: %w", err)
		}
		doc = converted
	}
	// YAML reads almost anything as a document — a login page is a string
	// scalar — so what came back has to be an object to be a spec at all.
	if trimmed := bytes.TrimSpace(doc); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("the API specification is not a JSON or YAML object")
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, doc, "", "  "); err != nil {
		return nil, fmt.Errorf("the API specification is not valid JSON: %w", err)
	}
	indented.WriteByte('\n')
	return indented.Bytes(), nil
}

// printEndpointsTable prints endpoints in a table format grouped by tags.
func printEndpointsTable(out io.Writer, endpoints []openapi.Endpoint) {
	groups := groupEndpointsByTag(endpoints)

	tagNames := make([]string, 0, len(groups))
	for tag := range groups {
		tagNames = append(tagNames, tag)
	}
	sort.Strings(tagNames)

	// Move "Other" to the end if present
	for i, tag := range tagNames {
		if tag == untaggedSection {
			tagNames = append(tagNames[:i], tagNames[i+1:]...)
			tagNames = append(tagNames, untaggedSection)
			break
		}
	}

	for i, tag := range tagNames {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "%s\n", color.New(color.Bold, color.FgWhite).Sprint(tag))

		// Use tabwriter for proper column alignment within each section
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		tagEndpoints := groups[tag]
		for j := range tagEndpoints {
			deprecated := ""
			if tagEndpoints[j].Deprecated {
				deprecated = " " + color.YellowString("(deprecated)")
			}
			// Fixed-width method column, so the ANSI codes do not skew it.
			fmt.Fprintf(w, "  %-8s\t%s\t%s%s\n", ColorizeMethod(tagEndpoints[j].Method), tagEndpoints[j].Path, tagEndpoints[j].OperationID, deprecated)
		}
		w.Flush() //nolint:errcheck // best-effort flush
	}
}

// groupEndpointsByTag groups endpoints by their primary (first) tag.
func groupEndpointsByTag(endpoints []openapi.Endpoint) map[string][]openapi.Endpoint {
	groups := make(map[string][]openapi.Endpoint)
	for i := range endpoints {
		tag := untaggedSection
		if len(endpoints[i].Tags) > 0 {
			tag = endpoints[i].Tags[0]
		}
		groups[tag] = append(groups[tag], endpoints[i])
	}
	return groups
}

// printEndpointsVerbose prints endpoints with full details.
func printEndpointsVerbose(out io.Writer, endpoints []openapi.Endpoint) {
	for i := range endpoints {
		if i > 0 {
			fmt.Fprintln(out, "---")
		}
		fmt.Fprintf(out, "%s %s\n", ColorizeMethod(endpoints[i].Method), endpoints[i].Path)
		if endpoints[i].OperationID != "" {
			fmt.Fprintf(out, "  Operation ID: %s\n", endpoints[i].OperationID)
		}
		if endpoints[i].Summary != "" {
			fmt.Fprintf(out, "  Summary: %s\n", endpoints[i].Summary)
		}
		if len(endpoints[i].Tags) > 0 {
			fmt.Fprintf(out, "  Tags: %s\n", strings.Join(endpoints[i].Tags, ", "))
		}
		if endpoints[i].Deprecated {
			fmt.Fprintf(out, "  %s\n", color.YellowString("DEPRECATED"))
		}
		if pathParams := openapi.GetPathParameters(endpoints[i].Path); len(pathParams) > 0 {
			fmt.Fprintf(out, "  Path Parameters: %s\n", strings.Join(pathParams, ", "))
		}
	}
}

// ColorizeMethod returns the method with appropriate coloring.
func ColorizeMethod(method string) string {
	switch method {
	case http.MethodGet:
		return color.GreenString(method)
	case http.MethodPost:
		return color.BlueString(method)
	case http.MethodPut:
		return color.YellowString(method)
	case http.MethodPatch:
		return color.CyanString(method)
	case http.MethodDelete:
		return color.RedString(method)
	default:
		return method
	}
}
