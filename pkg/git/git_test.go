package git

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestPkgGitSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestIsGitRepository() {
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
		s.Run(tt.name, func() {
			s.Equal(IsGitRepository(), tt.want)
		})
	}
}
