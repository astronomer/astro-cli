package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, tt.want, IsGitRepository())
		})
	}
}
