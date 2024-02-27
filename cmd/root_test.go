package cmd

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

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

func TestRootCommandLocal(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand()
	assert.NoError(t, err)
	assert.Contains(t, output, "astro [command]")
	//
	//// Software root command
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err = executeCommand()
	assert.NoError(t, err)
	assert.Contains(t, output, "astro [command]")
	assert.Contains(t, output, "--verbosity")
}

func TestRootCommandCloudContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	version.CurrVersion = "1.0.0"
	output, err := executeCommand("help")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro [command]")
	assert.Contains(t, output, "completion")
	assert.Contains(t, output, "deploy")
	assert.Contains(t, output, "deployment")
	assert.Contains(t, output, "dev")
	assert.Contains(t, output, "help")
	assert.Contains(t, output, "version")
	assert.Contains(t, output, "workspace")
	assert.Contains(t, output, "run")
	assert.NotContains(t, output, "Run flow commands")
}

func TestRootCompletionCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	completionShellMapSha := map[string]string{"bash": "291b774846025599cd10107324f8a776", "fish": "44b594d5d9e4203e1089396732832061", "zsh": "b9baad5816441d010ca622974699274b", "powershell": "8e03321aa8fa1b18756662efd3fca6d5"}
	for shell, sha := range completionShellMapSha {
		cmd1 := exec.Command("astro", "completion", shell)
		output1, _ := cmd1.Output()
		cmd2 := exec.Command("openssl", "md5")
		cmd2.Stdin = strings.NewReader(string(output1))
		output2, _ := cmd2.Output()
		cmd3 := exec.Command("sed", "s/^.*= //")
		cmd3.Stdin = strings.NewReader(string(output2))
		output, _ := cmd3.Output()
		assert.Contains(t, string(output), sha)
	}
}

func TestRootCommandSoftwareContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	version.CurrVersion = "1.0.0"
	output, err := executeCommand("help")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro [command]")
	assert.Contains(t, output, "completion")
	assert.Contains(t, output, "dev")
	assert.Contains(t, output, "help")
	assert.Contains(t, output, "version")
	assert.Contains(t, output, "workspace")
	assert.Contains(t, output, "user")
	assert.Contains(t, output, "deploy")
	assert.Contains(t, output, "deployment")
	assert.Contains(t, output, "run")
	assert.NotContains(t, output, "Run flow commands")
}
