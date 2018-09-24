package printutil

import (
	"fmt"
)

// Table represents a table to be printed
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
}

// Row represents a row to be printed
type Row struct {
	Raw      []string
	Rendered string
	Colored  bool
}

// AddRow is the preferred interface for adding a row to a table
func (t *Table) AddRow(row []string, color bool) {
	if len(t.RenderedPadding) == 0 {
		p := t.GetPadding()
		t.RenderedPadding = p
	}

	ri := strSliceToInterSlice(row)

	rr := fmt.Sprintf(t.RenderedPadding, ri...)

	r := Row{
		Raw:      row,
		Rendered: rr,
		Colored:  color,
	}

	t.Rows = append(t.Rows, r)
}

// Print header __as well as__ rows
func (t *Table) Print() error {
	if len(t.Rows) == 0 && len(t.NoResultsMsg) != 0 {
		fmt.Println(t.NoResultsMsg)
		return nil
	}

	t.PrintHeader()
	t.PrintRows()

	if len(t.SuccessMsg) != 0 {
		fmt.Println(t.SuccessMsg)
	}
	return nil
}

// PrintHeader prints header
func (t *Table) PrintHeader() {
	if len(t.RenderedPadding) == 0 {
		p := t.GetPadding()
		t.RenderedPadding = p
	}

	header := strSliceToInterSlice(t.Header)
	t.RenderedHeader = fmt.Sprintf(t.RenderedPadding, header...)

	fmt.Println(t.RenderedHeader)
}

// PrintRows prints rows with an "S"
func (t *Table) PrintRows() {
	for _, r := range t.Rows {
		if r.Colored && len(t.ColorRowCode) == 2 {
			fmt.Println(t.ColorRowCode[0] + r.Rendered + t.ColorRowCode[1])
		} else {
			fmt.Println(r.Rendered)
		}
	}
}

// GetPadding converts an array of ints into template padding for str fmting
func (t *Table) GetPadding() string {
	padStr := " "
	for _, x := range t.Padding {
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
