package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt during the lookup of the user a command means stops the
// command there: the lookup returns the cancellation, and no request is sent
// on a client that expects none.
func TestUserLookupsStopOnInterrupt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := astrov1_mocks.NewClientWithResponsesInterface(t)

	t.Run("organization users", func(t *testing.T) {
		_, err := GetOrgUsers(ctx, client)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("workspace users", func(t *testing.T) {
		_, err := GetWorkspaceUsers(ctx, client, "", 0)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("deployment users", func(t *testing.T) {
		_, err := GetDeploymentUsers(ctx, client, "dep", 0)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("a role change finds its user first", func(t *testing.T) {
		_, err := UpdateUserRole(ctx, "someone@example.com", "ORGANIZATION_MEMBER", client)
		require.ErrorIs(t, err, context.Canceled)
	})
}
