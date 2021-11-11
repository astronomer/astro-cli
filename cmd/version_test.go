package cmd

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/spf13/cobra"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestVersionRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	// Locally we are not support version, that's why we see err
	_, err := executeCommand("version")
	assert.EqualError(t, err, messages.ErrInvalidCLIVersion)
}

func TestNewVersionCmd(t *testing.T) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	cmd := newVersionCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
