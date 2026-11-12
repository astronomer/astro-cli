package astro

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// Renaming a Workspace token given by ID reads no Workspace; changing its role
// does, and is refused before anything is asked or sent.
func TestUpdateAWorkspaceTokenByIDUnderAnOrganizationToken(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	resetTokenCommandFlags(t)
	tokenID = "token-id"

	t.Run("a rename needs no Workspace", func(t *testing.T) {
		mc := workspaceTokenClient()
		mc.On("UpdateApiTokenWithResponse", mock.Anything, "env-org", "token-id", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.ApiToken{Id: "token-id", Name: "new", Scope: astrov1.ApiTokenScopeWORKSPACE},
		}, nil).Once()
		astroV1Client = mc
		tokenName = "new"
		t.Cleanup(func() { tokenName = "" })

		var out bytes.Buffer
		require.NoError(t, updateWorkspaceToken(&cobra.Command{}, nil, &out))
		mc.AssertCalled(t, "UpdateApiTokenWithResponse", mock.Anything, "env-org", "token-id", mock.Anything)
	})

	t.Run("a role change needs one, before anything is asked or sent", func(t *testing.T) {
		mc := new(astrov1_mocks.ClientWithResponsesInterface) // fails the test on any call
		astroV1Client = mc
		tokenRole = "WORKSPACE_MEMBER"
		t.Cleanup(func() { tokenRole = "" })

		var err error
		asked := captureStderr(t, func() { err = updateWorkspaceToken(&cobra.Command{}, nil, &bytes.Buffer{}) })
		require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
		assert.Empty(t, asked)
	})
}

// A Deployment name is looked up in the workspace of the login in use. Under an
// API token that is the current context's alone: another host's saved login
// last used a workspace nobody chose in this run.
func TestAnAPITokenDoesNotTakeAnotherHostsSavedWorkspace(t *testing.T) {
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	current, err := config.GetCurrentDomain()
	require.NoError(t, err)

	other := config.Context{Domain: "other.example.com"}
	require.NoError(t, context.Switch(other.Domain))
	require.NoError(t, other.SetContextKey("workspace", "other-ws"))
	require.NoError(t, context.Switch(current))

	assert.Equal(t, "other-ws", func() string {
		t.Setenv("ASTRO_API_TOKEN", "")
		return instancelocate.Workspace(other.Domain)
	}(), "a saved login keeps its own workspace")

	t.Setenv("ASTRO_API_TOKEN", tokenWith(t, "apiTokenId:token-id", "organizationId:env-org"))
	assert.Empty(t, instancelocate.Workspace(other.Domain), "a token run took another host's saved workspace")
	assert.Empty(t, instancelocate.Workspace(current), "a token run took the saved workspace of its own host")
}
