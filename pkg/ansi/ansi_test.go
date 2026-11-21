package ansi

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/pflag"
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
// ForceColors, and over FORCE_COLOR and CLICOLOR_FORCE.
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
	s.Equal("x", ForStderr().Red("x"), "NO_COLOR wins over CLICOLOR_FORCE")

	s.T().Setenv(forceColor, "1")
	s.Equal("x", ForStderr().Red("x"), "NO_COLOR wins over FORCE_COLOR")
}

// The precedence, highest first: --no-color, NO_COLOR, FORCE_COLOR and
// CLICOLOR_FORCE, CLICOLOR=0, then the stream, which TERM=dumb is not.
func TestShouldColorPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		terminal bool
		want     bool
	}{
		{"terminal", nil, true, true},
		{"pipe", nil, false, false},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, true, false},
		{"FORCE_COLOR on a pipe", map[string]string{"FORCE_COLOR": "1"}, false, true},
		{"FORCE_COLOR=0 on a terminal", map[string]string{"FORCE_COLOR": "0"}, true, false},
		{"FORCE_COLOR on a dumb terminal", map[string]string{"FORCE_COLOR": "1", "TERM": "dumb"}, true, true},
		{"CLICOLOR_FORCE on a pipe", map[string]string{"CLICOLOR_FORCE": "1"}, false, true},
		{"CLICOLOR_FORCE=0 on a terminal", map[string]string{"CLICOLOR_FORCE": "0"}, true, false},
		{"CLICOLOR=0 on a terminal", map[string]string{"CLICOLOR": "0"}, true, false},
		{"CLICOLOR_FORCE over CLICOLOR=0", map[string]string{"CLICOLOR_FORCE": "1", "CLICOLOR": "0"}, false, true},
		{"NO_COLOR on a terminal", map[string]string{"NO_COLOR": "1"}, true, false},
		{"empty NO_COLOR is unset", map[string]string{"NO_COLOR": ""}, true, true},
		{"NO_COLOR over CLICOLOR_FORCE", map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}, false, false},
		{"NO_COLOR over FORCE_COLOR", map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range []string{cliColorForce, forceColor, "CLICOLOR", "NO_COLOR", "TERM"} {
				t.Setenv(v, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := shouldColor(func() bool { return tt.terminal }); got != tt.want {
				t.Errorf("shouldColor = %v, want %v", got, tt.want)
			}
		})
	}
}

// --no-color beats every environment variable and the terminal, and applies as
// soon as the flag is parsed.
func TestNoColorFlagBeatsTheEnvironment(t *testing.T) {
	t.Setenv(forceColor, "1")
	t.Setenv(cliColorForce, "1")
	prev := noColorFlag
	t.Cleanup(func() { noColorFlag = prev; color = Color() })
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	AddNoColorFlag(fs)
	if !shouldColor(func() bool { return true }) {
		t.Fatal("FORCE_COLOR should color before the flag")
	}
	if err := fs.Parse([]string{"--no-color"}); err != nil {
		t.Fatal(err)
	}
	if shouldColor(func() bool { return true }) {
		t.Error("--no-color did not turn color off")
	}
	if Bold("x") != "x" {
		t.Errorf("Bold after --no-color = %q", Bold("x"))
	}
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

// A warning's or a note's message is text: what a terminal would act on is
// dropped, its line breaks are kept.
func (s *Suite) TestLabelMessagesTakeTextAsText() {
	for name, label := range map[string]func(io.Writer, string){"Warn": Warn, "Note": Note} {
		var b bytes.Buffer
		label(&b, "build yanked\x1b]52;c;aGk=\x07\x1b[2J\nsee the notes")
		got := b.String()
		s.Contains(got, "build yanked", name)
		s.Contains(got, "\nsee the notes", "%s lost the line break", name)
		s.NotContains(got, "]52;", "%s let a clipboard write through", name)
		s.NotContains(got, "\x1b[2J", "%s let a screen clear through", name)
	}
}
