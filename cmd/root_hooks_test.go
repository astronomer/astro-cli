package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The hook fails closed: whatever Astro's setup returns stops the command,
// as it was returned, so its kind and message reach the person. What is soft is
// decided inside setup (a token renewal that failed while the access token
// still works; the version check), and never reaches the hook as an error.
func TestRootPersistentPreRunESetupErrors(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("false"))
	origSetup := astroSetup
	t.Cleanup(func() { astroSetup = origSetup })

	run := func(setupErr error) error {
		astroSetup = func(*cobra.Command, astrov1.APIClient) error { return setupErr }
		return CreateRootPersistentPreRunE(nil)(&cobra.Command{Use: "list"}, nil)
	}

	transport := &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}}
	hard := []struct {
		name string
		err  error
		kind cliout.ProblemKind
	}{
		{"an organization that can't be looked up stops the command", fmt.Errorf("%w: %w", astroCmd.ErrAPITokenOrganization, errors.New("failed to perform request, status 429")), ""},
		{"an organization lookup that got no answer is api_unavailable", fmt.Errorf("%w: %w", astroCmd.ErrAPITokenOrganization, transport), KindAPIUnavailable},
		{"an API token with no permissions", astroCmd.ErrNotAPIToken, KindUnauthenticated},
		{"an expired API token", astroCmd.ErrAPITokenExpired, KindUnauthenticated},
		{"a malformed API token", astroCmd.ErrAPITokenMalformed, KindUnauthenticated},
		{"a token overlay that could not be applied", errors.New("cannot apply ASTRO_API_TOKEN's workspace to this run: this context has no login from the environment"), ""},
		{"a login the run may not start", fmt.Errorf("%w; stdin is not a terminal", astroAuth.ErrLoginNeeded), KindUnauthenticated},
		{"a login that could not be renewed", fmt.Errorf("%w, run 'astro login' to log in again: refused", astroCmd.ErrLoginRenewal), KindUnauthenticated},
		{"no context set", config.ErrGetHomeString, KindUnauthenticated},
		{"a config that did not parse", fmt.Errorf("%w: config.yaml did not parse", astroCmd.ErrConfigUnreadable), KindUnauthenticated},
		{"a device login that failed", errors.New("device code expired"), ""},
		{"Deployment API keys", astroCmd.ErrLegacyAPIKeys, KindUnauthenticated},
	}
	for _, tc := range hard {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.err)
			require.Error(t, err)
			assert.Same(t, tc.err, err, "returned as setup gave it")
			assert.Equal(t, tc.kind, problemKinds.Of(err))
		})
	}

	t.Run("an interrupt is returned for exit 130", func(t *testing.T) {
		err := run(input.ErrInterrupted)
		require.ErrorIs(t, err, input.ErrInterrupted)
		assert.Equal(t, cliout.ExitInterrupted, cliout.ExitCode(context.Background(), err))
	})

	t.Run("a setup that found nothing wrong lets the command run", func(t *testing.T) {
		assert.NoError(t, run(nil))
	})

	t.Run("Deployment API keys in the environment stop the command", func(t *testing.T) {
		t.Setenv("ASTRO_API_TOKEN", "")
		t.Setenv("ASTRONOMER_KEY_ID", "id")
		t.Setenv("ASTRONOMER_KEY_SECRET", "secret")
		astroSetup = origSetup
		root := &cobra.Command{Use: "astro"}
		leaf := &cobra.Command{Use: "list"}
		root.AddCommand(leaf)
		err := CreateRootPersistentPreRunE(nil)(leaf, nil)
		require.ErrorIs(t, err, astroCmd.ErrLegacyAPIKeys)
		assert.Equal(t, KindUnauthenticated, problemKinds.Of(err))
	})
}
