package api

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const untaggedSection = "Other"

// ListOptions holds options for the list command.
type ListOptions struct {
	Out       io.Writer
	specCache *openapi.Cache
	Filter    string
	Verbose   bool
	Refresh   bool
}

// NewListCmd creates the 'astro api <cmdName> ls' command.
// cmdName should be "cloud" or "airflow".
func NewListCmd(out io.Writer, specCache *openapi.Cache, cmdName string) *cobra.Command {
	opts := &ListOptions{
		Out:       out,
		specCache: specCache,
	}

	// Build API description based on command name
	apiDesc := "Astro Cloud API"
	filterExample := "deployments"
	if cmdName == "airflow" {
		apiDesc = "Airflow API"
		filterExample = "dags"
	}

	cmd := &cobra.Command{
		Use:     "ls [filter]",
		Aliases: []string{"list"},
		Short:   fmt.Sprintf("List available %s endpoints", apiDesc),
		Long: fmt.Sprintf(`List all available endpoints from the %s.

You can optionally provide a filter to search for specific endpoints.
The filter matches against endpoint paths, methods, operation IDs, summaries, and tags.`, apiDesc),
		Example: fmt.Sprintf(`  # List all endpoints
  astro api %s ls

  # Filter endpoints
  astro api %s ls %s

  # List POST endpoints
  astro api %s ls POST

  # Show verbose output with descriptions
  astro api %s ls --verbose`, cmdName, cmdName, filterExample, cmdName, cmdName),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Filter = args[0]
			}
			return runList(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Show additional details like summaries and tags")
	cmd.Flags().BoolVar(&opts.Refresh, "refresh", false, "Force refresh of the OpenAPI specification cache")

	return cmd
}

// runList executes the list command.
func runList(opts *ListOptions) error {
	// Load OpenAPI spec
	if err := opts.specCache.Load(opts.Refresh); err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	endpoints := opts.specCache.GetEndpoints()
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints found in API specification")
	}

	// Filter endpoints if a filter was provided
	if opts.Filter != "" {
		endpoints = openapi.FilterEndpoints(endpoints, opts.Filter)
		if len(endpoints) == 0 {
			fmt.Fprintf(opts.Out, "No endpoints found matching '%s'\n", opts.Filter)
			return nil
		}
	}

	// Print endpoints
	if opts.Verbose {
		printEndpointsVerbose(opts.Out, endpoints)
	} else {
		printEndpointsTable(opts.Out, endpoints)
	}

	plural := "s"
	if len(endpoints) == 1 {
		plural = ""
	}
	fmt.Fprintf(opts.Out, "\nFound %d endpoint%s\n", len(endpoints), plural)
	return nil
}

// printEndpointsTable prints endpoints in a table format grouped by tags.
func printEndpointsTable(out io.Writer, endpoints []openapi.Endpoint) {
	// Group endpoints by their primary tag
	groups := groupEndpointsByTag(endpoints)

	// Get sorted tag names
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

	// Print each group
	for i, tag := range tagNames {
		if i > 0 {
			fmt.Fprintln(out)
		}

		// Print section header
		fmt.Fprintf(out, "%s\n", color.New(color.Bold, color.FgWhite).Sprint(tag))

		// Use tabwriter for proper column alignment within each section
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

		tagEndpoints := groups[tag]
		for j := range tagEndpoints {
			deprecated := ""
			if tagEndpoints[j].Deprecated {
				deprecated = " " + color.YellowString("(deprecated)")
			}

			// Print with fixed-width method column to avoid ANSI code issues
			fmt.Fprintf(w, "  %-8s\t%s\t%s%s\n", colorizeMethod(tagEndpoints[j].Method), tagEndpoints[j].Path, tagEndpoints[j].OperationID, deprecated)
		}

		w.Flush()
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

		method := colorizeMethod(endpoints[i].Method)
		fmt.Fprintf(out, "%s %s\n", method, endpoints[i].Path)

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

		// Show path parameters
		pathParams := openapi.GetPathParameters(endpoints[i].Path)
		if len(pathParams) > 0 {
			fmt.Fprintf(out, "  Path Parameters: %s\n", strings.Join(pathParams, ", "))
		}
	}
}

// colorizeMethod returns the method with appropriate coloring.
func colorizeMethod(method string) string {
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
