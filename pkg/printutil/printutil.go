package printutil

import (
	"fmt"
)

type Table struct {
	Padding      []int
	Header       []string
	Rows         [][]string
	RenderedRows []string
	// ColorCond    func() bool
	SuccessMsg   string
	NoResultsMsg string
}

func (t *Table) PrintAll() error {
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
	header := strSliceToInterSlice(t.Header)
	h := fmt.Sprintf(t.BuildPadding(), header...)

	fmt.Println(h)
}

func (t *Table) PrintRows() {
	t.BuildRows()
	for _, r := range t.RenderedRows {
		fmt.Println(r)
	}
}

func (t *Table) BuildRows() {
	// TODO This function could trim strings down to length of padding
	// at the index of the column
	p := t.BuildPadding()
	var rows []string
	for _, r := range t.Rows {
		row := strSliceToInterSlice(r)
		rows = append(rows, fmt.Sprintf(p, row...))
	}

	t.RenderedRows = rows
}

func (t *Table) BuildPadding() string {
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
