package ansi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestAnsi(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestShouldUseColors() {
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
		s.Run(tt.name, func() {
			os.Setenv(cliColorForce, tt.cliColorForce)
			s.Equal(tt.want, shouldUseColors())
		})
	}
}
