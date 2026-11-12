package astro

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/ide"
	"github.com/astronomer/astro-cli/pkg/input"
)

// resetTokenCommandFlags clears the package variables the workspace and token
// commands read, and puts them back.
func resetTokenCommandFlags(t *testing.T) {
	t.Helper()
	prevClient := astroV1Client
	prevOutput := workspaceTokenOutput
	t.Cleanup(func() {
		astroV1Client = prevClient
		workspaceTokenOutput = prevOutput
		updateWorkspaceRole, addWorkspaceRole, tokenRole, tokenID, orgTokenID, workspaceTokenID = "", "", "", "", "", ""
		deploymentID, tokenName, orgTokenName = "", "", ""
		forceDelete, forceRotate = false, false
	})
	updateWorkspaceRole, addWorkspaceRole, tokenRole, tokenID, orgTokenID, workspaceTokenID = "", "", "", "", "", ""
	deploymentID, tokenName, orgTokenName = "", "", ""
	forceDelete, forceRotate = false, false
	workspaceTokenOutput = cliout.FormatText
}

// workspaceTokenClient is a control plane holding one Workspace-scoped API
// token, token-id, in the organization env-org.
func workspaceTokenClient() *astrov1_mocks.ClientWithResponsesInterface {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	secret := "new-secret"
	token := astrov1.ApiToken{Id: "token-id", Name: "tok", Scope: astrov1.ApiTokenScopeWORKSPACE, Token: &secret}
	mc.On("GetApiTokenWithResponse", mock.Anything, "env-org", "token-id").Return(&astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &token,
	}, nil).Once()
	mc.On("RotateApiTokenWithResponse", mock.Anything, "env-org", "token-id").Return(&astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &token,
	}, nil).Maybe()
	mc.On("DeleteApiTokenWithResponse", mock.Anything, "env-org", "token-id").Return(&astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil).Maybe()
	return mc
}

// Rotating a token, and deleting a Workspace token, given by ID, act on the
// token and need no Workspace, so an Organization token with none can do both.
func TestRotateAndDeleteAWorkspaceTokenByIDNeedNoWorkspace(t *testing.T) {
	for name, tc := range map[string]struct {
		run      func(*cobra.Command, *bytes.Buffer) error
		endpoint string
		want     string
	}{
		"rotate": {
			run: func(cmd *cobra.Command, out *bytes.Buffer) error {
				return rotateWorkspaceToken(cmd, []string{"token-id"}, out)
			},
			endpoint: "RotateApiTokenWithResponse",
			want:     "new-secret",
		},
		"delete": {
			run: func(cmd *cobra.Command, out *bytes.Buffer) error {
				return deleteWorkspaceToken(cmd, []string{"token-id"}, out)
			},
			endpoint: "DeleteApiTokenWithResponse",
			want:     "successfully deleted",
		},
	} {
		t.Run(name, func(t *testing.T) {
			withWorkspaces(t, "", "")
			runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
			resetTokenCommandFlags(t)
			forceRotate, forceDelete = true, true
			mc := workspaceTokenClient()
			astroV1Client = mc

			var out bytes.Buffer
			require.NoError(t, tc.run(&cobra.Command{}, &out))
			assert.Contains(t, out.String(), tc.want)
			mc.AssertCalled(t, tc.endpoint, mock.Anything, "env-org", "token-id")
		})
	}
}

// A run that may not ask names the missing Workspace, not the question it
// would have had to ask first.
func TestMissingWorkspaceIsSaidBeforeAQuestionThatCannotBeAsked(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	resetTokenCommandFlags(t)
	astroV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	t.Cleanup(input.SetGuard(func() string { return "this run cannot ask" }))

	cmd := &cobra.Command{}
	var out bytes.Buffer
	for name, run := range map[string]func() error{
		"workspace team remove":     func() error { return removeWorkspaceTeam(cmd, nil, &out) },
		"workspace team add":        func() error { return addWorkspaceTeam(cmd, nil, &out) },
		"workspace team update":     func() error { return updateWorkspaceTeam(cmd, nil, &out) },
		"workspace user add":        func() error { return addWorkspaceUser(cmd, nil, &out) },
		"workspace user update":     func() error { return updateWorkspaceUser(cmd, nil, &out) },
		"workspace user remove":     func() error { return removeWorkspaceUser(cmd, nil, &out) },
		"workspace token create":    func() error { return createWorkspaceToken(cmd, &out) },
		"workspace token add":       func() error { return addOrgTokenToWorkspace(cmd, nil, &out) },
		"organization-token add":    func() error { return addOrgTokenWorkspaceRole(cmd, nil, &out) },
		"organization-token update": func() error { return updateOrgTokenWorkspaceRole(cmd, nil, &out) },
	} {
		err := run()
		require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace, name)
		assert.False(t, input.IsRequired(err), "%s fails on its question, not on the Workspace", name)
	}
}

// `deployment token workspace-token add` reads the Workspace its token is found
// in, and says there is none before it asks for the role.
func TestDeploymentTokenAddSaysThereIsNoWorkspaceBeforeAskingForARole(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	resetTokenCommandFlags(t)
	astroV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	deploymentID = "dep-id"

	var err error
	asked := captureStderr(t, func() {
		err = setTokenDeploymentRole(&cobra.Command{}, nil, &bytes.Buffer{}, tokenKindWorkspace, tokenRoleAdd)
	})
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	assert.Empty(t, asked, "a question was written before the error")
	assert.Equal(t, 1, bytes.Count([]byte(err.Error()), []byte("pass --workspace")), "the advice is given once: %v", err)
	assert.Contains(t, err.Error(), "the token's ID")
}

// The ide commands say what an Organization token can do about it.
func TestIDECommandsUnderAnOrganizationTokenSayWhatWorks(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")

	_, _, err := validateWorkspaceAndOrgID(&config.Context{Organization: "env-org"})
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	assert.Contains(t, err.Error(), astrosession.HintCurrent)

	_, err = ide.ListProjects(nil)
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	assert.Contains(t, err.Error(), astrosession.HintCurrent)
}
