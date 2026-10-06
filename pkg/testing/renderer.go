package testing

import (
	"encoding/json"
	"io"
)

// Renderer stands in for cmd/cliout.Renderer in the tests of the renderers
// below cmd/ (pkg/output, internal/platform/astro/env and the lists built on
// them), which may not import cmd/. Those renderers hand a result to the
// command's Renderer and render only its text; this is what a test hands them
// instead.
//
// It encodes json the way cliout does off a terminal — compact, one line, no
// HTML escaping — but it is not that encoder, so a test of how the CLI lays
// json out belongs in cmd/cliout. What a test here can check is which value
// was published and what the text says.
type Renderer struct {
	// JSON selects json mode; otherwise the text renderer runs.
	JSON bool
	Out  io.Writer
}

// Emit publishes v in json mode, or runs text in text mode.
func (r Renderer) Emit(v any, text func(io.Writer) error) error {
	if r.JSON {
		enc := json.NewEncoder(r.Out)
		enc.SetEscapeHTML(false)
		return enc.Encode(v)
	}
	return text(r.Out)
}
