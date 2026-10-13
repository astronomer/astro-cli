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
	"errors"
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
	"github.com/spf13/pflag"
)

// Format selects how a command renders its results.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// ParseFormat validates an --output value against text, json and extras. A
// value it does not know is a usage error: the command was invoked wrongly,
// and nothing ran.
//
// This is the one place --output is parsed. A flag AddOutputFlag registered
// calls it as the value is set, so a command with one reads the Format the
// flag wrote and never calls this. The one caller besides the flag is `astro
// deploy`, whose --output is a plain string flag because it is ignored
// outside a pyproject.toml project, and so is checked only inside one.
//
// A command that renders through a package below cmd/ (pkg/output,
// internal/platform/astro/env) hands it a Renderer built from the result, as
// that package's output.Emitter: those packages may not import cmd/, so they
// take the interface, draw only the text, and leave the json to Emit.
func ParseFormat(s string, extras ...Format) (Format, error) {
	f := Format(s)
	if f == FormatText || f == FormatJSON || slices.Contains(extras, f) {
		return f, nil
	}
	return "", Usage(fmt.Errorf("unknown output format %q (supported: %s)", s, strings.Join(formatNames(extras), ", ")))
}

// AddOutputFlag registers the shared --output flag on cmd's persistent flags,
// so one registration covers a whole command family, and writes its value into
// target, FormatText until a value is given. A command that offers a format
// beyond text and json for a special use (dotenv for `astro env variable
// list`, yaml for `astro deployment inspect`) names it as an extra, here and
// nowhere else.
//
// The flag validates its own value: a format the command does not offer fails
// while cobra parses flags, as the usage error ParseFormat returns, before any
// pre-run refreshes a token, records telemetry or asks an API anything. So
// target only ever holds a format the command offers, and the command reads
// it as it is. The one --output that does not fail there is `astro deploy`'s,
// which is not this flag (see ParseFormat).
//
// An extra never reaches a Renderer, which renders text and json only (see
// Emit): the command maps it first, to text whose renderer draws it (inspect's
// yaml) or to a writer of its own (dotenv).
//
// It owns cmd's flag error func: one set on cmd beforehand is replaced. Any
// flag error other than a refused --output goes to the parent's, which is
// looked up when the error happens, so a func an ancestor gains later (the
// root's, which Execute sets) is consulted. On a root there is no parent, and
// Execute's handling takes over when it runs the tree, replacing this func.
func AddOutputFlag(cmd *cobra.Command, target *Format, extras ...Format) {
	usage := "Output format: " + listFormats(extras)
	*target = FormatText
	cmd.PersistentFlags().VarP(&formatValue{target: target, extras: extras}, "output", "o", usage)
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if bad := badFormat(err); bad != nil {
			return bad.refusal()
		}
		if cmd.HasParent() {
			// A func that returns nil must not turn a flag error into
			// success, so the error stands unless it gives another.
			if e := cmd.Parent().FlagErrorFunc()(c, err); e != nil {
				err = e
			}
		}
		if IsUsage(err) {
			return err
		}
		return Usage(err)
	})
}

// formatValue is --output's value: a Format that refuses a format the command
// does not offer, and what the command has to say when it does (OnBadFormat).
type formatValue struct {
	target  *Format
	extras  []Format
	explain func(value string, refused error) error
}

func (v *formatValue) String() string { return string(*v.target) }

// Type is "string", what the flag was before it validated, so the help still
// reads `-o, --output string`.
func (v *formatValue) Type() string { return "string" }

func (v *formatValue) Set(s string) error {
	f, err := ParseFormat(s, v.extras...)
	if err != nil {
		return err
	}
	*v.target = f
	return nil
}

// OnBadFormat lets cmd say more than "unknown output format" when its
// --output value is refused: explain gets the value and the refusal, and what
// it returns is reported instead, as a usage error. Returning nil, or refused
// itself, keeps the plain refusal. For a value with a story: `-o <path>` from
// when -o named audit-logs export's file, or dotenv on a listing that has no
// values.
//
// The explanation is kept on the flag, so cmd must be the command that
// registered --output with AddOutputFlag; anything else is a programming
// error, and panics.
func OnBadFormat(cmd *cobra.Command, explain func(value string, refused error) error) {
	f := cmd.PersistentFlags().Lookup("output")
	if f == nil {
		panic("cliout.OnBadFormat: " + cmd.CommandPath() + " has no --output of its own; call AddOutputFlag on it first")
	}
	v, ok := f.Value.(*formatValue)
	if !ok {
		panic("cliout.OnBadFormat: " + cmd.CommandPath() + "'s --output is not cliout's; register it with AddOutputFlag")
	}
	v.explain = explain
}

