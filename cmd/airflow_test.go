package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func executeCommandC(client *houston.Client, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	rootCmd := NewRootCmd(client, buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(args)
	_, err = rootCmd.ExecuteC()
	client.HTTPClient.HTTPClient.CloseIdleConnections()
	return buf.String(), err
}

func executeCommand(args ...string) (output string, err error) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	output, err = executeCommandC(client, args...)
	return output, err
}

func TestDevRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("dev")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
}

func TestNewAirflowInitCmd(t *testing.T) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	cmd := newAirflowInitCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStartCmd(t *testing.T) {
	cmd := newAirflowStartCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowKillCmd(t *testing.T) {
	cmd := newAirflowKillCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowLogsCmd(t *testing.T) {
	cmd := newAirflowLogsCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStopCmd(t *testing.T) {
	cmd := newAirflowStopCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowPSCmd(t *testing.T) {
	cmd := newAirflowPSCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowRunCmd(t *testing.T) {
	cmd := newAirflowRunCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowUpgradeCheckCmd(t *testing.T) {
	cmd := newAirflowUpgradeCheckCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
