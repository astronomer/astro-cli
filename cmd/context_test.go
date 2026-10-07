package cmd

import (
	"bytes"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestContextSwitch() {
	previous := cloudSwitch
	s.T().Cleanup(func() { cloudSwitch = previous })

	var got string
	cloudSwitch = func(domain string, astroV1Client astrov1.APIClient, out io.Writer) error {
		got = domain
		return nil
	}

	s.Run("an Astro domain or short name goes through the saved login", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		tests := map[string]string{
			"prod":                      "astronomer.io",
			"stage":                     "astronomer-stage.io",
			"pr12345":                   "pr12345.astronomer-dev.io",
			"astronomer-dev.io":         "astronomer-dev.io",
			"pr12345.astronomer-dev.io": "pr12345.astronomer-dev.io",
		}
		for name, want := range tests {
			got = ""
			s.NoError(switchContext(&cobra.Command{}, []string{name}, nil, textTo(new(bytes.Buffer))))
			s.Equal(want, got, name)
		}
	})

	s.Run("an APC domain switches as before", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		got = ""
		s.NoError(switchContext(&cobra.Command{}, []string{"software.example.com"}, nil, textTo(new(bytes.Buffer))))
		s.Empty(got)
		domain, err := config.GetCurrentDomain()
		s.NoError(err)
		s.Equal("software.example.com", domain)
	})
}

func (s *CmdSuite) TestContextSwitchPicker() {
	previous := cloudSwitch
	s.T().Cleanup(func() { cloudSwitch = previous })

	var got string
	cloudSwitch = func(domain string, astroV1Client astrov1.APIClient, out io.Writer) error {
		got = domain
		return nil
	}

	previousMayPrompt := contextPickerMayPrompt
	s.T().Cleanup(func() { contextPickerMayPrompt = previousMayPrompt })
	contextPickerMayPrompt = func() bool { return true }

	const green, reset = "\033[1;32m", "\033[0m"

	// Two saved contexts, astronomer.io current: astronomer-dev.io is row 1
	// and astronomer.io row 2, sorted by domain.
	setup := func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		dev := config.Context{Domain: "astronomer-dev.io", UserEmail: "someone@example.com"}
		s.Require().NoError(dev.SetContext())
		got = ""
	}
	// run returns what the picker drew on stdout and what went to stderr.
	run := func(stdin string) (picker, stderr string, err error) {
		cmd := &cobra.Command{}
		cmd.SetIn(strings.NewReader(stdin))
		errOut := new(bytes.Buffer)
		cmd.SetErr(errOut)
		out := new(bytes.Buffer)
		err = switchContext(cmd, nil, nil, textTo(out))
		return out.String(), errOut.String(), err
	}
	// rowOf is the line of the picker's table that holds domain.
	rowOf := func(picker, domain string) string {
		for _, line := range strings.Split(picker, "\n") {
			if strings.Contains(line, " "+domain+" ") {
				return line
			}
		}
		return ""
	}

	s.Run("draws the current context in green and switches to the row picked", func() {
		setup()
		picker, _, err := run("1\n")
		s.NoError(err)
		s.Contains(picker, "Switch to which context?")
		s.Contains(picker, "DOMAIN")
		s.Contains(picker, "\n> ")
		current := rowOf(picker, "astronomer.io")
		s.True(strings.HasPrefix(current, green) && strings.Contains(current, reset), "current row is green: %q", current)
		other := rowOf(picker, "astronomer-dev.io")
		s.NotContains(other, green)
		s.Contains(other, "someone@example.com")
		s.NotContains(picker, "ASTRO_DOMAIN", "no mark column without the variable")
		s.Equal("astronomer-dev.io", got)
	})

	s.Run("marks a row ASTRO_DOMAIN made current, and warns that it outranks the switch", func() {
		setup()
		s.T().Setenv("ASTRO_DOMAIN", "astronomer-dev.io")
		picker, stderr, err := run("2\n")
		s.NoError(err)
		current := rowOf(picker, "astronomer-dev.io")
		s.True(strings.HasPrefix(current, green), "the variable's host is the green row: %q", current)
		s.Contains(current, "ASTRO_DOMAIN")
		s.NotContains(rowOf(picker, "astronomer.io"), "ASTRO_DOMAIN")
		s.Equal("astronomer.io", got)
		s.Contains(stderr, "ASTRO_DOMAIN=astronomer-dev.io is set in this shell")
	})

	s.Run("does not warn when ASTRO_DOMAIN names the switched-to host another way", func() {
		setup()
		cmd := &cobra.Command{}
		errOut := new(bytes.Buffer)
		cmd.SetErr(errOut)
		for _, env := range []string{"astronomer.io", "https://cloud.astronomer.io/", "Astronomer.io"} {
			errOut.Reset()
			s.T().Setenv("ASTRO_DOMAIN", env)
			s.NoError(switchContext(cmd, []string{"cloud.astronomer.io"}, nil, textTo(new(bytes.Buffer))), env)
			s.NotContains(errOut.String(), "outranks", env)
		}
		s.T().Setenv("ASTRO_DOMAIN", "cloud.astronomer-dev.io")
		s.NoError(switchContext(cmd, []string{"astronomer.io"}, nil, textTo(new(bytes.Buffer))))
		s.Contains(errOut.String(), "outranks", "a different host still warns")
	})

	s.Run("refuses an answer that is not a row number", func() {
		for _, answer := range []string{"", "3", "0", "dev", "astronomer.io"} {
			setup()
			_, _, err := run(answer + "\n")
			s.ErrorIs(err, errInvalidContextSelection, answer)
			s.Empty(got, answer)
		}
	})

	s.Run("away from a terminal it asks for the domain instead of prompting", func() {
		setup()
		contextPickerMayPrompt = func() bool { return false }
		s.T().Cleanup(func() { contextPickerMayPrompt = func() bool { return true } })
		picker, _, err := run("1\n")
		s.ErrorContains(err, "astro context switch <domain>")
		s.Empty(picker)
		s.Empty(got)
	})

	s.Run("with no saved contexts it points at login", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		_, _, err := run("1\n")
		s.ErrorContains(err, "astro login")
	})
}
