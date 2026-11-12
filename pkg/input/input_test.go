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

			got, err := Text(tt.args.promptText)
			s.NoError(err)
			s.Equal(tt.want, got)

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
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
			// Nobody answering is not a no: it is a question this run could
			// not get answered.
			name:        "no input",
			inputString: "",
			args:        args{promptText: "enter y or n"},
			want:        false,
			errAssertion: func(t assert.TestingT, err error, _ ...interface{}) bool {
				return assert.True(t, IsRequired(err), "%v", err)
			},
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
		// A hidden answer is never read from a stdin that is not a terminal,
		// piped or ended: it would be echoed by whatever types at the far end,
		// or taken from a pipe not meant as a password. It is input_required,
		// not the terminal error of taking a pipe for a terminal.
		{
			name:        "a pipe is refused, not read",
			inputString: "hunter2\n",
			args:        args{"enter pass"},
			want:        "",
			errAssertion: func(t assert.TestingT, err error, _ ...interface{}) bool {
				return assert.True(t, IsRequired(err), "%v", err) && assert.NotContains(t, err.Error(), "ioctl")
			},
		},
		{
			name:        "ended input is refused too",
			inputString: "",
			args:        args{"enter pass"},
			want:        "",
			errAssertion: func(t assert.TestingT, err error, _ ...interface{}) bool {
				return assert.True(t, IsRequired(err), "%v", err)
			},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			// No terminal on stdin: a hidden answer is refused, not read.
			defer SetTerminals(false, true)()
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
