package cmd

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/suite"
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
	completionShellMapSha := map[string]string{"bash": "291b774846025599cd10107324f8a776", "fish": "44b594d5d9e4203e1089396732832061", "zsh": "b9baad5816441d010ca622974699274b", "powershell": "8e03321aa8fa1b18756662efd3fca6d5"}
	for shell, sha := range completionShellMapSha {
		output1, _ := executeCommand("completion", shell)
		cmd2 := exec.Command("openssl", "md5")
		cmd2.Stdin = strings.NewReader(output1)
		output2, _ := cmd2.Output()
		cmd3 := exec.Command("sed", "s/^.*= //")
		cmd3.Stdin = strings.NewReader(string(output2))
		output, _ := cmd3.Output()
		s.Contains(string(output), sha)
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
