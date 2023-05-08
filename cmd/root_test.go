package cmd

import (
	"bytes"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestCmdSuite(t *testing.T) {
	suite.Run(t, new(Suite))
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

func (s *Suite) TestRootCommandLocal() {
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

func (s *Suite) TestRootCommandCloudContext() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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

func (s *Suite) TestRootCommandSoftwareContext() {
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
