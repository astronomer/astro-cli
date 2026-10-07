package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// runContextCmd runs `astro context` with args the way main runs it.
func runContextCmd(args ...string) (stdout, stderr string, err error) {
	return runCommands(func(out io.Writer) []*cobra.Command {
		return []*cobra.Command{newContextCmd(nil, out)}
	}, append([]string{"context"}, args...)...)
}

// decodeOne decodes stdout as the one json value a run published: a second
// value, or anything else beside it, fails the decode.
func decodeOne[T any](s *CmdSuite, stdout string) T {
	s.T().Helper()
	var v T
	dec := json.NewDecoder(strings.NewReader(stdout))
	s.Require().NoError(dec.Decode(&v), stdout)
	s.Require().False(dec.More(), "more than one value on stdout: %s", stdout)
	return v
}

// savedContexts is astronomer.io, current, as the test config holds it, and
// astronomer-dev.io beside it.
func (s *CmdSuite) savedContexts() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	dev := config.Context{Domain: "astronomer-dev.io", UserEmail: "someone@example.com"}
	s.Require().NoError(dev.SetContext())
}

func (s *CmdSuite) TestContextListOutput() {
	s.Run("text is the table, the current context in green", func() {
		s.savedContexts()
		stdout, _, err := runContextCmd("list")
		s.NoError(err)
		lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
		s.Require().Len(lines, 3, stdout)
		s.Contains(lines[0], "NAME")
		s.Contains(lines[1], "astronomer-dev.io")
		s.NotContains(lines[1], "\033[1;32m")
		s.Contains(lines[2], "astronomer.io")
		s.True(strings.HasPrefix(lines[2], "\033[1;32m"), "current row is green: %q", lines[2])
	})

	s.Run("json lists every context under contexts, the current one marked", func() {
		s.savedContexts()
		stdout, stderr, err := runContextCmd("list", "-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		list := decodeOne[context.InfoList](s, stdout)
		s.Equal([]context.Info{
			{Domain: "astronomer-dev.io", UserEmail: "someone@example.com"},
			{Domain: "astronomer.io", OrganizationID: "test-org-id", WorkspaceID: "ck05r3bor07h40d02y2hw4n4v", IsCurrent: true},
		}, list.Contexts)
		s.NotContains(stdout, "token", "the login is never published")
	})

	s.Run("json fails as one error object with no current context", func() {
		s.savedContexts()
		s.Require().NoError(config.ResetCurrentContext())
		stdout, stderr, err := runContextCmd("list", "-o", "json")
		s.Error(err)
		s.Empty(stderr)
		failure := decodeOne[cliout.ErrorObject](s, stdout)
		s.Equal(1, failure.Code)
		s.Equal(KindUnauthenticated, failure.Kind)
	})
}

func (s *CmdSuite) TestContextSwitchOutput() {
	previous := cloudSwitch
	s.T().Cleanup(func() { cloudSwitch = previous })
	// An Astro switch that finds its login: the login check's notes, then the
	// switch.
	cloudSwitch = func(domain string, _ astrov1.APIClient, out io.Writer) error {
		fmt.Fprintf(out, "Logged in to %s\n", domain)
		return context.Switch(domain)
	}
	previousMayPrompt := contextPickerMayPrompt
	s.T().Cleanup(func() { contextPickerMayPrompt = previousMayPrompt })

	s.Run("json publishes the APC context now current", func() {
		s.savedContexts()
		stdout, stderr, err := runContextCmd("switch", "software.example.com", "-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		s.Equal(context.Info{Domain: "software.example.com", IsCurrent: true}, decodeOne[context.Info](s, stdout))
		domain, err := config.GetCurrentDomain()
		s.NoError(err)
		s.Equal("software.example.com", domain)
	})

	s.Run("text for APC is the context table", func() {
		s.savedContexts()
		stdout, _, err := runContextCmd("switch", "software.example.com")
		s.NoError(err)
		s.Contains(stdout, "CONTEXT DOMAIN")
		s.Contains(stdout, "software.example.com")
		s.True(strings.HasSuffix(stdout, "\n Switched context\n"), stdout)
	})

	s.Run("json publishes the Astro context now current, the login's notes on stderr", func() {
		s.savedContexts()
		stdout, stderr, err := runContextCmd("switch", "dev", "-o", "json")
		s.NoError(err)
		s.Equal("Logged in to astronomer-dev.io\n", stderr)
		got := decodeOne[context.Info](s, stdout)
		s.Equal(context.Info{Domain: "astronomer-dev.io", UserEmail: "someone@example.com", IsCurrent: true}, got)
	})

	s.Run("text for Astro is what the login check said, and nothing more", func() {
		s.savedContexts()
		stdout, _, err := runContextCmd("switch", "dev")
		s.NoError(err)
		s.Equal("Logged in to astronomer-dev.io\n", stdout)
	})

	s.Run("json with no domain refuses to pick, naming the argument", func() {
		for _, terminal := range []bool{true, false} {
			s.savedContexts()
			contextPickerMayPrompt = func() bool { return terminal }
			stdout, stderr, err := runContextCmd("switch", "-o", "json")
			s.Error(err)
			s.Empty(stderr)
			failure := decodeOne[cliout.ErrorObject](s, stdout)
			s.Equal(cliout.KindInputRequired, failure.Kind, "terminal=%v", terminal)
			s.Equal(1, failure.Code)
			if terminal {
				s.Contains(failure.Error, "the domain as an argument")
			} else {
				s.Contains(failure.Error, "astro context switch <domain>")
			}
			domain, err := config.GetCurrentDomain()
			s.NoError(err)
			s.Equal("astronomer.io", domain, "nothing switched")
		}
	})
}

func (s *CmdSuite) TestContextDeleteOutput() {
	s.T().Cleanup(func() { noPrompt = false })

	s.Run("json publishes what it deleted", func() {
		s.savedContexts()
		stdout, stderr, err := runContextCmd("delete", "astronomer-dev.io", "-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		s.Equal(context.Removal{Domain: "astronomer-dev.io", Action: "deleted"}, decodeOne[context.Removal](s, stdout))
		s.False(context.Exists("astronomer-dev.io"))
	})

	s.Run("text is the line it always printed", func() {
		s.savedContexts()
		stdout, _, err := runContextCmd("delete", "astronomer-dev.io")
		s.NoError(err)
		s.Equal("Successfully deleted context: astronomer-dev.io\n", stdout)
	})

	s.Run("json refuses to confirm deleting the current context, naming --yes", func() {
		s.savedContexts()
		stdout, stderr, err := runContextCmd("delete", "astronomer.io", "-o", "json")
		s.Error(err)
		s.Empty(stderr)
		failure := decodeOne[cliout.ErrorObject](s, stdout)
		s.Equal(cliout.KindInputRequired, failure.Kind)
		s.Contains(failure.Error, "--yes")
		s.True(context.Exists("astronomer.io"), "nothing deleted")
	})

	s.Run("json deletes the current context with --yes", func() {
		s.savedContexts()
		stdout, _, err := runContextCmd("delete", "astronomer.io", "--yes", "-o", "json")
		s.NoError(err)
		s.Equal(context.Removal{Domain: "astronomer.io", Action: "deleted"}, decodeOne[context.Removal](s, stdout))
		_, err = config.GetCurrentDomain()
		s.ErrorIs(err, config.ErrGetHomeString, "no context is current after")
	})

	s.Run("json fails as one error object for a context that is not saved", func() {
		s.savedContexts()
		stdout, stderr, err := runContextCmd("delete", "nope.example.com", "-o", "json")
		s.ErrorIs(err, config.ErrContextNotExist)
		s.Empty(stderr)
		failure := decodeOne[cliout.ErrorObject](s, stdout)
		s.Equal(1, failure.Code)
		s.Contains(failure.Error, "nope.example.com")
	})
}
