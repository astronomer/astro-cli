package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/itchyny/gojq"
	jsoncolor "github.com/neilotoole/jsoncolor"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/input"
)

// newColor creates a color.Color that is force-enabled for output.
// Since isColorEnabled() already gates whether colorization happens,
// we force-enable here so fatih/color doesn't second-guess us based
// on its own terminal detection.
func newColor(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	c.EnableColor()
	return c
}

// OutputOptions configures how output is processed.
type OutputOptions struct {
	FilterOutput string // jq filter expression
	Template     string // Go template
	ColorEnabled bool   // whether to colorize output
	Indent       string // indentation for JSON
}

// processOutput processes the response body based on output options.
func processOutput(body []byte, out io.Writer, opts OutputOptions) error {
	// jq filtering takes precedence
	if opts.FilterOutput != "" {
		return evaluateJQ(body, out, opts.FilterOutput, opts.ColorEnabled)
	}

	// Go template formatting
	if opts.Template != "" {
		return evaluateTemplate(body, out, opts.Template)
	}

	// Default: pretty-print JSON
	return writeColorizedJSON(out, body, opts.ColorEnabled, opts.Indent)
}

// evaluateJQ evaluates a jq expression against the input JSON.
func evaluateJQ(body []byte, out io.Writer, expr string, colorize bool) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("parsing jq expression: %w", err)
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	terminal := isTerminal(out)
	iter := query.Run(data)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			// A jq error can carry response text (error(.message)), and it is
			// printed to stderr as a message: without what a terminal would
			// act on.
			return fmt.Errorf("jq error: %s", input.Lines(err.Error()))
		}

		// Format output
		output, err := formatValue(v, colorize, terminal)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, output)
	}

	return nil
}

// formatValue formats a single value for output. A string is printed as it
// is, so for a terminal it is printed without the escapes a response can
// carry (input.Lines); anything else is re-encoded as JSON, which escapes them
// itself.
func formatValue(v interface{}, colorize, terminal bool) (string, error) {
	switch val := v.(type) {
	case string:
		if terminal {
			return input.Lines(val), nil
		}
		return val, nil
	case nil:
		if colorize {
			return newColor(color.FgCyan).Sprint("null"), nil
		}
		return "null", nil
	default:
		// For complex types, marshal to JSON
		var buf bytes.Buffer
		if err := writeColorizedJSON(&buf, mustMarshal(v), colorize, "  "); err != nil {
			return "", err
		}
		return strings.TrimRight(buf.String(), "\n"), nil
	}
}

// mustMarshal marshals a value to JSON, returning raw bytes on error.
func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(fmt.Sprintf("%v", v))
	}
	return b
}

// evaluateTemplate evaluates a Go template against the input JSON.
func evaluateTemplate(body []byte, out io.Writer, tmplStr string) error {
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	tmpl, err := template.New("output").Funcs(templateFuncs()).Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	// The response's strings, not the template's output, are what get
	// cleaned for a terminal: the template's own color calls are escapes too.
	if isTerminal(out) {
		data = linesIn(data)
	}
	return tmpl.Execute(out, data)
}

// templateFuncs returns custom template functions.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"color": func(c string, s interface{}) string {
			switch c {
			case "red":
				return color.RedString("%v", s)
			case "green":
				return color.GreenString("%v", s)
			case "yellow":
				return color.YellowString("%v", s)
			case "blue":
				return color.BlueString("%v", s)
			case "magenta":
				return color.MagentaString("%v", s)
			case "cyan":
				return color.CyanString("%v", s)
			case "white":
				return color.WhiteString("%v", s)
			default:
				return fmt.Sprintf("%v", s)
			}
		},
		"pluck": func(key string, items interface{}) []interface{} {
			var result []interface{}
			if arr, ok := items.([]interface{}); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						if v, exists := m[key]; exists {
							result = append(result, v)
						}
					}
				}
			}
			return result
		},
	}
}

// writeColorizedJSON writes pretty-printed (and optionally colorized) JSON to the writer.
// Uses neilotoole/jsoncolor for colorization, which provides jq-like colors and is
// significantly faster than stdlib for indentation.
func writeColorizedJSON(w io.Writer, data []byte, colorize bool, indent string) error {
	// Parse the raw JSON into a generic value so the encoder can re-emit it.
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		// If we can't parse it, write it as it came and bail: on a terminal,
		// without the escapes it may carry (forTerminal).
		_, writeErr := io.WriteString(w, forTerminal(w, string(data)))
		return writeErr
	}

	enc := jsoncolor.NewEncoder(w)
	enc.SetIndent("", indent)
	enc.SetEscapeHTML(false)

	if colorize {
		enc.SetColors(jsoncolor.DefaultColors())
	}

	return enc.Encode(v)
}

// isColorEnabled returns true if color output should be enabled.
func isColorEnabled(out io.Writer) bool {
	// Respect fatih/color's NoColor setting (set by --no-color flag or NO_COLOR env)
	if color.NoColor {
		return false
	}
	return isTerminal(out)
}

// isTerminal reports whether w is a terminal. A variable so tests can stand
// one in for a buffer.
var isTerminal = cliout.IsTerminal

// forTerminal is s as it is written to w. A response's text, printed as it
// came, reaches a terminal without its escape sequences and other control
// characters (input.Lines), since the terminal would act on them rather than
// show them; written to a file or a pipe it is left byte for byte.
func forTerminal(w io.Writer, s string) string {
	if isTerminal(w) {
		return input.Lines(s)
	}
	return s
}

// headForTerminal is a response's status line and headers as -i and -v print
// them to w: on a terminal, each one line (input.Plain) and without what the
// terminal would act on; anywhere else, as they came.
func headForTerminal(w io.Writer, status string, headers http.Header) (string, http.Header) {
	if !isTerminal(w) {
		return status, headers
	}
	plain := make(http.Header, len(headers))
	for name, values := range headers {
		for _, v := range values {
			plain[name] = append(plain[name], input.Plain(v))
		}
	}
	return input.Plain(status), plain
}

// linesIn is a decoded JSON value with input.Lines applied to every string
// value in it, for a template writing to a terminal. Keys are left as they
// are: cleaning them could make two keys one, and which value survived would
// depend on map order.
func linesIn(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return input.Lines(val)
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = linesIn(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, item := range val {
			out[k] = linesIn(item)
		}
		return out
	default:
		return v
	}
}
