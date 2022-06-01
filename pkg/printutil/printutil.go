package printutil

import (
	"errors"
	"fmt"
	"github.com/astronomer/astro-cli/pkg/input"
	"io"
	"os"
	"sort"
	"strconv"
)

var errInvalidTableSelection = errors.New("invalid selection from table")

type Comparer[T any] interface {
	Before(other T) bool
}

type TableT[T Comparer[T]] struct {
	ExtractRowFn func(element T, index int) []string

	Contents []T

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

	SelectionPromptMsg string

	// Len 2 array with elements representing ColorCode and ColorTrm respectively
	ColorRowCode [2]string
	// Function which will eval whether to apply color to a col
	// ColorColCond    func(t *Table) (bool, error)
	// ColorColCode [2]string

	altPadding []int

	DynamicPadding bool
}

func (t TableT[T]) Select() (T, error) {
	var zeroT T
	if len(t.Contents) == 0 {
		return zeroT, errors.New(t.NoResultsMsg)
	}
	fmt.Println(t.SelectionPromptMsg)
	sort.Slice(t.Contents, func(i, j int) bool {
		return t.Contents[i].Before(t.Contents[j])
	})
	m := make(map[string]T)
	for i := range t.Contents {
		m[strconv.Itoa(i+1)] = t.Contents[i]
	}
	t.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := m[choice]
	if !ok {
		return zeroT, errInvalidTableSelection
	}
	return selected, nil
}

func (t TableT[T]) WithContents(c []T, extractContentsFn func(elem T, index int) []string, colorFn func(w T) (bool, error)) TableT[T] {
	t.Contents = c
	for i, elem := range c {
		// calculate color if needed
		var color bool
		if colorFn != nil {
			var err error
			color, err = colorFn(elem)
			if err != nil {
				color = false
			}
		} else {
			color = false
		}

		// add rows to table
		if t.ExtractRowFn != nil {
			t.AddRow(t.ExtractRowFn(elem, i), color)
		} else {
			t.AddRow(extractContentsFn(elem, i), color)
		}
	}
	return t
}

// AddRow is the preferred interface for adding a row to a table
func (t *TableT[T]) AddRow(values []string, color bool) {
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

// Print header __as well as__ rows
func (t *TableT[T]) Print(out io.Writer) error {
	if len(t.Rows) == 0 && t.NoResultsMsg != "" {
		fmt.Fprintln(out, t.NoResultsMsg)
		return nil
	}

	t.PrintHeader(out)
	t.PrintRows(out)

	if t.SuccessMsg != "" {
		fmt.Fprintln(out, t.SuccessMsg)
	}
	return nil
}

// PrintHeader prints header
func (t *TableT[T]) PrintHeader(out io.Writer) {
	if t.DynamicPadding {
		t.dynamicPadding(Row{Raw: t.Header, Colored: false})
	} else {
		t.altPadding = t.Padding
	}

	p := getPadding(t.altPadding)

	headerSelectPrefix := ""
	if t.GetUserInput {
		headerSelectPrefix = fmt.Sprintf("%-5s", "#")
	}

	header := strSliceToInterSlice(t.Header)
	t.RenderedHeader = fmt.Sprintf(p, header...)

	fmt.Fprintln(out, headerSelectPrefix+t.RenderedHeader)
}

// PrintRows prints rows with an "S"
func (t *TableT[T]) PrintRows(out io.Writer) {
	if t.RenderedPadding == "" {
		p := getPadding(t.altPadding)
		t.RenderedPadding = p
	}

	for i, r := range t.Rows {
		ri := strSliceToInterSlice(r.Raw)
		rr := fmt.Sprintf(t.RenderedPadding, ri...)

		// Responsible for adding the int in front of a row for selection by user
		rowSelectPrefix := ""
		if t.GetUserInput {
			rowSelectPrefix = fmt.Sprintf("%-5s", strconv.Itoa(i+1))
		}
		if r.Colored && len(t.ColorRowCode) == 2 {
			fmt.Fprintln(out, rowSelectPrefix+t.ColorRowCode[0]+rr+t.ColorRowCode[1])
		} else {
			fmt.Fprintln(out, rowSelectPrefix+rr)
		}
	}
}

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

	DynamicPadding bool
}

// Row represents a row to be printed
type Row struct {
	Raw      []string
	Rendered string
	Colored  bool
}

// AddRow is the preferred interface for adding a row to a table
func (t *Table) AddRow(values []string, color bool) {
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

// Print header __as well as__ rows
func (t *Table) Print(out io.Writer) error {
	if len(t.Rows) == 0 && t.NoResultsMsg != "" {
		fmt.Fprintln(out, t.NoResultsMsg)
		return nil
	}

	t.PrintHeader(out)
	t.PrintRows(out)

	if t.SuccessMsg != "" {
		fmt.Fprintln(out, t.SuccessMsg)
	}
	return nil
}

// PrintHeader prints header
func (t *Table) PrintHeader(out io.Writer) {
	if t.DynamicPadding {
		t.dynamicPadding(Row{Raw: t.Header, Colored: false})
	} else {
		t.altPadding = t.Padding
	}

	p := getPadding(t.altPadding)

	headerSelectPrefix := ""
	if t.GetUserInput {
		headerSelectPrefix = fmt.Sprintf("%-5s", "#")
	}

	header := strSliceToInterSlice(t.Header)
	t.RenderedHeader = fmt.Sprintf(p, header...)

	fmt.Fprintln(out, headerSelectPrefix+t.RenderedHeader)
}

// PrintRows prints rows with an "S"
func (t *Table) PrintRows(out io.Writer) {
	if t.RenderedPadding == "" {
		p := getPadding(t.altPadding)
		t.RenderedPadding = p
	}

	for i, r := range t.Rows {
		ri := strSliceToInterSlice(r.Raw)
		rr := fmt.Sprintf(t.RenderedPadding, ri...)

		// Responsible for adding the int in front of a row for selection by user
		rowSelectPrefix := ""
		if t.GetUserInput {
			rowSelectPrefix = fmt.Sprintf("%-5s", strconv.Itoa(i+1))
		}
		if r.Colored && len(t.ColorRowCode) == 2 {
			fmt.Fprintln(out, rowSelectPrefix+t.ColorRowCode[0]+rr+t.ColorRowCode[1])
		} else {
			fmt.Fprintln(out, rowSelectPrefix+rr)
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
//  to see if it longer than the value already stored in it's place in
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

func (t *TableT[T]) dynamicPadding(row Row) {
	for i, col := range row.Raw {
		colLength := len(col) + 5
		if len(t.altPadding) < len(row.Raw) {
			t.altPadding = append(t.altPadding, colLength)
		} else if t.altPadding[i] < colLength {
			t.altPadding[i] = colLength
		}
	}
}
