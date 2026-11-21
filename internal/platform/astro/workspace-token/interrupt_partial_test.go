package workspacetoken

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A role written, then an interrupt before the rename: the error still is the
// interrupt and says the role changed, the result is the token as it now is,
// and no rename is sent.
func TestInterruptBetweenTheRoleAndTheRename(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	token := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(&astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &token,
	}, nil).Once()
	client.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(
		func(context.Context, string, string, astrov1.UpdateApiTokenRolesJSONRequestBody, ...astrov1.RequestEditorFn) (*astrov1.UpdateApiTokenRolesResponse, error) {
			cancel()
			return rolesOK(), nil
		}).Once()

	got, err := UpdateToken(ctx, "t1", "", "renamed", "", "WORKSPACE_OWNER", workspaceID, nil, pickIndex(0), client)
	require.ErrorIs(t, err, context.Canceled)
	require.Contains(t, err.Error(), "updated the token's role to WORKSPACE_OWNER")
	require.Equal(t, "one", got.PreviousName)
	require.Equal(t, "WORKSPACE_OWNER", got.Token.Role)
	client.AssertNotCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
