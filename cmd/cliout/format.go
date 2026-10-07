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
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	jsoncolor "github.com/neilotoole/jsoncolor"
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
// package below cmd/ (pkg/output, internal/platform/astro/env) hands it a
// Renderer built from the result, as that package's output.Emitter: those
// packages may not import cmd/, so they take the interface, draw only the
// text, and leave the json to Emit.
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

// Renderer writes command results. Emit and EmitEvent are the single output
// path: json mode encodes the value, and text mode runs the text renderer
// over the same value. Human output is a rendering of the same data, never a
// separate code path.
type Renderer struct {
	Format Format
	Out    io.Writer
	// Style lays out a json result (Emit). The zero value, StyleAuto, decides
	// from Out, the way gh does: indented and colored on a terminal, compact
	// on one line anywhere else. A test is never on a terminal, so it sets
	// Style to see the other layouts.
	Style Style
}

// NotesTo is where a command hands the code below it to draw what it asks and
// the notes it prints along the way: out, the command's own writer, in text,
// as always; and the command's stderr under json, where stdout carries the one
// result. Nothing is asked under json (the run refuses instead), so what lands
// on stderr there is notes only.
func NotesTo(cmd *cobra.Command, format Format, out io.Writer) io.Writer {
	if format == FormatJSON {
		return cmd.ErrOrStderr()
	}
	return out
}

// Style is how a json result is laid out. Only whitespace and color differ:
// the keys, values and their order are the same in every style.
type Style int

const (
	// StyleAuto decides from the writer: StyleColor on a terminal, or
	// StyleIndented there when color is off; StyleCompact anywhere else.
	StyleAuto Style = iota
	// StyleCompact is one line, no color: what a pipe, a file or a test
	// reads. Whitespace changes do not break a parser, and a script reads
	// through a pipe, so this is what it gets.
	StyleCompact
	// StyleIndented is two-space indentation, no color.
	StyleIndented
	// StyleColor is StyleIndented, colored the way jq colors.
	StyleColor
)

// StyleFor is the layout a json result written to w gets: compact unless w is
// a terminal, and then indented, colored unless color is off.
//
// Color is off when fatih/color says so, which is the CLI's one color
// switch: it reads NO_COLOR and TERM=dumb at start-up, and `astro api
// --no-color` sets it. The terminal test is the one pkg/ansi and the old
// pkg/output printer use.
func StyleFor(w io.Writer) Style {
	f, ok := w.(*os.File)
	if !ok || !(isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())) {
		return StyleCompact
	}
	if color.NoColor {
		return StyleIndented
	}
	return StyleColor
}

// Emit writes v, a command's result. In json mode it is laid out by
// r.Style (see StyleFor); in text mode Emit calls text, which must render v
// and nothing else.
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
	style := r.Style
	// Once a run has written a stream record, it is a stream: whatever it
	// writes after (the error object that ends a failed start, a blocked
	// start's missing values, a closing record) is one more line of it, so a
	// consumer reading line by line never meets an indented object at the end.
	if streamStarted.Load() {
		style = StyleCompact
	}
	return r.emit(v, text, style)
}

// streamStarted is set by the first EmitEvent of a run, and cleared by
// ResetStream, which Execute calls before each run. A run is one process in
// production, so this is the process's own state, not shared between
// commands.
var streamStarted atomic.Bool

// ResetStream marks the start of a run: nothing has been streamed yet.
func ResetStream() { streamStarted.Store(false) }

// EmitEvent writes v as one record of a stream: a log line, a state change,
// a warning, a check finding and its summary, and whatever ends the stream.
// In json mode it is always one compact line, on a terminal too, so the
// stream is NDJSON wherever it goes and repeated calls never interleave
// indented objects with single-line ones. Text mode is Emit's.
func (r Renderer) EmitEvent(v any, text func(w io.Writer) error) error {
	if r.Format == FormatJSON {
		streamStarted.Store(true)
	}
	return r.emit(v, text, StyleCompact)
}

// emit is the door both lead through, and the one place in cmd/ that holds a
// json encoder for output (TestEmitIsTheOnlyJSONEncoder in cmd/local).
//
// HTML escaping is off: nothing here is embedded in a page, and `<` in a
// traceback reads better than \u003c. The value decoded is the same either
// way.
func (r Renderer) emit(v any, text func(w io.Writer) error, style Style) error {
	if EmitObserver != nil {
		EmitObserver(v)
	}
	if r.Format == FormatJSON {
		if style == StyleAuto {
			style = StyleFor(r.Out)
		}
		if style == StyleColor {
			enc := jsoncolor.NewEncoder(r.Out)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			enc.SetColors(jsoncolor.DefaultColors())
			return enc.Encode(v)
		}
		enc := json.NewEncoder(r.Out)
		enc.SetEscapeHTML(false)
		if style == StyleIndented {
			enc.SetIndent("", "  ")
		}
		return enc.Encode(v)
	}
	if text == nil {
		panic("Renderer.Emit: text mode with no text renderer — this value is " +
			"json-only, so the caller must not reach here in text mode. The " +
			"`if r.Format == FormatJSON` branch around a streaming Emit is what " +
			"prevents it.")
	}
	return text(r.Out)
}

// EmitObserver, when set, is handed every value Emit or EmitEvent publishes. It is nil in
// production and is a test door, exported only because the tests that arm it
// live in other packages (cmd/local's TestMain): the schema goldens pin Go
// types, and nothing in them can tell you a command still emits the type its
// golden holds. Watching the door is the only way to know, and the door is
// only worth watching because everything goes through it — see
// TestEmitIsTheOnlyJSONEncoder in cmd/local.
//
// Nothing outside a test assigns it.
var EmitObserver func(any)
