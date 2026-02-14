package api

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateJQ(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expr     string
		expected string
	}{
		{
			name:     "simple property access",
			input:    `{"name": "test", "value": 42}`,
			expr:     ".name",
			expected: "test\n",
		},
		{
			name:     "array access",
			input:    `[{"name": "a"}, {"name": "b"}, {"name": "c"}]`,
			expr:     ".[].name",
			expected: "a\nb\nc\n",
		},
		{
			name:     "filter array",
			input:    `[1, 2, 3, 4, 5]`,
			expr:     ".[] | select(. > 3)",
			expected: "4\n5\n",
		},
		{
			name:     "nested property",
			input:    `{"config": {"nested": {"value": "deep"}}}`,
			expr:     ".config.nested.value",
			expected: "deep\n",
		},
		{
			name:     "array length",
			input:    `[1, 2, 3, 4, 5]`,
			expr:     "length",
			expected: "5\n",
		},
		{
			name:     "keys",
			input:    `{"a": 1, "b": 2, "c": 3}`,
			expr:     "keys",
			expected: "[\n  \"a\",\n  \"b\",\n  \"c\"\n]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := evaluateJQ([]byte(tt.input), &buf, tt.expr, false)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestEvaluateJQColorized(t *testing.T) {
	var buf bytes.Buffer
	// null result with color
	err := evaluateJQ([]byte(`{"val": null}`), &buf, ".val", true)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "null")
	assert.Contains(t, buf.String(), "\x1b[") // ANSI escape for colored null

	// complex (object) result with color
	buf.Reset()
	err = evaluateJQ([]byte(`{"obj": {"a": 1}}`), &buf, ".obj", true)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "\x1b[")
}

func TestEvaluateJQNullNoColor(t *testing.T) {
	var buf bytes.Buffer
	err := evaluateJQ([]byte(`null`), &buf, ".", false)
	require.NoError(t, err)
	assert.Equal(t, "null\n", buf.String())
}

func TestEvaluateJQError(t *testing.T) {
	var buf bytes.Buffer

	// Invalid jq expression
	err := evaluateJQ([]byte(`{}`), &buf, ".[invalid", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing jq expression")

	// Invalid JSON
	err = evaluateJQ([]byte(`{invalid}`), &buf, ".name", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing JSON")
}

func TestEvaluateJQRuntimeError(t *testing.T) {
	var buf bytes.Buffer
	// Iterating over a non-iterable value produces a jq runtime error
	err := evaluateJQ([]byte(`42`), &buf, ".[]", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jq error")
}

func TestEvaluateTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		tmpl     string
		expected string
	}{
		{
			name:     "simple property",
			input:    `{"name": "test"}`,
			tmpl:     "Name: {{.name}}",
			expected: "Name: test",
		},
		{
			name:     "range over array",
			input:    `[{"name": "a"}, {"name": "b"}]`,
			tmpl:     "{{range .}}{{.name}}\n{{end}}",
			expected: "a\nb\n",
		},
		{
			name:     "conditional",
			input:    `{"enabled": true}`,
			tmpl:     "{{if .enabled}}enabled{{else}}disabled{{end}}",
			expected: "enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := evaluateTemplate([]byte(tt.input), &buf, tt.tmpl)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestEvaluateTemplateError(t *testing.T) {
	var buf bytes.Buffer

	// Invalid template
	err := evaluateTemplate([]byte(`{}`), &buf, "{{.name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing template")

	// Invalid JSON
	err = evaluateTemplate([]byte(`{invalid}`), &buf, "{{.name}}")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing JSON")
}

func TestWriteColorizedJSON(t *testing.T) {
	input := `{"name":"test","count":42,"enabled":true,"empty":null}`

	// Without colors
	var buf bytes.Buffer
	err := writeColorizedJSON(&buf, []byte(input), false, "  ")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "name")
	assert.Contains(t, buf.String(), "test")
	assert.Contains(t, buf.String(), "42")
	assert.NotContains(t, buf.String(), "\x1b[") // no ANSI when color disabled

	// With colors (should contain ANSI codes via force-enabled color instances)
	buf.Reset()
	err = writeColorizedJSON(&buf, []byte(input), true, "  ")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "\x1b[") // ANSI escape sequence
}

func TestWriteColorizedJSON_PerValueType(t *testing.T) {
	// Each JSON value type should be separately colorized when color is enabled.
	tests := []struct {
		name  string
		input string
		// literal is the raw (non-ANSI) substring that must appear in output.
		literal string
	}{
		{name: "string value", input: `{"key":"hello"}`, literal: "hello"},
		{name: "integer value", input: `{"key":42}`, literal: "42"},
		{name: "float value", input: `{"key":3.14}`, literal: "3.14"},
		{name: "boolean true", input: `{"key":true}`, literal: "true"},
		{name: "boolean false", input: `{"key":false}`, literal: "false"},
		{name: "null value", input: `{"key":null}`, literal: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var colored bytes.Buffer
			err := writeColorizedJSON(&colored, []byte(tt.input), true, "  ")
			require.NoError(t, err)

			out := colored.String()
			// The raw literal must still be present (within ANSI wrappers).
			assert.Contains(t, out, tt.literal)
			// The output must contain at least one ANSI escape.
			assert.Contains(t, out, "\x1b[", "expected ANSI escape for %s", tt.name)

			// Verify that the no-color variant does NOT contain ANSI escapes.
			var plain bytes.Buffer
			err = writeColorizedJSON(&plain, []byte(tt.input), false, "  ")
			require.NoError(t, err)
			assert.NotContains(t, plain.String(), "\x1b[", "plain output should have no ANSI escapes")
		})
	}
}

