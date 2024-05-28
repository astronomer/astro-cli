package ansi

import (
	"errors"

	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("mock error")

func (s *Suite) TestSpinner() {
	type args struct {
		text string
		fn   func() error
	}
	tests := []struct {
		name         string
		args         args
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic case",
			args:         args{text: "testing", fn: func() error { return nil }},
			errAssertion: assert.NoError,
		},
		{
			name: "basic error case",
			args: args{text: "testing", fn: func() error { return errMock }},
			errAssertion: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.ErrorIs(t, err, errMock, msgAndArgs...)
			},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.errAssertion(s.T(), Spinner(tt.args.text, tt.args.fn))
		})
	}
}
