package api

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/apirequest"
	"github.com/astronomer/astro-cli/pkg/openapi"
)

// ListOptions holds options for the list command.
type ListOptions struct {
	Out       io.Writer
	specCache *openapi.Cache
	Filter    string
	Verbose   bool
	Refresh   bool
	Format    cliout.Format
}

// runList executes the list command. The rendering is apirequest's, shared
// with `astro local api ls`; what is this command's own is where the spec
// comes from.
func runList(opts *ListOptions) error {
	if opts.specCache == nil {
		return fmt.Errorf("API specification not initialized. Ensure you are logged in and try again")
	}

	if opts.Verbose && opts.Format != cliout.FormatJSON {
		fmt.Fprintf(opts.Out, "Spec URL: %s\n\n", opts.specCache.GetSpecURL())
	}

	if err := opts.specCache.Load(opts.Refresh); err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	endpoints := opts.specCache.GetEndpoints()
	if err := apirequest.NonEmpty(endpoints); err != nil {
		return err
	}
	endpoints = openapi.FilterEndpoints(endpoints, opts.Filter)

	// The (possibly empty) list goes under "endpoints". The rows are built
	// only under json: text draws from the endpoints themselves.
	r := cliout.Renderer{Format: opts.Format, Out: opts.Out}
	return r.Emit(cliout.Lazy(func() any { return apirequest.NewEndpointList(apirequest.Rows(endpoints)) }),
		func(w io.Writer) error {
			apirequest.WriteEndpoints(w, endpoints, opts.Filter, opts.Verbose)
			return nil
		})
}
