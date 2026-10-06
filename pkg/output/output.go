// Package output renders the cloud lists' tables. It does not encode json:
// a list's result goes to the command's Renderer, whose json is the CLI's
// one encoder (cmd/cliout), and this package draws only the text view.
package output

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/pkg/printutil"
)

// Emitter is the door a command's result leaves by: the value, which it
// publishes as json when the command was asked for json, and the text
// renderer it runs otherwise. cmd/cliout.Renderer is the implementation the
// CLI runs. It lives in cmd/, which nothing below cmd/ may import, so the
// command hands its Renderer down as this, and the json layout (pretty and
// colored on a terminal, compact when piped) is decided in one place.
type Emitter interface {
	Emit(v any, text func(w io.Writer) error) error
}

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

// printTable renders data as a table using printutil.Table
func printTable(cfg *TableConfig, data any, out io.Writer) error {
	if cfg == nil {
		return fmt.Errorf("table config required for text format")
	}
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

	return tab.Print(out)
}

// PrintData fetches data via fetchFn and publishes it through r: as json, or
// as the table tableCfg describes. This eliminates boilerplate in the common
// pattern of: fetch data, then render it in the format the command was asked
// for.
func PrintData[T any](fetchFn func() (*T, error), tableCfg *TableConfig, r Emitter) error {
	data, err := fetchFn()
	if err != nil {
		return err
	}
	return r.Emit(data, func(w io.Writer) error { return printTable(tableCfg, data, w) })
}
