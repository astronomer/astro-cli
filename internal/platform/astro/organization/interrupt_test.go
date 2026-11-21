package organization

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt while an Organization or one of its tokens is looked up by
// name stops the command there, with the cancellation and no request sent.
func TestOrganizationLookupsStopOnInterrupt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := astrov1_mocks.NewClientWithResponsesInterface(t)

	t.Run("a switch by name", func(t *testing.T) {
		_, err := Switch(ctx, "some org name", client, new(bytes.Buffer))
		require.ErrorIs(t, err, context.Canceled)
		require.NotContains(t, err.Error(), "cannot connect")
	})
	t.Run("an Organization token by name", func(t *testing.T) {
		pick := func(string, []apitoken.Token) (int, error) { return 0, nil }
		_, err := FindToken(ctx, "", "named", "test-org-id", pick, client)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("the Organization tokens a command picks among", func(t *testing.T) {
		_, err := ScopedTokens(ctx, client)
		require.ErrorIs(t, err, context.Canceled)
	})
}
