package ansi

import (
	"errors"
	"testing"
)

var errMock = errors.New("mock error")

func TestSpinner(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			if err := Spinner(tt.args.text, tt.args.fn); (err != nil) != tt.wantErr && !errors.Is(err, errMock) {
				t.Errorf("Spinner() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
