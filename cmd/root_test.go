package cmd

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/suite"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/version"
)

type CmdSuite struct {
	suite.Suite
}

func TestCmd(t *testing.T) {
	suite.Run(t, new(CmdSuite))
}

func executeCommandC(args ...string) (c *cobra.Command, output string, err error) {
	testUtil.SetupOSArgsForGinkgo()
	buf := new(bytes.Buffer)
	rootCmd := NewRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(args)
	c, err = rootCmd.ExecuteC()
	return c, buf.String(), err
}

func executeCommand(args ...string) (output string, err error) {
	_, output, err = executeCommandC(args...)
	return output, err
}

func (s *CmdSuite) TestRootCommandLocal() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand()
	s.NoError(err)
	s.Contains(output, "astro [command]")
	//
	//// Software root command
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err = executeCommand()
	s.NoError(err)
	s.Contains(output, "astro [command]")
	s.Contains(output, "--verbosity")
}

func (s *CmdSuite) TestRootCommandCloudContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	version.CurrVersion = "1.0.0"
	output, err := executeCommand("help")
	s.NoError(err)
	s.Contains(output, "astro [command]")
	s.Contains(output, "completion")
	s.Contains(output, "deploy")
	s.Contains(output, "deployment")
	s.Contains(output, "dev")
	s.Contains(output, "help")
	s.Contains(output, "version")
	s.Contains(output, "workspace")
	s.Contains(output, "run")
	s.NotContains(output, "Run flow commands")
}

func (s *CmdSuite) TestRootCompletionCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	shells := []string{"bash", "fish", "zsh", "powershell"}
	for _, shell := range shells {
		_, err := executeCommand("completion", shell)
		s.NoError(err)
	}
}

func (s *CmdSuite) TestRootCommandSoftwareContext() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	version.CurrVersion = "1.0.0"
	output, err := executeCommand("help")
	s.NoError(err)
	s.Contains(output, "astro [command]")
	s.Contains(output, "completion")
	s.Contains(output, "dev")
	s.Contains(output, "help")
	s.Contains(output, "version")
	s.Contains(output, "workspace")
	s.Contains(output, "user")
	s.Contains(output, "deploy")
	s.Contains(output, "deployment")
	s.Contains(output, "run")
	s.NotContains(output, "Run flow commands")
}

// The self-hosted platform is branded "Astro Private Cloud" (APC); the old
// "Astronomer Software"/"Software" wording must not reappear in command help.
func (s *CmdSuite) TestNoLegacySoftwareBrandingInDescriptions() {
	legacyBranding := regexp.MustCompile(`(?i)(astronomer\s+)?software`)

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, text := range []string{cmd.Short, cmd.Long, cmd.Example} {
			s.NotRegexp(legacyBranding, text, "%q command description contains legacy branding", cmd.CommandPath())
		}
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			s.NotRegexp(legacyBranding, f.Usage, "%q flag --%s usage contains legacy branding", cmd.CommandPath(), f.Name)
		})
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}

	// Software context so the self-hosted subcommand tree is registered too.
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	walk(NewRootCmd())
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	walk(NewRootCmd())
}
