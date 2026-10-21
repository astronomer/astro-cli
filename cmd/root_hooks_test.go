package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestRootPersistentPreRunESetupErrors(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("false"))
	origSetup := cloudSetup
	t.Cleanup(func() { cloudSetup = origSetup })

	run := func(setupErr error) error {
		cloudSetup = func(*cobra.Command, astrov1.APIClient) error { return setupErr }
		return CreateRootPersistentPreRunE(nil)(&cobra.Command{Use: "list"}, nil)
	}

	t.Run("an organization that can't be looked up stops the command", func(t *testing.T) {
		err := run(fmt.Errorf("%w: %w", cloudCmd.ErrAPITokenOrganization, errors.New("failed to perform request, status 429")))
		assert.ErrorIs(t, err, cloudCmd.ErrAPITokenOrganization)
		assert.ErrorContains(t, err, "status 429")
	})

	t.Run("an expired API token stops the command", func(t *testing.T) {
		assert.EqualError(t, run(errors.New("the API token given has expired")), "API Token is expired")
	})

	t.Run("other setup errors are only logged", func(t *testing.T) {
		assert.NoError(t, run(errors.New("the API token given does not appear to be an Astro API Token")))
	})
}
