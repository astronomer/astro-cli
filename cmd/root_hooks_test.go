package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestRootPersistentPreRunESetupErrors(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("false"))
	origSetup := astroSetup
	t.Cleanup(func() { astroSetup = origSetup })

	run := func(setupErr error) error {
		astroSetup = func(*cobra.Command, astrov1.APIClient) error { return setupErr }
		return CreateRootPersistentPreRunE(nil)(&cobra.Command{Use: "list"}, nil)
	}

	t.Run("an organization that can't be looked up stops the command", func(t *testing.T) {
		err := run(fmt.Errorf("%w: %w", astroCmd.ErrAPITokenOrganization, errors.New("failed to perform request, status 429")))
		assert.ErrorIs(t, err, astroCmd.ErrAPITokenOrganization)
		assert.ErrorContains(t, err, "status 429")
	})

	t.Run("an expired API token stops the command", func(t *testing.T) {
		assert.EqualError(t, run(errors.New("the API token given has expired")), "API Token is expired")
	})

	t.Run("other setup errors are only logged", func(t *testing.T) {
		assert.NoError(t, run(errors.New("the API token given does not appear to be an Astro API Token")))
	})
}
