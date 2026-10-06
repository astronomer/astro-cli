// Package cliout is the output contract every command in the CLI shares:
// the --output flag and its formats, the single door a payload leaves by
// (Renderer.Emit), and how a failure is reported — the json error object, its
// kind, and the process exit code.
//
// It lives under cmd/ because rendering and exiting are the cmd layer's job;
// nothing below cmd/ may import it. It knows no command family: cmd/local
// brings its own failure kinds, the root brings the cloud ones, and Execute
// takes the composed table.
package cliout

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// Format selects how a command renders its results.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// ParseFormat validates an --output flag value against text, json and the
// extras the command declared when it registered the flag. A value it does
// not know is a usage error: the command was invoked wrongly, and nothing ran.
//
// This is the one place --output is parsed. A command that renders through a
// package below cmd/ converts the result to that package's own format type at
// the call site (env.Format(f), output.Format(f)): those packages may not
// import cmd/, so they cannot take a Format, and their writers only render.
func ParseFormat(s string, extras ...Format) (Format, error) {
	f := Format(s)
	if f == FormatText || f == FormatJSON || slices.Contains(extras, f) {
		return f, nil
	}
	return "", Usage(fmt.Errorf("unknown output format %q (supported: %s)", s, strings.Join(formatNames(extras), ", ")))
}

// AddOutputFlag registers the shared --output flag on cmd's persistent flags,
// so one registration covers a whole command family. A command that offers a
// format beyond text and json for a special use (dotenv for `astro env
// variable list`, yaml for `astro deployment inspect`) names it as an extra,
// and passes the same extras to ParseFormat.
func AddOutputFlag(cmd *cobra.Command, target *string, extras ...Format) {
	names := formatNames(extras)
	usage := "Output format: " + strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
	cmd.PersistentFlags().StringVarP(target, "output", "o", string(FormatText), usage)
}

// formatNames lists text, json and the extras, in that order.
func formatNames(extras []Format) []string {
	names := []string{string(FormatText), string(FormatJSON)}
	for _, f := range extras {
		names = append(names, string(f))
	}
	return names
}

// Renderer writes command results. Emit is the single output path: json mode
// encodes the value as one line — so repeated calls on a streaming surface
// form NDJSON — and text mode runs the text renderer over the same value.
// Human output is a rendering of the same data, never a separate code path.
type Renderer struct {
	Format Format
	Out    io.Writer
}

// Emit writes v. In text mode it calls text, which must render v and
// nothing else.
//
// A nil text renderer is only legal on a streaming surface, where the
// caller has already established it is in json mode and the human rendering
// is a table written once at the end. Reaching text mode with nil is a
// programming error, and it panics rather than writing nothing.
//
// Deliberately not a silent no-op. An earlier version of this returned nil
// there, on the theory that it made the caller's `if r.Format == FormatJSON`
// branch unnecessary. It does not: delete that branch from emitRows, envList
// or renderCheck and a no-op turns `astro local list` into an empty table
// with exit 0 — a human sees nothing and is told nothing. The branch is
// load-bearing, and a panic is what says so when it goes missing.
func (r Renderer) Emit(v any, text func(w io.Writer) error) error {
	if EmitObserver != nil {
		EmitObserver(v)
	}
	if r.Format == FormatJSON {
		return json.NewEncoder(r.Out).Encode(v)
	}
	if text == nil {
		panic("Renderer.Emit: text mode with no text renderer — this value is " +
			"json-only, so the caller must not reach here in text mode. The " +
			"`if r.Format == FormatJSON` branch around a streaming Emit is what " +
			"prevents it.")
	}
	return text(r.Out)
}

// EmitObserver, when set, is handed every value Emit publishes. It is nil in
// production and is a test door, exported only because the tests that arm it
// live in other packages (cmd/local's TestMain): the schema goldens pin Go
// types, and nothing in them can tell you a command still emits the type its
// golden holds. Watching the door is the only way to know, and the door is
// only worth watching because everything goes through it — see
// TestEmitIsTheOnlyJSONEncoder in cmd/local.
//
// Nothing outside a test assigns it.
var EmitObserver func(any)
