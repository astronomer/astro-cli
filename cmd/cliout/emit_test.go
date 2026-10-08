package cliout

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A json-only value is legal on a streaming surface, where the caller has
// established the format before it gets here.
func TestEmitWithNoTextRendererWritesJSON(t *testing.T) {
	var out bytes.Buffer
	r := Renderer{Format: FormatJSON, Out: &out}

	require.NoError(t, r.Emit(map[string]string{"k": "v"}, nil))
	assert.JSONEq(t, `{"k":"v"}`, out.String())
}

// And reaching text mode with one is a programming error that says so.
//
// The alternative, returning nil, is what the first version of this did. It
// looks harmless and is not: `astro local list` in text mode would print an
// empty table and exit 0, telling a human nothing, and no test would fail.
// The loud version is the whole guarantee, so it needs a test of its own —
// without one, reverting the panic to a silent return breaks nothing.
func TestEmitPanicsInTextModeWithNoTextRenderer(t *testing.T) {
	var out bytes.Buffer
	r := Renderer{Format: FormatText, Out: &out}

	assert.PanicsWithValue(t,
		"Renderer.Emit: text mode with no text renderer — this value is "+
			"json-only, so the caller must not reach here in text mode. The "+
			"`if r.Format == FormatJSON` branch around a streaming Emit is what "+
			"prevents it.",
		func() { _ = r.Emit(map[string]string{"k": "v"}, nil) })

	assert.Empty(t, out.String(), "nothing should reach the writer")
}

// A Format that is neither text nor json never came through a flag
// AddOutputFlag bound: "" is a variable no flag was bound to, and an extra
// (yaml, dotenv) is the command's to map before it builds a Renderer. Both
// are programming errors, and rendering them as text would hide one behind
// exit 0, so every door panics on them, with a text renderer or without, and
// for a Lazy too, before anything reaches the writer or the observer.
func TestEmitPanicsOnAFormatThatIsNeitherTextNorJSON(t *testing.T) {
	text := func(w io.Writer) error {
		_, err := io.WriteString(w, "rendered")
		return err
	}
	observed := 0
	EmitObserver = func(any) { observed++ }
	t.Cleanup(func() { EmitObserver = nil })
	for _, f := range []Format{"", "yaml", "dotenv", "xml"} {
		var out bytes.Buffer
		r := Renderer{Format: f, Out: &out}
		assert.Panics(t, func() { _ = r.Emit("v", text) }, "Emit, format %q", f)
		assert.Panics(t, func() { _ = r.EmitEvent("v", text) }, "EmitEvent, format %q", f)
		assert.Panics(t, func() { _ = r.Emit(Lazy(func() any { return "v" }), text) }, "a Lazy, format %q", f)
		assert.Empty(t, out.String(), "format %q: nothing should reach the writer", f)
	}
	assert.Zero(t, observed, "nothing should reach the observer")
}

// The ordinary path still runs the text renderer over the same value.
func TestEmitTextModeRunsTheRenderer(t *testing.T) {
	var out bytes.Buffer
	r := Renderer{Format: FormatText, Out: &out}

	require.NoError(t, r.Emit("ignored", func(w io.Writer) error {
		_, err := io.WriteString(w, "rendered")
		return err
	}))
	assert.Equal(t, "rendered", out.String())
}

// layoutCase is a result with what the json layouts have to keep intact:
// nesting, a list, and characters an HTML-escaping encoder would rewrite.
type layoutCase struct {
	Items []layoutItem `json:"items"`
}

type layoutItem struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

var layoutValue = layoutCase{Items: []layoutItem{{Name: "a<b>&c", URL: "https://example.com/?x=1&y=2"}}}

const layoutLine = `{"items":[{"name":"a<b>&c","url":"https://example.com/?x=1&y=2"}]}` + "\n"

// Off a terminal a result is one compact line, and the bytes are the
// contract a script reads: no indentation, no HTML escaping, one trailing
// newline. A buffer is never a terminal, so the zero Style lands here.
func TestEmitResultIsCompactOffATerminal(t *testing.T) {
	for _, style := range []Style{StyleAuto, StyleCompact} {
		var out bytes.Buffer
		require.NoError(t, Renderer{Format: FormatJSON, Out: &out, Style: style}.Emit(layoutValue, nil))
		assert.Equal(t, layoutLine, out.String(), "style %d", style)
	}
}

