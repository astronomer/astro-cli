package workspacetoken

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt while a token is looked up by name stops the command there,
// with the cancellation, before any request is sent.
func TestTokenLookupsStopOnInterrupt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	pick := pickIndex(1)

	t.Run("FindToken", func(t *testing.T) {
		_, err := FindToken(ctx, "", "named", workspaceID, "test-org-id", nil, pick, client)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("FindTokenIn", func(t *testing.T) {
		_, err := FindTokenIn(ctx, "", "named", workspaceID, "test-org-id", nil, pick, client)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("TokensOf", func(t *testing.T) {
		_, err := TokensOf(ctx, workspaceID, nil, client)
		require.ErrorIs(t, err, context.Canceled)
	})
}
