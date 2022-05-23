package git

import (
	"testing"
)

func TestIsGitRepository(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{
			name: "basic case",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGitRepository(); got != tt.want {
				t.Errorf("IsGitRepository() = %v, want %v", got, tt.want)
			}
		})
	}
}
