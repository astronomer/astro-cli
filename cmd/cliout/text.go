package cliout

// The text half of output: how a renderer writes lines without checking every
// one, and the table the CLI's lists print. Both are the standard library
// (bufio, text/tabwriter); there is no table package to learn.

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteText runs render over a buffered writer on w and returns the one error
// that matters: whether the text reached w.
//
// A bufio.Writer's error is sticky. Once a write to w fails, every later write
// does nothing and the failure is kept until Flush returns it. So render writes
// its lines with fmt.Fprintf and fmt.Fprintln and checks none of them, and the
// check happens once, here, instead of after every line:
//
//	return cliout.WriteText(w, func(b *bufio.Writer) {
//		fmt.Fprintf(b, "[%s]\n", name)
//		for _, o := range options {
//			fmt.Fprintln(b, o.Key+" = "+o.Value)
//		}
//	})
//
// render returns nothing because nothing it does can fail: a renderer formats
// values it was handed. Work that can fail happens before the render.
func WriteText(w io.Writer, render func(b *bufio.Writer)) error {
	b := bufio.NewWriter(w)
	render(b)
	return b.Flush()
}

// Text is WriteText shaped as Emit's text renderer:
//
//	return r.Emit(result, cliout.Text(func(b *bufio.Writer) { ... }))
func Text(render func(b *bufio.Writer)) func(io.Writer) error {
	return func(w io.Writer) error { return WriteText(w, render) }
}

// tableGap is the space after each column's widest cell.
const tableGap = 5

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
// WriteText (or Text) checks. Pass it straight to Text when the table is all
// a renderer prints: r.Emit(v, cliout.Text(table.Render)).
func (t *Table) Render(b *bufio.Writer) {
	if len(t.rows) == 0 && t.Empty != "" {
		fmt.Fprintln(b, t.Empty)
		return
	}
	// Every cell, the last included, ends in a tab, so every column is padded
	// to its width: the old tables padded the last column too, and a line
	// that is byte-for-byte what it was is the point.
	tw := tabwriter.NewWriter(b, 0, 0, tableGap, ' ', 0)
	writeTableLine(tw, t.Header)
	for _, row := range t.rows {
		writeTableLine(tw, row)
	}
	tw.Flush() //nolint:errcheck // its only error is b's, which is sticky: the caller's Flush reports it
}

func writeTableLine(tw *tabwriter.Writer, cells []string) {
	fmt.Fprintln(tw, " "+strings.Join(cells, "\t")+"\t")
}
