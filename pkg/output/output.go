package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	jsoncolor "github.com/neilotoole/jsoncolor"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/printutil"
)

// Format represents the output format type
type Format string

const (
	// FormatTable outputs in a human-readable table format
	FormatTable Format = "table"
	// FormatJSON outputs in JSON format
	FormatJSON Format = "json"
	// FormatTemplate outputs using a custom Go template
	FormatTemplate Format = "template"
)

// TableColumn defines how to extract one column from a data item (type-erased)
type TableColumn struct {
	Header string
	Value  func(item any) string
}

// TableConfig configures table rendering for the Printer
type TableConfig struct {
	Columns      []TableColumn
	Items        func(data any) []any
	ColorRow     func(item any) bool
	ColorRowCode [2]string
	Padding      []int
	NoResultsMsg string
}

// Column defines a type-safe table column
type Column[T any] struct {
	Header string
	Value  func(T) string
}

// TableOption applies optional configuration to a TableConfig
type TableOption func(*TableConfig)

// WithColorRow adds row coloring based on a predicate
func WithColorRow[T any](pred func(T) bool, code [2]string) TableOption {
	return func(tc *TableConfig) {
		tc.ColorRow = func(item any) bool { return pred(item.(T)) }
		tc.ColorRowCode = code
	}
}

// WithPadding sets explicit column padding
func WithPadding(padding []int) TableOption {
	return func(tc *TableConfig) {
		tc.Padding = padding
	}
}

// WithNoResultsMsg sets the message shown when there are no results
func WithNoResultsMsg(msg string) TableOption {
	return func(tc *TableConfig) {
		tc.NoResultsMsg = msg
	}
}

// BuildTableConfig converts type-safe Column[T] definitions into a TableConfig
func BuildTableConfig[T any](columns []Column[T], items func(data any) []T, opts ...TableOption) *TableConfig {
	wrappedCols := make([]TableColumn, len(columns))
	for i, c := range columns {
		wrappedCols[i] = TableColumn{
			Header: c.Header,
			Value:  func(v any) string { return c.Value(v.(T)) },
		}
	}

	tc := &TableConfig{
		Columns: wrappedCols,
		Items: func(d any) []any {
			ts := items(d)
			out := make([]any, len(ts))
			for i := range ts {
				out[i] = ts[i]
			}
			return out
		},
	}

	for _, opt := range opts {
		opt(tc)
	}

	return tc
}

// Options configures how output is formatted
type Options struct {
	// Format specifies the output format (table, json, or template)
	Format Format
	// Template is the Go template string (only used when Format == FormatTemplate)
	Template string
	// Out is the writer for output (defaults to os.Stdout)
	Out io.Writer
	// NoColor disables colorization
	NoColor bool
	// Table configures table rendering (required when Format == FormatTable)
	Table *TableConfig
}

// GetOut returns the output writer, defaulting to os.Stdout
func (o *Options) GetOut() io.Writer {
	if o.Out != nil {
		return o.Out
	}
	return os.Stdout
}

// IsColorEnabled returns true if color output should be enabled
func (o *Options) IsColorEnabled() bool {
	if o.NoColor || color.NoColor {
		return false
	}
	if f, ok := o.GetOut().(*os.File); ok {
		return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return false
}

// Printer provides consistent output formatting across commands
type Printer struct {
	opts Options
}

// New creates a new Printer with the given options
func New(opts Options) *Printer {
	return &Printer{opts: opts}
}

// Print outputs data according to the configured format
func (p *Printer) Print(data any) error {
	switch p.opts.Format {
	case FormatJSON:
		return p.printJSON(data)
	case FormatTemplate:
		return p.printTemplate(data)
	case FormatTable:
		if p.opts.Table == nil {
			return fmt.Errorf("table config required for table format")
		}
		return p.printTable(data)
	default:
		return fmt.Errorf("unsupported output format: %s", p.opts.Format)
	}
}

// printTable renders data as a table using printutil.Table
func (p *Printer) printTable(data any) error {
	cfg := p.opts.Table
	items := cfg.Items(data)

	headers := make([]string, len(cfg.Columns))
	for i, col := range cfg.Columns {
		headers[i] = col.Header
	}

	tab := &printutil.Table{
		Header:         headers,
		DynamicPadding: true,
		Padding:        cfg.Padding,
		NoResultsMsg:   cfg.NoResultsMsg,
		ColorRowCode:   cfg.ColorRowCode,
	}

	for _, item := range items {
		row := make([]string, len(cfg.Columns))
		for i, col := range cfg.Columns {
			row[i] = col.Value(item)
		}
		colored := cfg.ColorRow != nil && cfg.ColorRow(item)
		tab.AddRow(row, colored)
	}

	return tab.Print(p.opts.GetOut())
}

// printJSON outputs data as JSON (with optional colorization)
func (p *Printer) printJSON(data any) error {
	enc := jsoncolor.NewEncoder(p.opts.GetOut())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if p.opts.IsColorEnabled() {
		enc.SetColors(jsoncolor.DefaultColors())
	}

	return enc.Encode(data)
}

// printTemplate executes a Go template against the data
func (p *Printer) printTemplate(data any) error {
	if p.opts.Template == "" {
		return fmt.Errorf("template string is required for template format")
	}

	tmpl, err := template.New("output").Funcs(templateFuncs()).Parse(p.opts.Template)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	return tmpl.Execute(p.opts.GetOut(), data)
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}
}

// ParseFormat parses a format string into a Format type
func ParseFormat(s string) (Format, error) {
	switch s {
	case "table", "":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "template":
		return FormatTemplate, nil
	default:
		return "", fmt.Errorf("invalid format %q (must be table, json, or template)", s)
	}
}

// ResolveFormat determines the output format from the JSON shorthand flag and format string.
func ResolveFormat(jsonFlag bool, formatStr string) (Format, error) {
	if jsonFlag {
		return FormatJSON, nil
	}
	return ParseFormat(formatStr)
}

// Flags holds the common output flag values for a command.
type Flags struct {
	JSON     bool
	Format   string
	Template string
}

// AddFlags registers --json, --output/-o, and --template flags on a cobra command.
func (f *Flags) AddFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.JSON, "json", false, "Output as JSON")
	cmd.Flags().StringVarP(&f.Format, "output", "o", "", "Output format (table|json|template)")
	cmd.Flags().StringVar(&f.Template, "template", "", "Go template string (use with --output template)")
	cmd.MarkFlagsMutuallyExclusive("json", "output")
}

// Resolve returns the parsed Format from the flag values.
func (f *Flags) Resolve() (Format, error) {
	return ResolveFormat(f.JSON, f.Format)
}

// PrintData fetches data via fetchFn and renders it using the given table config, format, template, and writer.
// This eliminates boilerplate in the common pattern of: fetch data, create printer, call Print.
func PrintData[T any](fetchFn func() (*T, error), tableCfg *TableConfig, format Format, tmpl string, out io.Writer) error {
	data, err := fetchFn()
	if err != nil {
		return err
	}

	return New(Options{
		Format:   format,
		Template: tmpl,
		Out:      out,
		Table:    tableCfg,
	}).Print(data)
}
