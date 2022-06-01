package printutil

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// duplicate to avoid circular import
func mockUserInput(t *testing.T, i string) func() {
	input := []byte(i)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()

	// set os.Stdin = new stdin, and return function to defer in the test
	realStdin := os.Stdin
	os.Stdin = r
	return func() { os.Stdin = realStdin }
}

type test struct {
	Name   string
	Num    int
	Ignore bool
}

func (t test) Before(other test) bool {
	return t.Num < other.Num
}

var collection = []test{
	{Name: "test", Num: 0, Ignore: true},
	{Name: "test1", Num: 1, Ignore: true},
	{Name: "test2", Num: 2, Ignore: true},
}

func TestWithContents(t *testing.T) {
	t.Run("test table provided extract function", func(t *testing.T) {
		tab := TableT[test]{
			Padding: []int{10, 10, 10},
			Header:  []string{"#", "name", "num"},
			ExtractRowFn: func(t test, i int) []string {
				return []string{strconv.Itoa(i + 1), t.Name, fmt.Sprintf("%d", t.Num)}
			},
		}.WithContents(collection, nil, func(w test) (bool, error) {
			return false, nil
		})
		assert.Equal(t, collection, tab.Contents)
		expectedRows := []Row{
			{Raw: []string{"1", "test", "0"}, Rendered: "", Colored: false},
			{Raw: []string{"2", "test1", "1"}, Rendered: "", Colored: false},
			{Raw: []string{"3", "test2", "2"}, Rendered: "", Colored: false},
		}
		assert.Equal(t, expectedRows, tab.Rows)
	})

	t.Run("test function provided extract function", func(t *testing.T) {
		tab := TableT[test]{
			Padding: []int{10, 10, 10},
			Header:  []string{"#", "name", "num"},
		}.WithContents(collection, func(t test, i int) []string {
			return []string{strconv.Itoa(i + 1), t.Name, fmt.Sprintf("%d", t.Num)}
		}, func(w test) (bool, error) {
			return false, nil
		})
		assert.Equal(t, collection, tab.Contents)
		expectedRows := []Row{
			{Raw: []string{"1", "test", "0"}, Rendered: "", Colored: false},
			{Raw: []string{"2", "test1", "1"}, Rendered: "", Colored: false},
			{Raw: []string{"3", "test2", "2"}, Rendered: "", Colored: false},
		}
		assert.Equal(t, expectedRows, tab.Rows)
	})
}

func TestTableTSelect(t *testing.T) {
	tab := TableT[test]{
		Padding: []int{10, 10, 10},
		Header:  []string{"#", "name", "num"},
		ExtractRowFn: func(t test, i int) []string {
			return []string{strconv.Itoa(i + 1), t.Name, fmt.Sprintf("%d", t.Num)}
		},
	}.WithContents(collection, nil, func(w test) (bool, error) {
		return false, nil
	})

	t.Run("test generic select", func(t *testing.T) {
		defer mockUserInput(t, "1")()
		actual, err := tab.Select()
		assert.NoError(t, err)
		assert.Equal(t, collection[0], actual)
	})

	t.Run("select empty contents", func(t *testing.T) {
		emptyTable := TableT[test]{
			Padding: []int{10, 10, 10},
			Header:  []string{"#", "name", "num"},
			ExtractRowFn: func(t test, i int) []string {
				return []string{strconv.Itoa(i + 1), t.Name, fmt.Sprintf("%d", t.Num)}
			},
		}
		_, err := emptyTable.Select()
		assert.Error(t, err)
	})

	t.Run("bad choice out of range", func(t *testing.T) {
		defer mockUserInput(t, "5")()
		_, err := tab.Select()
		assert.Error(t, err)
	})

	t.Run("bad choice negative", func(t *testing.T) {
		defer mockUserInput(t, "-1")()
		_, err := tab.Select()
		assert.Error(t, err)
	})

	t.Run("bad choice random string", func(t *testing.T) {
		defer mockUserInput(t, "asdf")()
		_, err := tab.Select()
		assert.Error(t, err)
	})
}

func TestTableAddRow(t *testing.T) {
	type args struct {
		values []string
		color  bool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "basic case",
			args: args{values: []string{"testing"}, color: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Table{}
			tr.AddRow(tt.args.values, tt.args.color)
			assert.Contains(t, tr.Rows[0].Raw[0], tt.args.values[0])
		})
	}
}

