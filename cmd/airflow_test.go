package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/astrohub"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func executeCommandC(client *astrohub.Client, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	rootCmd := NewRootCmd(client, buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(args)
	c, err = rootCmd.ExecuteC()
	client.HTTPClient.HTTPClient.CloseIdleConnections()
	return c, buf.String(), err
}

func executeCommand(args ...string) (output string, err error) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())

	_, output, err = executeCommandC(client, args...)
	return output, err
}

func TestDevRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("dev")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
}

func TestNewAirflowInitCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowInitCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStartCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowStartCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowKillCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowKillCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowLogsCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowLogsCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStopCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowStopCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowPSCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowPSCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowRunCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowRunCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowUpgradeCheckCmd(t *testing.T) {
	client := astrohub.NewAstrohubClient(httputil.NewHTTPClient())
	cmd := newAirflowUpgradeCheckCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
