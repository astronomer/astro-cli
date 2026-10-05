package cmd

import (
	"strings"

	"github.com/astronomer/astro-cli/cmd/cliout"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/version"
)

// withVersion stamps the linked-in version for one test and restores it.
func (s *CmdSuite) withVersion(v string) {
	prev := version.CurrVersion
	version.CurrVersion = v
	s.T().Cleanup(func() { version.CurrVersion = prev })
}

// The text line is a contract: astronomer/deploy-action runs
// `astro version | awk '{print $4}'` and branches on what it gets.
func (s *CmdSuite) TestVersionTextIsByteStable() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.withVersion("1.29.0")

	for _, args := range [][]string{{"version"}, {"version", "-o", "text"}, {"version", "--output", "text"}} {
		output, err := executeCommand(args...)
		s.Require().NoError(err, args)
		s.Equal("Astro CLI Version: 1.29.0\n", output, args)
		s.Equal("1.29.0", strings.Fields(output)[3], "the field deploy-action's awk reads")
	}
}

// The json shape is pinned exactly. A field added here is a field every
// consumer may come to depend on, so the shape changes deliberately or not at
// all.
func (s *CmdSuite) TestVersionJSONShape() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.withVersion("1.29.0")

	output, err := executeCommand("version", "-o", "json")
	s.Require().NoError(err)
	s.Equal(`{"version":"1.29.0"}`+"\n", output)
}

func (s *CmdSuite) TestVersionRejectsUnknownOutput() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	_, err := executeCommand("version", "-o", "yaml")
	s.EqualError(err, `unknown output format "yaml" (supported: text, json)`)
	s.True(cliout.IsUsage(err), "a bad --output is a usage error: exit 2")
}