func TestWriteColorizedJSON_KeysVsValues(t *testing.T) {
	// Keys and string values should receive different ANSI sequences.
	input := `{"mykey":"myval"}`

	var buf bytes.Buffer
	err := writeColorizedJSON(&buf, []byte(input), true, "  ")
	require.NoError(t, err)

	out := buf.String()
	// Both key and value must be present.
	assert.Contains(t, out, "mykey")
	assert.Contains(t, out, "myval")

	// Find the ANSI prefix before "mykey" and "myval". They should differ
	// because jsoncolor assigns different colors to keys and string values.
	keyIdx := strings.Index(out, "mykey")
	valIdx := strings.Index(out, "myval")
	require.Greater(t, keyIdx, 0, "mykey should be preceded by ANSI escape")
	require.Greater(t, valIdx, 0, "myval should be preceded by ANSI escape")

	// Walk backwards from the key/val to find the start of its ANSI sequence.
	findPrecedingEscape := func(s string, pos int) string {
		// Look for the nearest \x1b[ before pos.
		sub := s[:pos]
		escIdx := strings.LastIndex(sub, "\x1b[")
		if escIdx == -1 {
			return ""
		}
		return sub[escIdx:pos]
	}

	keyEsc := findPrecedingEscape(out, keyIdx)
	valEsc := findPrecedingEscape(out, valIdx)

	assert.NotEmpty(t, keyEsc, "key should have an ANSI prefix")
	assert.NotEmpty(t, valEsc, "value should have an ANSI prefix")
	assert.NotEqual(t, keyEsc, valEsc, "keys and string values should use different ANSI colors")
}

func TestProcessOutput(t *testing.T) {
	input := []byte(`{"items": [{"name": "a"}, {"name": "b"}]}`)

	// Default (JSON pretty print)
	var buf bytes.Buffer
	err := processOutput(input, &buf, OutputOptions{Indent: "  "})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "items")

	// With jq filter
	buf.Reset()
	err = processOutput(input, &buf, OutputOptions{FilterOutput: ".items[].name", Indent: "  "})
	require.NoError(t, err)
	assert.Equal(t, "a\nb\n", buf.String())

	// With template
	buf.Reset()
	err = processOutput(input, &buf, OutputOptions{Template: "{{range .items}}{{.name}},{{end}}", Indent: "  "})
	require.NoError(t, err)
	assert.Equal(t, "a,b,", buf.String())
}

func TestTemplateFuncs(t *testing.T) {
	funcs := templateFuncs()

	// Test join (note: strings.Join signature is ([]string, string))
	joinFunc := funcs["join"].(func([]string, string) string)
	assert.Equal(t, "a,b,c", joinFunc([]string{"a", "b", "c"}, ","))

	// Test pluck
	pluckFunc := funcs["pluck"].(func(string, interface{}) []interface{})
	items := []interface{}{
		map[string]interface{}{"name": "a", "value": 1},
		map[string]interface{}{"name": "b", "value": 2},
	}
	result := pluckFunc("name", items)
	assert.Equal(t, []interface{}{"a", "b"}, result)

	// Pluck with non-array input returns nil
	result = pluckFunc("name", "not-an-array")
	assert.Nil(t, result)

	// Pluck skips items missing the key
	items = []interface{}{
		map[string]interface{}{"name": "a"},
		map[string]interface{}{"other": "b"},
	}
	result = pluckFunc("name", items)
	assert.Equal(t, []interface{}{"a"}, result)
}

func TestTemplateFuncsColor(t *testing.T) {
	funcs := templateFuncs()
	colorFunc := funcs["color"].(func(string, interface{}) string)

	// Known colors should return the value (possibly with ANSI wrapping)
	for _, c := range []string{"red", "green", "yellow", "blue", "magenta", "cyan", "white"} {
		out := colorFunc(c, "test")
		assert.Contains(t, out, "test", "color %q should contain the value", c)
	}

	// Unknown color falls through to default (plain formatting)
	out := colorFunc("unknown", "test")
	assert.Equal(t, "test", out)
}

func TestWriteColorizedJSON_InvalidJSON(t *testing.T) {
	// Invalid JSON should fall back to writing raw bytes
	raw := []byte(`not valid json`)
	var buf bytes.Buffer
	err := writeColorizedJSON(&buf, raw, false, "  ")
	require.NoError(t, err)
	assert.Equal(t, "not valid json", buf.String())
}

func TestIsColorEnabled(t *testing.T) {
	// Non-file writer always returns false
	var buf bytes.Buffer
	assert.False(t, isColorEnabled(&buf))

	// Respects NoColor flag
	origNoColor := color.NoColor
	defer func() { color.NoColor = origNoColor }()

	color.NoColor = true
	assert.False(t, isColorEnabled(&buf))
	assert.False(t, isColorEnabled(os.Stdout))
}

func TestProcessOutputColorEnabled(t *testing.T) {
	input := []byte(`{"name":"test"}`)
	var buf bytes.Buffer
	err := processOutput(input, &buf, OutputOptions{ColorEnabled: true, Indent: "  "})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "name")
	assert.Contains(t, buf.String(), "\x1b[") // ANSI escapes present
}
