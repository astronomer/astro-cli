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
	"strings"
	"text/tabwriter"
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
	// Empty is printed, alone, when the table has no rows. Left "", an empty
	// table prints its header row and nothing under it.
	Empty string

	rows [][]string
}

// cellReplacer makes arbitrary text safe for one tabwriter cell. tabwriter
// ends a cell at \t and \v and a line at \n and \f, whatever its flags, so
// each of those becomes a space.
var cellReplacer = strings.NewReplacer("\t", " ", "\v", " ", "\r", "", "\n", " ", "\f", " ")

// AddRow appends one row, a cell per column.
func (t *Table) AddRow(cells ...string) {
	row := make([]string, len(cells))
	for i, c := range cells {
		row[i] = cellReplacer.Replace(c)
	}
	t.rows = append(t.rows, row)
}

// Render writes the table to b. It takes a bufio.Writer, not any io.Writer,
// because it does not report a failed write: b keeps it for its Flush, which
// the caller checks (cmd/cliout's WriteText and Text do).
func (t *Table) Render(b *bufio.Writer) {
	if len(t.rows) == 0 && t.Empty != "" {
		fmt.Fprintln(b, t.Empty)
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
