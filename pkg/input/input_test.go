package input

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestInput(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestText() {
	type args struct {
		promptText string
	}
	tests := []struct {
		name        string
		args        args
		inputString string
		want        string
	}{
		{
			name:        "basic case",
			inputString: "testing",
			args:        args{promptText: "enter text input"},
			want:        "testing",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			// mock os.Stdin
			input := []byte(tt.inputString)
			r, w, err := os.Pipe()
			s.Require().NoError(err)
			_, err = w.Write(input)
			s.NoError(err)
			w.Close()
			stdin := os.Stdin
			os.Stdin = r

			s.Equal(tt.want, Text(tt.args.promptText))

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
	}
}

func (s *Suite) TestChoicePrompt() {
	tests := []struct {
		count, preselected int
		want               string
	}{
		{1, 0, "Choose 1: "},
		{1, 1, "Choose 1 [1]: "},
		{3, 0, "Choose 1-3: "},
		{3, 2, "Choose 1-3 [2]: "},
	}
	for _, tt := range tests {
		s.Equal(tt.want, ChoicePrompt(tt.count, tt.preselected))
	}
}

func (s *Suite) TestConfirm() {
	type args struct {
		promptText string
	}
	tests := []struct {
		name         string
		inputString  string
		args         args
		want         bool
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "no case",
			inputString:  "n",
			args:         args{promptText: "enter y or n"},
			want:         false,
			errAssertion: assert.NoError,
		},
		{
			name:         "yes case",
			inputString:  "y",
			args:         args{promptText: "enter y or n"},
			want:         true,
			errAssertion: assert.NoError,
		},
		{
			name:         "no input",
			inputString:  "",
			args:         args{promptText: "enter y or n"},
			want:         false,
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			// mock os.Stdin
			input := []byte(tt.inputString)
			r, w, err := os.Pipe()
			s.Require().NoError(err)
			_, err = w.Write(input)
			s.NoError(err)
			w.Close()
			stdin := os.Stdin
			os.Stdin = r

			got, err := Confirm(tt.args.promptText)
			if !tt.errAssertion(s.T(), err) {
				return
			}

			s.Equal(tt.want, got)

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
	}
}

func (s *Suite) TestPassword() {
	type args struct {
		promptText string
	}
	tests := []struct {
		name         string
		inputString  string
		args         args
		want         string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "unsupported error",
			inputString:  "",
			args:         args{"enter pass"},
			want:         "",
			errAssertion: assert.Error,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			// mock os.Stdin
			input := []byte(tt.inputString)
			r, w, err := os.Pipe()
			s.Require().NoError(err)
			_, err = w.Write(input)
			s.NoError(err)
			w.Close()
			stdin := os.Stdin
			os.Stdin = r

			got, err := Password(tt.args.promptText)
			if !tt.errAssertion(s.T(), err) {
				return
			}

			s.Equal(tt.want, got)

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
	}
}
