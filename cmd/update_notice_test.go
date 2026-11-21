package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The upgrade notice is shown only to a person: upgrade_message on,
// NO_UPDATE_NOTIFIER unset, no CI, and not --output json.
func TestUpdateNoticeGates(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	for _, v := range []string{"CI", "JENKINS_URL", "HUDSON_URL", "TF_BUILD", "NO_UPDATE_NOTIFIER"} {
		t.Setenv(v, "")
	}
	require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("true"))

	newCmd := func(output string) *cobra.Command {
		c := &cobra.Command{Use: "list"}
		var f cliout.Format
		cliout.AddOutputFlag(c, &f)
		if output != "" {
			require.NoError(t, c.ParseFlags([]string{"--output", output}))
		}
		return c
	}

	assert.True(t, updateNoticeWanted(newCmd("")), "a person at a terminal")
	assert.True(t, updateNoticeWanted(&cobra.Command{Use: "version"}), "a command with no --output")

	assert.False(t, updateNoticeWanted(newCmd("json")), "--output json")

	t.Run("CI", func(t *testing.T) {
		t.Setenv("CI", "true")
		assert.False(t, updateNoticeWanted(newCmd("")))
	})
	t.Run("NO_UPDATE_NOTIFIER", func(t *testing.T) {
		t.Setenv("NO_UPDATE_NOTIFIER", "1")
		assert.False(t, updateNoticeWanted(newCmd("")))
	})
	t.Run("upgrade_message false", func(t *testing.T) {
		require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("false"))
		t.Cleanup(func() { require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("true")) })
		assert.False(t, updateNoticeWanted(newCmd("")))
	})
}
