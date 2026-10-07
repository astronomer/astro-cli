package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestZshCompletionHelpNamesFpath() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("completion", "zsh", "--help")
	s.NoError(err)
	for _, want := range []string{
		"astro completion zsh > $(brew --prefix)/share/zsh/site-functions/_astro",
		"fpath=(/opt/homebrew/share/zsh/site-functions $fpath)",
		`rm -f "${ZDOTDIR:-$HOME}"/.zcompdump*; exec zsh`,
	} {
		s.Contains(output, want)
	}
}

// The zsh help is replaced where help reads it, not by building cobra's
// completion commands early, which would list completion before help.
func (s *CmdSuite) TestRootMenuListsHelpBeforeCompletion() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("--help")
	s.NoError(err)
	help, completion := strings.Index(output, "\n  help "), strings.Index(output, "\n  completion ")
	s.Positive(help, output)
	s.Greater(completion, help, output)
}

func TestZshCompletionLongIsWrappedByHelp(t *testing.T) {
	if problem := checkLongUnwrapped(&cobra.Command{Long: zshCompletionLong("astro")}); problem != "" {
		t.Error(problem)
	}
}
