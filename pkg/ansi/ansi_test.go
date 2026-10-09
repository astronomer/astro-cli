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

// A question goes to stderr, so its colors follow stderr: colored at a
// terminal with stdout redirected, and plain on a redirected stderr.
func (s *Suite) TestForStderrFollowsStderrNotStdout() {
	for _, v := range []string{cliColorForce, "CLICOLOR", "NO_COLOR"} {
		s.T().Setenv(v, "")
		s.Require().NoError(os.Unsetenv(v))
	}
	redirected, err := os.CreateTemp(s.T().TempDir(), "stdout")
	s.Require().NoError(err)
	defer redirected.Close()
	previousOut, previousCheck := Output, isMessagesTerminal
	s.T().Cleanup(func() { Output, isMessagesTerminal = previousOut, previousCheck })
	Output = redirected
	s.Require().False(IsOutputTerminal(), "stdout is redirected")

	isMessagesTerminal = func() bool { return true }
	for _, colored := range []string{ForStderr().Bold("x"), ForStderr().Green("x"), ForStderr().Red("x"), ForStderr().Cyan("x")} {
		s.Contains(colored, "\x1b[", "stderr is a terminal: %q", colored)
	}

	isMessagesTerminal = func() bool { return false }
	for _, plain := range []string{ForStderr().Bold("x"), ForStderr().Green("x"), ForStderr().Red("x"), ForStderr().Cyan("x")} {
		s.Equal("x", plain, "stderr is redirected")
	}
}
