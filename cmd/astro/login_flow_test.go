package astro

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A command's login check that has to log in hands the login flow stderr:
// the flow runs ahead of the command's own output, which stdout carries.
// The flow itself writes nothing else to stdout (the auth package's "a
// browser login asks on stderr and leaves stdout empty").
func TestSetupHandsTheLoginFlowStderr(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Setenv("ASTRO_API_TOKEN", "")
	t.Setenv("ASTRONOMER_KEY_ID", "")
	c, err := config.GetCurrentContext()
	assert.NoError(t, err)
	assert.NoError(t, c.SetContextKey("token", ""))

	previous := authLogin
	t.Cleanup(func() { authLogin = previous })
	var got io.Writer
	authLogin = func(_, _ string, _ astrov1.APIClient, out io.Writer, _, _, _ bool) error {
		got = out
		return nil
	}
	root := &cobra.Command{Use: topLvlCmd}
	cmd := &cobra.Command{Use: "probe", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(cmd)

	assert.NoError(t, Setup(cmd, nil))
	assert.Same(t, os.Stderr, got)
}
