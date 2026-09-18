package local

import (
	"bytes"
	"io"
	"testing"

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
