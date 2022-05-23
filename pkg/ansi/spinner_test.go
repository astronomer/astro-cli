package ansi

import (
	"errors"
	"testing"
)

func TestSpinner(t *testing.T) {
	mockError := errors.New("mock error")
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
			args:    args{text: "testing", fn: func() error { return mockError }},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Spinner(tt.args.text, tt.args.fn); (err != nil) != tt.wantErr && !errors.Is(err, mockError) {
				t.Errorf("Spinner() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