// On a terminal it is indented two spaces, and colored unless color is off.
// Only whitespace and color change: the same value decodes from each.
//
// With no stream started: streamStarted is package state, and a test that
// ran an EmitEvent before this one would turn the result compact.
func TestEmitResultIsIndentedOnATerminal(t *testing.T) {
	ResetStream()
	var out bytes.Buffer
	require.NoError(t, Renderer{Format: FormatJSON, Out: &out, Style: StyleIndented}.Emit(layoutValue, nil))
	assert.Equal(t, `{
  "items": [
    {
      "name": "a<b>&c",
      "url": "https://example.com/?x=1&y=2"
    }
  ]
}
`, out.String())

	var colored bytes.Buffer
	require.NoError(t, Renderer{Format: FormatJSON, Out: &colored, Style: StyleColor}.Emit(layoutValue, nil))
	assert.Contains(t, colored.String(), "\x1b[", "StyleColor writes ANSI color")
	assert.Equal(t, out.String(), ansiEscape.ReplaceAllString(colored.String(), ""),
		"with the color stripped, StyleColor is StyleIndented byte for byte")
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// An event is one line whatever the Renderer's Style says: a stream stays
// NDJSON on a terminal, where a result would be indented.
func TestEmitEventIsAlwaysOneLine(t *testing.T) {
	t.Cleanup(ResetStream)
	for _, style := range []Style{StyleAuto, StyleCompact, StyleIndented, StyleColor} {
		var out bytes.Buffer
		r := Renderer{Format: FormatJSON, Out: &out, Style: style}
		require.NoError(t, r.EmitEvent(layoutValue, nil))
		require.NoError(t, r.EmitEvent(layoutValue, nil))
		assert.Equal(t, layoutLine+layoutLine, out.String(), "style %d", style)
	}
}

// Once a run has streamed, a result after it is one more line of the stream,
// not an indented object: a failed `astro local start -o json` ends with its
// error object on one line, on a terminal too. A new run starts afresh.
func TestAResultAfterAStreamIsOneLine(t *testing.T) {
	t.Cleanup(ResetStream)
	ResetStream()
	var out bytes.Buffer
	r := Renderer{Format: FormatJSON, Out: &out, Style: StyleColor}
	require.NoError(t, r.EmitEvent(layoutValue, nil))
	require.NoError(t, r.Emit(layoutValue, nil))
	assert.Equal(t, layoutLine+layoutLine, out.String())

	ResetStream()
	out.Reset()
	require.NoError(t, Renderer{Format: FormatJSON, Out: &out, Style: StyleIndented}.Emit(layoutValue, nil))
	assert.Contains(t, out.String(), "\n  ", "with no stream, a result is laid out as its style says")
}

// Execute clears a previous run's stream, so a result in this run is laid out
// by its own style.
func TestExecuteStartsWithNoStream(t *testing.T) {
	t.Cleanup(ResetStream)
	var out bytes.Buffer
	require.NoError(t, Renderer{Format: FormatJSON, Out: &out}.EmitEvent(layoutValue, nil))
	root := &cobra.Command{Use: "astro", RunE: func(*cobra.Command, []string) error { return nil }}
	require.NoError(t, Execute(context.Background(), root, nil, io.Discard, nil))
	assert.False(t, streamStarted.Load())
}

// EmitEvent's text mode is Emit's: the renderer, over the same value.
func TestEmitEventTextModeRunsTheRenderer(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, Renderer{Format: FormatText, Out: &out}.EmitEvent("ignored", func(w io.Writer) error {
		_, err := io.WriteString(w, "rendered")
		return err
	}))
	assert.Equal(t, "rendered", out.String())
}

// Both doors are watched.
func TestEmitObserverSeesResultsAndEvents(t *testing.T) {
	var seen []any
	EmitObserver = func(v any) { seen = append(seen, v) }
	t.Cleanup(func() { EmitObserver = nil })
	t.Cleanup(ResetStream)

	r := Renderer{Format: FormatJSON, Out: io.Discard}
	require.NoError(t, r.Emit("result", nil))
	require.NoError(t, r.EmitEvent("event", nil))
	assert.Equal(t, []any{"result", "event"}, seen)
}

// StyleFor decides from the writer: anything that is not a terminal is
// compact, which is what keeps every test, pipe and redirect on one line.
func TestStyleForIsCompactOffATerminal(t *testing.T) {
	assert.Equal(t, StyleCompact, StyleFor(&bytes.Buffer{}))

	f, err := os.CreateTemp(t.TempDir(), "out")
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	assert.Equal(t, StyleCompact, StyleFor(f), "a regular file is not a terminal")
}
