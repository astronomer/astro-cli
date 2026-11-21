package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt while a Deployment, or a token of one, is looked up stops the
// command there with the cancellation, not as a Deployment that was not found.
func TestDeploymentLookupsStopOnInterrupt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Deployments by name reach the list with the run's context", func(t *testing.T) {
		orig := ListDeployments
		t.Cleanup(func() { ListDeployments = orig })
		ListDeployments = func(c context.Context, _, _ string, _ astrov1.APIClient) ([]astrov1.Deployment, error) {
			assert.Equal(t, ctx, c)
			return nil, c.Err()
		}
		_, err := DeploymentsNamed(ctx, "etl", "ws", "org", astrov1_mocks.NewClientWithResponsesInterface(t))
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("one Deployment by id reads under the run's context", func(t *testing.T) {
		client := astrov1_mocks.NewClientWithResponsesInterface(t)
		client.On("GetDeploymentWithResponse", ctx, "org", "dep", mock.Anything).Return(nil, context.Canceled).Once()
		_, err := GetDeploymentByID(ctx, "org", "dep", client)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("a Deployment token by name", func(t *testing.T) {
		pick := func(string, []apitoken.Token) (int, error) { return 0, nil }
		_, err := FindToken(ctx, "", "named", deploymentID, nil, pick, astrov1_mocks.NewClientWithResponsesInterface(t))
		require.ErrorIs(t, err, context.Canceled)
	})
}