func TestTablePrint(t *testing.T) {
	type fields struct {
		Padding         []int
		RenderedPadding string
		Header          []string
		RenderedHeader  string
		Truncate        bool
		Rows            []Row
		GetUserInput    bool
		SuccessMsg      string
		NoResultsMsg    string
		ColorRowCode    [2]string
		altPadding      []int
		DynamicPadding  bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantOut string
		wantErr bool
	}{
		{
			name:    "empty table case",
			fields:  fields{NoResultsMsg: "no rows present"},
			wantOut: "no rows present",
			wantErr: false,
		},
		{
			name:    "basic case",
			fields:  fields{SuccessMsg: "printed all rows", Rows: []Row{{Raw: []string{"testing"}}}},
			wantOut: "printed all rows",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Table{
				Padding:         tt.fields.Padding,
				RenderedPadding: tt.fields.RenderedPadding,
				Header:          tt.fields.Header,
				RenderedHeader:  tt.fields.RenderedHeader,
				Truncate:        tt.fields.Truncate,
				Rows:            tt.fields.Rows,
				GetUserInput:    tt.fields.GetUserInput,
				SuccessMsg:      tt.fields.SuccessMsg,
				NoResultsMsg:    tt.fields.NoResultsMsg,
				ColorRowCode:    tt.fields.ColorRowCode,
				altPadding:      tt.fields.altPadding,
				DynamicPadding:  tt.fields.DynamicPadding,
			}
			out := &bytes.Buffer{}
			if err := tr.Print(out); (err != nil) != tt.wantErr {
				t.Errorf("Table.Print() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOut := out.String(); !strings.Contains(gotOut, tt.wantOut) {
				t.Errorf("Table.Print() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func TestTablePrintHeader(t *testing.T) {
	type fields struct {
		Padding         []int
		RenderedPadding string
		Header          []string
		RenderedHeader  string
		Truncate        bool
		Rows            []Row
		GetUserInput    bool
		SuccessMsg      string
		NoResultsMsg    string
		ColorRowCode    [2]string
		altPadding      []int
		DynamicPadding  bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantOut string
	}{
		{
			name:    "basic case",
			fields:  fields{Header: []string{"testing"}, GetUserInput: true},
			wantOut: "testing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Table{
				Padding:         tt.fields.Padding,
				RenderedPadding: tt.fields.RenderedPadding,
				Header:          tt.fields.Header,
				RenderedHeader:  tt.fields.RenderedHeader,
				Truncate:        tt.fields.Truncate,
				Rows:            tt.fields.Rows,
				GetUserInput:    tt.fields.GetUserInput,
				SuccessMsg:      tt.fields.SuccessMsg,
				NoResultsMsg:    tt.fields.NoResultsMsg,
				ColorRowCode:    tt.fields.ColorRowCode,
				altPadding:      tt.fields.altPadding,
				DynamicPadding:  tt.fields.DynamicPadding,
			}
			out := &bytes.Buffer{}
			tr.PrintHeader(out)
			if gotOut := out.String(); !strings.Contains(gotOut, tt.wantOut) {
				t.Errorf("Table.PrintHeader() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func TestTablePrintRows(t *testing.T) {
	type fields struct {
		Padding         []int
		RenderedPadding string
		Header          []string
		RenderedHeader  string
		Truncate        bool
		Rows            []Row
		GetUserInput    bool
		SuccessMsg      string
		NoResultsMsg    string
		ColorRowCode    [2]string
		altPadding      []int
		DynamicPadding  bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantOut string
	}{
		{
			name:    "basic case",
			fields:  fields{Rows: []Row{{Raw: []string{"testing"}}}, GetUserInput: true},
			wantOut: "testing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Table{
				Padding:         tt.fields.Padding,
				RenderedPadding: tt.fields.RenderedPadding,
				Header:          tt.fields.Header,
				RenderedHeader:  tt.fields.RenderedHeader,
				Truncate:        tt.fields.Truncate,
				Rows:            tt.fields.Rows,
				GetUserInput:    tt.fields.GetUserInput,
				SuccessMsg:      tt.fields.SuccessMsg,
				NoResultsMsg:    tt.fields.NoResultsMsg,
				ColorRowCode:    tt.fields.ColorRowCode,
				altPadding:      tt.fields.altPadding,
				DynamicPadding:  tt.fields.DynamicPadding,
			}
			out := &bytes.Buffer{}
			tr.PrintRows(out)
			if gotOut := out.String(); !strings.Contains(gotOut, tt.wantOut) {
				t.Errorf("Table.PrintRows() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func TestGetPadding(t *testing.T) {
	type args struct {
		padding []int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic case",
			args: args{padding: []int{1, 2}},
			want: " %-1s%-2s",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPadding(tt.args.padding); got != tt.want {
				t.Errorf("getPadding() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStrSliceToInterSlice(t *testing.T) {
	type args struct {
		ss []string
	}
	tests := []struct {
		name string
		args args
		want []interface{}
	}{
		{
			name: "basic case",
			args: args{ss: []string{"test1", "test2"}},
			want: []interface{}{"test1", "test2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strSliceToInterSlice(tt.args.ss); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("strSliceToInterSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTableDynamicPadding(t *testing.T) {
	type fields struct {
		altPadding []int
	}
	type args struct {
		row Row
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		wantAltPadding []int
	}{
		{
			name:           "basic case",
			fields:         fields{altPadding: []int{1}},
			args:           args{row: Row{Raw: []string{"test"}}},
			wantAltPadding: []int{9},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Table{
				altPadding: tt.fields.altPadding,
			}
			tr.dynamicPadding(tt.args.row)
			assert.ElementsMatch(t, tr.altPadding, tt.wantAltPadding)
		})
	}
}
