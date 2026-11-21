// Package texttable lays out the CLI's lists as aligned text columns, with the
// standard library's text/tabwriter and nothing else.
//
// It lives below cmd/ so that a list drawn in internal/ (a picker, most of
// all: see pkg/picker) looks the same as one drawn in cmd/, where cmd/cliout
// names this same type cliout.Table.
package texttable

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/astronomer/astro-cli/pkg/input"
)

// gap is the space after each column's widest cell.
const gap = 5

// Table is a list printed as aligned columns under a header row: no borders,
// no color. It writes the layout the CLI's tables have always had (the older
// printutil.Table's), so a list converted to it looks the same:
//
//	#     KEY      VALUE     SECRET
//	1     A        1         false
//	2     TOKEN    ****      true
//
// Every line also starts with one space, which gofmt strips from the example,
// and each column, the last included, is as wide as its widest cell (header
// included) plus five spaces. Widths count characters, not bytes, so a row
// holding "é" lines up with the rest; a double-width character, as in CJK
// text, still counts as one.
//
// A cell is one line: a tab or a line break in it would split the row or open
// a column that shifts every row below, so AddRow turns each into a space
// (and drops a carriage return).
type Table struct {
	// Header is the first row. It sets the number of columns.
	Header []string
	// Empty is said, on EmptyTo, when the table has no rows. Left "", an
	// empty table prints its header row and nothing under it. It is a message
	// for a person, so it never goes to the stream a script reads: that gets
	// the header alone when Plain, and nothing when drawn for a terminal.
	Empty string
	// EmptyTo is where Empty is said. Nil is the notices writer of the
	// destination (cliout.TableText), else os.Stderr.
	EmptyTo io.Writer
	// Plain lays the table out for a script: the cells of a line joined by one
	// tab, with no leading space and no padding. Left false, the table is the
	// aligned layout a person reads. cliout.TableText sets it from the
	// destination.
	Plain bool

	rows [][]string
}

// AddRow appends one row, a cell per column. Each cell goes in as input.Cell
// has it: tabwriter ends a cell at \t and \v and a line at \n and \f,
// whatever its flags, so each of those becomes a space, and a terminal would
// act on an escape in a value from somewhere else.
func (t *Table) AddRow(cells ...string) {
	row := make([]string, len(cells))
	for i, c := range cells {
		row[i] = input.Cell(c)
	}
	t.rows = append(t.rows, row)
}

// Render writes the table to b. It takes a bufio.Writer, not any io.Writer,
// because it does not report a failed write: b keeps it for its Flush, which
// the caller checks (cmd/cliout's WriteText and Text do).
func (t *Table) Render(b *bufio.Writer) {
	if len(t.rows) == 0 && t.Empty != "" {
		to := t.EmptyTo
		if to == nil {
			to = os.Stderr
		}
		fmt.Fprintln(to, t.Empty)
		if t.Plain {
			fmt.Fprintln(b, strings.Join(t.Header, "\t"))
		}
		return
	}
	if t.Plain {
		fmt.Fprintln(b, strings.Join(t.Header, "\t"))
		for _, row := range t.rows {
			fmt.Fprintln(b, strings.Join(row, "\t"))
		}
		return
	}
	// Every cell, the last included, ends in a tab, so every column is padded
	// to its width: the old tables padded the last column too, and a line
	// that is byte-for-byte what it was is the point.
	tw := tabwriter.NewWriter(b, 0, 0, gap, ' ', 0)
	writeLine(tw, t.Header)
	for _, row := range t.rows {
		writeLine(tw, row)
	}
	tw.Flush() //nolint:errcheck // its only error is b's, which is sticky: the caller's Flush reports it
}

func writeLine(tw *tabwriter.Writer, cells []string) {
	fmt.Fprintln(tw, " "+strings.Join(cells, "\t")+"\t")
}

// Columns lays out lines whose cells are separated by tabs, for a renderer
// that writes its own rows (fmt.Fprintln(c, "A\tB")) instead of building a
// Table. To a terminal, the cells are aligned in columns two spaces apart, as
// text/tabwriter does with its padding of 2. To anything else, a pipe or a
// file a script reads, the tabs are left as they are: one tab between cells,
// no padding. Flush ends the writing and reports the one error that matters.
//
// terminal is whether the destination is one (input.IsTerminalWriter).
func Columns(w io.Writer, terminal bool) interface {
	io.Writer
	Flush() error
} {
	if terminal {
		return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	}
	return &plainColumns{w: w}
}

type plainColumns struct{ w io.Writer }

func (p *plainColumns) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *plainColumns) Flush() error                { return nil }

// Say tells a person something about a listing: on the notices writer of w
// (see input.WithNotices), stderr when it carries none, where a script reading
// stdout does not see it.
func Say(w io.Writer, message string) {
	fmt.Fprintln(input.NoticesFor(w), message)
}

// SayEmpty says an empty list's message, which is for a person: on stderr. A
// script's stdout, when w is not a terminal, keeps the header line alone;
// a terminal's keeps nothing, as the message is what a person came for.
func SayEmpty(w io.Writer, terminal bool, message string, header []string) error {
	Say(w, message)
	if terminal {
		return nil
	}
	_, err := fmt.Fprintln(w, strings.Join(header, "\t"))
	return err
}
