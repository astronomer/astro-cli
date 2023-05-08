package ansi

import (
	"errors"
)

var errMock = errors.New("mock error")

func (s *Suite) TestSpinner() {
	type args struct {
		text string
		fn   func() error
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "basic case",
			args:    args{text: "testing", fn: func() error { return nil }},
			wantErr: false,
		},
		{
			name:    "basic error case",
			args:    args{text: "testing", fn: func() error { return errMock }},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			if err := Spinner(tt.args.text, tt.args.fn); (err != nil) != tt.wantErr && !errors.Is(err, errMock) {
				s.FailNowf("Spinner()", "error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
