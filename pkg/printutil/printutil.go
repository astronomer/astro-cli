package printutil

import (
	"fmt"
	"strconv"
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

	// RowSelectionPrompt puts a number in front of each row and prompts user for selection
	GetUserInput bool

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

	headerPadding []int

	DynamicPadding bool
}

// comment
type TempRow struct {
	Values []string
	Color  bool
}

// Row represents a row to be printed
type Row struct {
	Raw      []string
	Rendered string
	Colored  bool
}

// AddRow is the preferred interface for adding a row to a table
func (t *Table) AddRow(values []string, color bool) {

	if len(t.altPadding) == 0 {
		if t.DynamicPadding {
			rows := []TempRow{}
			rows = append(rows, TempRow{values, color})
			t.dynamicPadding(rows)
		} else {
			t.altPadding = t.Padding
		}
	}

	if len(t.RenderedPadding) == 0 {
		p := t.GetPadding(t.altPadding)
		t.RenderedPadding = p
	}

	ri := strSliceToInterSlice(values)
	rr := fmt.Sprintf(t.RenderedPadding, ri...)

	r := Row{
		Raw:      values,
		Rendered: rr,
		Colored:  color,
	}

	t.Rows = append(t.Rows, r)
}

// comment
func (t *Table) AddRows(rows []TempRow) {

	if t.DynamicPadding {
		t.dynamicPadding(rows)
	} else {
		t.altPadding = t.Padding
	}

	for _, row := range rows {
		t.AddRow(row.Values, row.Color)
	}
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
	t.createHeaderPadding()

	p := t.GetPadding(t.headerPadding)

	headerSelectPrefix := ""
	if t.GetUserInput {
		headerSelectPrefix = fmt.Sprintf("%-5s", "#")
	}

	header := strSliceToInterSlice(t.Header)
	t.RenderedHeader = fmt.Sprintf(p, header...)

	fmt.Println(headerSelectPrefix + t.RenderedHeader)
}

// PrintRows prints rows with an "S"
func (t *Table) PrintRows() {
	for i, r := range t.Rows {
		// Responsible for adding the int in front of a row for selection by user
		rowSelectPrefix := ""
		if t.GetUserInput {
			rowSelectPrefix = fmt.Sprintf("%-5s", strconv.Itoa(i+1))
		}
		if r.Colored && len(t.ColorRowCode) == 2 {
			fmt.Println(rowSelectPrefix + t.ColorRowCode[0] + r.Rendered + t.ColorRowCode[1])
		} else {
			fmt.Println(rowSelectPrefix + r.Rendered)
		}
	}
}

// GetPadding converts an array of ints into template padding for str fmting
func (t *Table) GetPadding(padding []int) string {
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

func (t *Table) createHeaderPadding() {
	if len(t.headerPadding) == 0 {
		for _, num := range t.Header {
			colLength := len(num) + 5
			t.headerPadding = append(t.headerPadding, colLength)
		}
	}
}

func (t *Table) coalesceHeaderAndRowPadding(val int, i int) {
	if val < t.headerPadding[i] {
		t.altPadding[i] = t.headerPadding[i]
	} else {
		t.headerPadding[i] = t.altPadding[i]
	}
}

func (t *Table) dynamicPadding(rows []TempRow) {
	for _, row := range rows {
		for i, col := range row.Values {
			colLength := len(col) + 5
			if len(t.altPadding) != len(row.Values) {
				t.altPadding = append(t.altPadding, colLength)
			} else {
				if t.altPadding[i] < colLength {
					t.altPadding[i] = colLength
				}
			}
		}
	}

	for i, val := range t.altPadding {
		t.createHeaderPadding()
		t.coalesceHeaderAndRowPadding(val, i)
	}
}
