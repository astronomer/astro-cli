package cmd

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func executeCommandC(client *houston.Client, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	rootCmd := NewRootCmd(client, buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(args)
	c, err = rootCmd.ExecuteC()
	client.HTTPClient.HTTPClient.CloseIdleConnections()
	return c, buf.String(), err
}

func executeCommand(args ...string) (output string, err error) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	_, output, err = executeCommandC(client, args...)
	return output, err
}

func TestDevRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("dev")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
}
