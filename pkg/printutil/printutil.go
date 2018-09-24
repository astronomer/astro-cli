package printutil

import (
	"fmt"
)

type Table struct {
	// A slice of ints defining the padding for each column
	Padding         []int
	RenderedPadding string

	// Slice of strings representing column headers
	Header         []string
	RenderedHeader string

	// Truncate rows if they exceed padding length
	Truncate bool
	// 2d slice of strings, with each slice containing a row
	// representation (nested slice of strings) of info to print
	// Rows [][]string

	// A place to store rows with padding correctly applied to them
	Rows []Row

	// A message to print after table has been printed
	SuccessMsg string

	// Optional message to print if no rows were passed to table
	NoResultsMsg string

	// Highlight a row with color

	// Function which will eval whether to apply color to a row
	ColorRowCode [2]string
	// Function which will eval whether to apply color to a col
	// ColorColCond    func(t *Table) (bool, error)
	// ColorColCode [2]string
}

type Row struct {
	Raw      []string
	Rendered string
	Colored  bool
}

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

func (t *Table) PrintHeader() {
	if len(t.RenderedPadding) == 0 {
		p := t.GetPadding()
		t.RenderedPadding = p
	}

	header := strSliceToInterSlice(t.Header)
	t.RenderedHeader = fmt.Sprintf(t.RenderedPadding, header...)

	fmt.Println(t.RenderedHeader)
}

func (t *Table) PrintRows() {
	for _, r := range t.Rows {
		if r.Colored && len(t.ColorRowCode) == 2 {
			fmt.Println(t.ColorRowCode[0] + r.Rendered + t.ColorRowCode[1])
		} else {
			fmt.Println(r.Rendered)
		}
	}
}

func (t *Table) GetPadding() string {
	padStr := " "
	for _, x := range t.Padding {
		temp := "%ds"
		padStr += "%-" + fmt.Sprintf(temp, x)
	}

	return padStr
}

func strSliceToInterSlice(ss []string) []interface{} {
	is := make([]interface{}, len(ss))
	for i, v := range ss {
		is[i] = v
	}
	return is
}
