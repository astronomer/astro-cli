package printutil

import (
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/pkg/input"
)

// Table represents a table to be printed.
//
// It is the old way, inherited from Astro CLI 1.x. New code prints a list with
// cliout.Table (cmd/cliout/text.go), which lays out the same columns with
// text/tabwriter and writes through cliout.Text's bufio.Writer, so a renderer
// checks one error instead of none. The remaining uses move over a command
// family at a time. A numbered table a person picks a row of is pkg/picker's.
type Table struct {
	// A slice of ints defining the padding for each column
	Padding         []int
	RenderedPadding string

	// Slice of strings representing column headers
	Header         []string
	RenderedHeader string

	// Truncate rows if they exceed padding length
	Truncate bool

	// An array of row structs
	Rows []Row

	// A message to print after table has been printed
	SuccessMsg string

	// Optional message to print if no rows were passed to table
	NoResultsMsg string

	// Len 2 array with elements representing ColorCode and ColorTrm respectively
	ColorRowCode [2]string
	// Function which will eval whether to apply color to a col
	// ColorColCond    func(t *Table) (bool, error)
	// ColorColCode [2]string

	altPadding []int

	DynamicPadding bool
}

// Row represents a row to be printed
type Row struct {
	Raw      []string
	Rendered string
	Colored  bool
}

// AddRow is the preferred interface for adding a row to a table. Each value
// goes in as input.Cell has it: most are an API's (a Deployment's name, a
// token's description) and a terminal would act on an escape in one, and a
// line break would end the row. Cleaned here, before the padding is measured,
// so the columns line up on what is printed. The row's color (ColorRowCode)
// is the table's own and wraps the row after.
func (t *Table) AddRow(values []string, color bool) {
	cells := make([]string, len(values))
	for i, v := range values {
		cells[i] = input.Cell(v)
	}
	values = cells
	if t.DynamicPadding {
		t.dynamicPadding(Row{Raw: values, Colored: color})
	} else {
		t.altPadding = t.Padding
	}

	r := Row{
		Raw:     values,
		Colored: color,
	}

	t.Rows = append(t.Rows, r)
}

// Print header __as well as__ rows.
//
// To a terminal the columns are padded, as they have always been. To anything
// else (a pipe, a file) a script is reading, so the layout is plain: one tab
// between cells (see printPlain), and the messages go to stderr.
func (t *Table) Print(out io.Writer) error {
	plain := !input.IsTerminalWriter(out)
	// The messages are for a person. To a terminal they follow the table as
	// they always have; to a pipe they go to stderr, and stdout is the table.
	msgTo := out
	if plain {
		msgTo = input.NoticesFor(out)
	}
	if len(t.Rows) == 0 && t.NoResultsMsg != "" {
		fmt.Fprintln(msgTo, t.NoResultsMsg)
		if plain {
			fmt.Fprintln(out, strings.Join(t.Header, "\t"))
		}
		return nil
	}

	if plain {
		t.printPlain(out)
	} else {
		t.PrintHeader(out)
		t.PrintRows(out)
	}

	if t.SuccessMsg != "" {
		fmt.Fprintln(msgTo, t.SuccessMsg)
	}
	return nil
}

// printPlain is the table for a script: the header and each row as its cells
// joined by one tab, with no padding, no leading space, no truncation and no
// color.
func (t *Table) printPlain(out io.Writer) {
	fmt.Fprintln(out, strings.Join(t.Header, "\t"))
	for _, r := range t.Rows {
		fmt.Fprintln(out, strings.Join(r.Raw, "\t"))
	}
}

// PrintHeader prints header
func (t *Table) PrintHeader(out io.Writer) {
	if t.DynamicPadding {
		t.dynamicPadding(Row{Raw: t.Header, Colored: false})
	} else {
		t.altPadding = t.Padding
	}

	p := getPadding(t.altPadding)

	header := strSliceToInterSlice(t.Header)
	t.RenderedHeader = fmt.Sprintf(p, header...)

	fmt.Fprintln(out, t.RenderedHeader)
}

// PrintRows prints rows, a colored one in ColorRowCode.
func (t *Table) PrintRows(out io.Writer) {
	if t.RenderedPadding == "" {
		p := getPadding(t.altPadding)
		t.RenderedPadding = p
	}

	for _, r := range t.Rows {
		ri := strSliceToInterSlice(r.Raw)
		rr := fmt.Sprintf(t.RenderedPadding, ri...)
		if r.Colored && len(t.ColorRowCode) == 2 {
			fmt.Fprintln(out, t.ColorRowCode[0]+rr+t.ColorRowCode[1])
		} else {
			fmt.Fprintln(out, rr)
		}
	}
}

// GetPadding converts an array of ints into template padding for str fmting
func getPadding(padding []int) string {
	padStr := " "
	for _, x := range padding {
		temp := "%ds"
		padStr += "%-" + fmt.Sprintf(temp, x)
	}
	return padStr
}

// strSliceToInterface is a necessary conversion for passing
// a series of strings as a variadic parameter to fmt.Sprintf()
// ex
// fmt.Sprintf(temp, strSliceToInterface(sliceofStrings)...)
func strSliceToInterSlice(ss []string) []interface{} {
	is := make([]interface{}, len(ss))
	for i, v := range ss {
		is[i] = v
	}
	return is
}

// If the altPadding slice is smaller than the number of incoming Values
// slice, then that means that values still need to be added to fill it up.
//
// If the altPadding slice length is equivalent to the Values slice length,
// then it should compare the length of the incoming value being evaluated
// to see if it longer than the value already stored in it's place in
// altPadding. If it is, replace it's value with the length of the
// incoming value.
//
// This helps ensure as it iterates through each row that the length
// of the longest value for each column is kept or replaced as new
// values are introduced.
func (t *Table) dynamicPadding(row Row) {
	for i, col := range row.Raw {
		colLength := len(col) + 5
		if len(t.altPadding) < len(row.Raw) {
			t.altPadding = append(t.altPadding, colLength)
		} else if t.altPadding[i] < colLength {
			t.altPadding[i] = colLength
		}
	}
}
