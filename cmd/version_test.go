package cmd

import (
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/spf13/cobra"
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestVersionRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	// Locally we are not support version, that's why we see err
	expectedOut := "Error: Astronomer CLI version is not valid\n"
	output, err := executeCommand("version")
	assert.Equal(t, expectedOut, output, err)
}

func TestNewVersionCmd(t *testing.T) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	cmd := newVersionCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
