package ansi

import (
	"os"
	"testing"
)

func TestShouldUseColors(t *testing.T) {
	tests := []struct {
		name          string
		cliColorForce string
		want          bool
	}{
		{
			name:          "basic true case",
			cliColorForce: "1",
			want:          true,
		},
		{
			name:          "basic false case",
			cliColorForce: "0",
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(cliColorForce, tt.cliColorForce)
			if got := shouldUseColors(); got != tt.want {
				t.Errorf("shouldUseColors() = %v, want %v", got, tt.want)
			}
		})
	}
}
