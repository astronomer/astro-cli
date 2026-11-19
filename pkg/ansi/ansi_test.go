package ansi

import (
	"os"
	"testing"

	"github.com/logrusorgru/aurora"
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

// NO_COLOR, set and not empty, turns color off on a terminal and over
// ForceColors; only an explicit CLICOLOR_FORCE wins over it.
func (s *Suite) TestNoColorTurnsColorOff() {
	for _, v := range []string{cliColorForce, "CLICOLOR", "NO_COLOR"} {
		s.T().Setenv(v, "")
		s.Require().NoError(os.Unsetenv(v))
	}
	previousCheck, previousForce := isMessagesTerminal, ForceColors
	s.T().Cleanup(func() { isMessagesTerminal, ForceColors = previousCheck, previousForce })
	isMessagesTerminal = func() bool { return true }
	ForceColors = true
	s.Contains(ForStderr().Red("x"), "\x1b[", "a terminal with NO_COLOR unset is colored")

	s.T().Setenv("NO_COLOR", "1")
	s.Equal("x", ForStderr().Red("x"), "NO_COLOR turns color off")

	s.T().Setenv(cliColorForce, "1")
	s.Contains(ForStderr().Red("x"), "\x1b[", "CLICOLOR_FORCE wins over NO_COLOR")
}

// A styled name is text, not a format, and reaches the terminal without what
// it would act on, whether or not color is on. A line break stays, for a
// prompt that starts on a fresh line.
func (s *Suite) TestStylingTakesTextAsText() {
	for _, on := range []bool{false, true} {
		orig := color
		color = aurora.NewAurora(on)
		p := Palette{aurora.NewAurora(on)}
		for name, style := range map[string]func(string) string{
			"Bold": Bold, "Green": Green, "Cyan": Cyan,
			"Palette.Bold": p.Bold, "Palette.Red": p.Red, "Palette.Green": p.Green,
			"Palette.Cyan": p.Cyan, "Palette.Yellow": p.Yellow,
		} {
			got := style("50% off\x1b]52;c;aGk=\x07\x1b[2J")
			s.Contains(got, "50% off", "%s (color %v) read the text as a format", name, on)
			s.NotContains(got, "]52;", "%s (color %v) let a clipboard write through", name, on)
			s.NotContains(got, "\x1b[2J", "%s (color %v) let a screen clear through", name, on)
			s.Contains(style("\nTeam name: "), "\nTeam name: ", "%s (color %v) lost the line break", name, on)
		}
		color = orig
	}
}
