package astro

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	workspacetoken "github.com/astronomer/astro-cli/internal/platform/astro/workspace-token"
)

// captureStderr runs fn with os.Stderr going to a file, and stdin offering an
// answer, and returns what was written: a question asked on the way would be.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	dir := t.TempDir()
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)
	inFile := filepath.Join(dir, "stdin")
	require.NoError(t, os.WriteFile(inFile, []byte("1\ny\n"), 0o600))
	in, err := os.Open(inFile)
	require.NoError(t, err)
	prevErr, prevIn := os.Stderr, os.Stdin
	os.Stderr, os.Stdin = errFile, in
	defer func() { os.Stderr, os.Stdin = prevErr, prevIn }()
	fn()
	require.NoError(t, errFile.Sync())
	out, err := os.ReadFile(errFile.Name())
	require.NoError(t, err)
	return string(out)
}

// With an Organization token and no --workspace, a command that would ask for
// a role, a team or a confirmation says there is no Workspace instead, and asks
// nothing.
func TestAnOrganizationTokenIsRefusedBeforeAnyQuestion(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	prevClient := astroV1Client
	t.Cleanup(func() {
		astroV1Client = prevClient
		updateWorkspaceRole, tokenRole, tokenID, orgTokenID, forceDelete = "", "", "", "", false
	})
	updateWorkspaceRole, tokenRole, tokenID, orgTokenID, forceDelete = "", "", "", "", false

	// A client that fails the test if a command reaches the platform for
	// anything but the token it was given by ID.
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "token-id").Return(&astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.ApiToken{Id: "token-id", Name: "tok", Scope: astrov1.ApiTokenScopeORGANIZATION},
	}, nil).Maybe()
	astroV1Client = mc

	cmd := &cobra.Command{}
	var out bytes.Buffer
	for name, run := range map[string]func() error{
		"workspace team update":     func() error { return updateWorkspaceTeam(cmd, nil, &out) },
		"workspace user update":     func() error { return updateWorkspaceUser(cmd, nil, &out) },
		"workspace team add":        func() error { return addWorkspaceTeam(cmd, nil, &out) },
		"workspace team remove":     func() error { return removeWorkspaceTeam(cmd, nil, &out) },
		"workspace user add":        func() error { return addWorkspaceUser(cmd, nil, &out) },
		"workspace user remove":     func() error { return removeWorkspaceUser(cmd, nil, &out) },
		"workspace token add":       func() error { return addOrgTokenToWorkspace(cmd, nil, &out) },
		"workspace token create":    func() error { return createWorkspaceToken(cmd, &out) },
		"workspace token update":    func() error { return updateWorkspaceToken(cmd, nil, &out) },
		"organization-token add":    func() error { return addOrgTokenWorkspaceRole(cmd, nil, &out) },
		"organization-token update": func() error { return updateOrgTokenWorkspaceRole(cmd, nil, &out) },
		"organization-token remove": func() error { return removeOrganizationTokenWorkspaceRole(cmd, nil, &out) },
		"workspace token delete of an Organization token by ID": func() error {
			tokenID = "token-id"
			defer func() { tokenID = "" }()
			return deleteWorkspaceToken(cmd, nil, &out)
		},
	} {
		var err error
		asked := captureStderr(t, func() { err = run() })
		require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace, name)
		assert.Empty(t, asked, "%s asked something before failing", name)
	}
}

// The advice follows the ways the command can be given a Workspace.
func TestTheMissingWorkspaceAdviceNamesOnlyWhatTheCommandTakes(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")

	projectFollowed = false
	_, err := coalesceWorkspace()
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	assert.Contains(t, err.Error(), "pass --workspace")
	assert.NotContains(t, err.Error(), "project")

	projectFollowed = true
	t.Cleanup(func() { projectFollowed = false })
	_, err = coalesceWorkspace()
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	assert.Contains(t, err.Error(), "--workspace, or run in a project")
}

// Creating or updating a Workspace token posts a Workspace id, so there must be
// one; rotating a token or deleting a Workspace token, given by ID, needs none.
func TestWorkspaceTokenCommandsUnderAnOrganizationToken(t *testing.T) {
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")

	_, err := workspacetoken.CreateToken("x", "", "WORKSPACE_MEMBER", "", 0, nil)
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	_, err = workspacetoken.UpdateToken("id", "", "new", "", "WORKSPACE_MEMBER", "", nil, nil)
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)

	ws, org, err := workspacetoken.Scope("")
	require.NoError(t, err)
	assert.Empty(t, ws)
	assert.Equal(t, "env-org", org)
}