// HasOutput reports whether cmd has the --output AddOutputFlag registers, its
// own or inherited, as a run of cmd would. `astro deploy`'s --output is a
// plain string flag, and not it.
func HasOutput(cmd *cobra.Command) bool {
	return outputValue(cmd) != nil
}

// FormatList is what cmd's --output takes, in the words its help gives them
// ("text, json or dotenv"), or "" when cmd has no such flag (HasOutput).
func FormatList(cmd *cobra.Command) string {
	v := outputValue(cmd)
	if v == nil {
		return ""
	}
	return listFormats(v.extras)
}

// Offers reports whether cmd's --output takes f.
func Offers(cmd *cobra.Command, f Format) bool {
	v := outputValue(cmd)
	return v != nil && (f == FormatText || f == FormatJSON || slices.Contains(v.extras, f))
}

// outputValue is the value of cmd's --output when AddOutputFlag registered
// it, or nil.
func outputValue(cmd *cobra.Command) *formatValue {
	f := cmd.Flag("output")
	if f == nil {
		return nil
	}
	if v, ok := f.Value.(*formatValue); ok {
		return v
	}
	return nil
}

// refusedFormat is a value an --output flag refused, and why.
type refusedFormat struct {
	flag  *formatValue
	value string
	err   error
}

// refusal is what a run reports for it: the command's explanation, if it has
// one to give, else ParseFormat's own usage error, without pflag's `invalid
// argument "x" for "-o, --output" flag:` in front of it.
func (r *refusedFormat) refusal() error {
	if r.flag.explain == nil {
		return r.err
	}
	explained := r.flag.explain(r.value, r.err)
	if explained == nil || explained == r.err {
		return r.err
	}
	return Usage(explained)
}

// badFormat returns err as a refused --output value, or nil if it is not one.
func badFormat(err error) *refusedFormat {
	var bad *pflag.InvalidValueError
	if !errors.As(err, &bad) {
		return nil
	}
	v, ok := bad.GetFlag().Value.(*formatValue)
	if !ok {
		return nil
	}
	return &refusedFormat{flag: v, value: bad.GetValue(), err: bad.Unwrap()}
}

// listFormats joins text, json and the extras the way --output's help names
// them: "text, json or dotenv".
func listFormats(extras []Format) string {
	names := formatNames(extras)
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
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
// It renders text and json, and panics on any other Format: "" from a
// variable no flag was bound to, or an extra the command did not map before
// it built the Renderer. Either is a programming error, and rendering it as
// text would hide it behind exit 0.
//
// HTML escaping is off: nothing here is embedded in a page, and `<` in a
// traceback reads better than \u003c. The value decoded is the same either
// way.
func (r Renderer) emit(v any, text func(w io.Writer) error, style Style) error {
	if r.Format != FormatText && r.Format != FormatJSON {
		panic(fmt.Sprintf("Renderer.Emit: format %q is neither text nor json. A Renderer's Format "+
			"comes from a flag AddOutputFlag bound, and a command maps an extra it offers "+
			"(yaml, dotenv) before it builds the Renderer.", r.Format))
	}
	if lazy, ok := v.(Lazy); ok {
		if r.Format != FormatJSON {
			return r.text(text)
		}
		v = lazy()
	}
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
	return r.text(text)
}

// Lazy is a payload built only if it is published: Emit calls it in json mode
// and never in text mode, for a value that costs something to build and that
// the text rendering does not read (describe's resolved schemas).
//
// A Lazy emitted in text mode publishes nothing, so EmitObserver is not told
// of it: there is no value to hand it without building one.
type Lazy func() any

// text runs the text renderer, which must exist in text mode.
func (r Renderer) text(text func(w io.Writer) error) error {
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
