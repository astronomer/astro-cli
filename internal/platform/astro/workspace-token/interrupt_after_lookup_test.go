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

// An interrupt that lands after the token is found, and before the change is
// sent, sends no change: the mock expects no update, and the run is the
// interrupt.
func TestInterruptAfterTheLookupSendsNoUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	token := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(func(context.Context, string, string, ...astrov1.RequestEditorFn) (*astrov1.GetApiTokenResponse, error) {
		cancel()
		return &astrov1.GetApiTokenResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &token}, nil
	}).Once()

	_, err := UpdateToken(ctx, "t1", "", "renamed", "", "", workspaceID, nil, pickIndex(0), client)
	require.ErrorIs(t, err, context.Canceled)
	client.AssertNotCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
