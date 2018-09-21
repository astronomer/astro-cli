package printutil

import (
	"fmt"
)

type Table struct {
	Padding      []int
	Header       []string
	Rows         [][]string
	Length       int
	RenderedRows []string
	ColorCond    func()
}

func (t Table) PrintAll() error {
	t.PrintHeader()
	t.PrintRows()
	return nil
}

func (t Table) PrintHeader() {
	h := fmt.Sprintf(t.BuildPadding(), t.Header)

	fmt.Println(h)
}

func (t Table) PrintRows() {
	t.BuildRows()
	for _, r := range t.RenderedRows {
		fmt.Println(r)
	}
}

func (t Table) BuildRows() {
	p := t.BuildPadding()

	for _, r := range t.Rows {
		t.RenderedRows = append(t.RenderedRows, fmt.Sprintf(p, r))
	}

}

func (t Table) BuildPadding() string {
	padStr := ""
	for _, x := range t.Padding {
		temp := "%-%ss"
		padStr += fmt.Sprintf(temp, x)
	}

	return padStr
}
