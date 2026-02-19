package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/itchyny/gojq"
	"github.com/mattn/go-isatty"
	jsoncolor "github.com/neilotoole/jsoncolor"
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
func evaluateJQ(input []byte, out io.Writer, expr string, colorize bool) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("parsing jq expression: %w", err)
	}

	var data interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	iter := query.Run(data)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq error: %w", err)
		}

		// Format output
		output, err := formatValue(v, colorize)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, output)
	}

	return nil
}

// formatValue formats a single value for output.
func formatValue(v interface{}, colorize bool) (string, error) {
	switch val := v.(type) {
	case string:
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
func evaluateTemplate(input []byte, out io.Writer, tmplStr string) error {
	var data interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	tmpl, err := template.New("output").Funcs(templateFuncs()).Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
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
		// If we can't parse it, write raw and bail.
		_, writeErr := w.Write(data)
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
	if f, ok := out.(*os.File); ok {
		return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return false
}
