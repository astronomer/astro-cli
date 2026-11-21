package team

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt during the lookup of the team a command means stops the
// command there: the lookup returns the cancellation, and no request is sent
// on a client that expects none.
func TestTeamLookupsStopOnInterrupt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := astrov1_mocks.NewClientWithResponsesInterface(t)

	t.Run("organization teams", func(t *testing.T) {
		_, err := GetOrgTeams(ctx, client)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("workspace teams", func(t *testing.T) {
		_, err := GetWorkspaceTeams(ctx, client, "", 0)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("deployment teams", func(t *testing.T) {
		_, err := GetDeploymentTeams(ctx, client, "dep", 0)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("a delete that picks among the teams", func(t *testing.T) {
		_, err := Delete(ctx, "", true, client)
		require.ErrorIs(t, err, context.Canceled)
	})
}
