package astro

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	workspacetoken "github.com/astronomer/astro-cli/internal/platform/astro/workspace-token"
	"github.com/astronomer/astro-cli/pkg/util"
)

// tokenWith is an API token carrying exactly permissions, as the platform
// issues them.
func tokenWith(t *testing.T, permissions ...string) string {
	t.Helper()
	claims := util.CustomClaims{
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "token-id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-signing-key"))
	require.NoError(t, err)
	return token
}

// withWorkspaces sets the --workspace flag's variable and the project's
// workspace for one test, as the pre-runs do.
func withWorkspaces(t *testing.T, flag, project string) {
	t.Helper()
	prevFlag, prevProject := workspaceID, projectWorkspaceID
	workspaceID, projectWorkspaceID = flag, project
	t.Cleanup(func() { workspaceID, projectWorkspaceID = prevFlag, prevProject })
}

// runWithToken sets up a saved login whose workspace is saved-ws, runs the
// login check with a token carrying permissions, and returns the workspace
// the run then reads.
func runWithToken(t *testing.T, permissions ...string) string {
	t.Helper()
	savedLogin(t, false)
	realTokenParser(t)
	noLoginFlow(t)
	fakeIDP(t)
	t.Setenv("ASTRO_API_TOKEN", tokenWith(t, permissions...))
	require.NoError(t, EnsureLogin(orgsClient(nil)))
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	return c.Workspace
}

// An Organization token selects no Workspace, so the Workspace the login last
// used is not the run's.
func TestAnOrganizationTokenDoesNotTakeTheSavedWorkspace(t *testing.T) {
	withWorkspaces(t, "", "")
	ws := runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	assert.Empty(t, ws, "the run inherited the saved workspace")

	_, err := coalesceWorkspace()
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
	assert.ErrorContains(t, err, "--workspace")
	assert.ErrorContains(t, err, "Organization token")

	// A command that reads the context for its workspace fails the same way,
	// before it calls the API.
	_, _, err = workspacetoken.Target("")
	require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
}

func TestAnOrganizationTokenStillTakesAWorkspaceFromTheFlagOrTheProject(t *testing.T) {
	withWorkspaces(t, "flag-ws", "project-ws")
	ws := runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	assert.Empty(t, ws)

	got, err := coalesceWorkspace()
	require.NoError(t, err)
	assert.Equal(t, "flag-ws", got)

	workspaceID = ""
	got, err = coalesceWorkspace()
	require.NoError(t, err)
	assert.Equal(t, "project-ws", got)

	got, _, err = workspacetoken.Target("flag-ws")
	require.NoError(t, err)
	assert.Equal(t, "flag-ws", got)
}

// A Workspace token and a Deployment token carry the workspace in their claim.
func TestScopedTokensSupplyTheirWorkspace(t *testing.T) {
	for name, permissions := range map[string][]string{
		"workspace":  {"apiTokenId:token-id", "organizationId:env-org", "workspaceId:claim-ws"},
		"deployment": {"apiTokenId:token-id", "organizationId:env-org", "workspaceId:claim-ws", "deploymentId:claim-dep"},
	} {
		t.Run(name, func(t *testing.T) {
			withWorkspaces(t, "", "")
			assert.Equal(t, "claim-ws", runWithToken(t, permissions...))
			got, err := coalesceWorkspace()
			require.NoError(t, err)
			assert.Equal(t, "claim-ws", got)
		})
	}
}

// With no token the login's own selection is the run's.
func TestAnInteractiveLoginKeepsItsSavedWorkspace(t *testing.T) {
	withWorkspaces(t, "", "")
	savedLogin(t, false)
	got, err := coalesceWorkspace()
	require.NoError(t, err)
	assert.Equal(t, "saved-ws", got)
}

// With no token and no saved workspace the error is the one that says how to
// pick one, not the token's.
func TestNoWorkspaceWithoutATokenKeepsItsMessage(t *testing.T) {
	withWorkspaces(t, "", "")
	assert.NotErrorIs(t, astrosession.NoWorkspace(errors.New("x")), astrosession.ErrOrgTokenNoWorkspace)
}
