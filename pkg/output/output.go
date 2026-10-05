package output

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	jsoncolor "github.com/neilotoole/jsoncolor"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/printutil"
)

// Format represents the output format type
type Format string

const (
	// FormatText outputs the human rendering, a table
	FormatText Format = "text"
	// FormatJSON outputs in JSON format
	FormatJSON Format = "json"
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
	// Format specifies the output format (text or json)
	Format Format
	// Out is the writer for output (defaults to os.Stdout)
	Out io.Writer
	// NoColor disables colorization
	NoColor bool
	// Table configures table rendering (required when Format == FormatText)
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
	case FormatText:
		if p.opts.Table == nil {
			return fmt.Errorf("table config required for text format")
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

// ParseFormat validates an --output flag value. The wording matches the
// astro local tree's, so every command rejects a bad -o the same way.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatText, FormatJSON:
		return Format(s), nil
	default:
		return "", fmt.Errorf("unknown output format %q (supported: text, json)", s)
	}
}

// Flags holds the output flag value for a command.
type Flags struct {
	Format string
}

// AddFlags registers --output/-o on a cobra command.
func (f *Flags) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.Format, "output", "o", string(FormatText), "Output format: text or json")
}

// Resolve returns the parsed Format from the flag value.
func (f *Flags) Resolve() (Format, error) {
	return ParseFormat(f.Format)
}

// PrintData fetches data via fetchFn and renders it using the given table config, format, and writer.
// This eliminates boilerplate in the common pattern of: fetch data, create printer, call Print.
func PrintData[T any](fetchFn func() (*T, error), tableCfg *TableConfig, format Format, out io.Writer) error {
	data, err := fetchFn()
	if err != nil {
		return err
	}

	return New(Options{
		Format: format,
		Out:    out,
		Table:  tableCfg,
	}).Print(data)
}
