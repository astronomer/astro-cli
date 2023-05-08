package printutil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestPkgPrintUtilSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestTableAddRow() {
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
		s.Run(tt.name, func() {
			tr := &Table{}
			tr.AddRow(tt.args.values, tt.args.color)
			s.Contains(tr.Rows[0].Raw[0], tt.args.values[0])
		})
	}
}

func (s *Suite) TestTablePrint() {
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
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
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
				s.Errorf(err, "Table.Print() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			s.Contains(out.String(), tt.wantOut)
		})
	}
}

func (s *Suite) TestTablePrintWithIndex() {
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
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
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
			if err := tr.PrintWithPageNumber(10, out); (err != nil) != tt.wantErr {
				s.Errorf(err, "Table.PrintWithPageNumber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			s.Contains(out.String(), tt.wantOut)
		})
	}
}

func (s *Suite) TestTablePrintHeader() {
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
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
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
			s.Contains(out.String(), tt.wantOut)
		})
	}
}

func (s *Suite) TestTablePrintRows() {
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
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
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
			tr.PrintRows(out, 0)
			s.Contains(out.String(), tt.wantOut)
		})
	}
}

func (s *Suite) TestGetPadding() {
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
		s.Run(tt.name, func() {
			s.Contains(getPadding(tt.args.padding), tt.want)
		})
	}
}

func (s *Suite) TestStrSliceToInterSlice() {
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
		s.Run(tt.name, func() {
			s.Equal(strSliceToInterSlice(tt.args.ss), tt.want)
		})
	}
}

func (s *Suite) TestTableDynamicPadding() {
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
		s.Run(tt.name, func() {
			tr := &Table{
				altPadding: tt.fields.altPadding,
			}
			tr.dynamicPadding(tt.args.row)
			s.ElementsMatch(tr.altPadding, tt.wantAltPadding)
		})
	}
}
