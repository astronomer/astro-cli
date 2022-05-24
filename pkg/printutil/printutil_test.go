package printutil

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
