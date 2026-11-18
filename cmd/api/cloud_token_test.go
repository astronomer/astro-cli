package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestFillPlaceholdersWithNoWorkspaceUnderATokenSaysToWriteTheID(t *testing.T) {
	t.Setenv("ASTRO_API_TOKEN", testUtil.MintAPIToken(t, time.Time{}, "organizationId:org"))
	_, err := fillPlaceholders("/workspaces/{workspaceId}", &config.Context{Organization: "org-123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "an Organization token does not")
	assert.Contains(t, err.Error(), "write the Workspace's id in the path")
	assert.NotContains(t, err.Error(), "workspace switch")

	t.Setenv("ASTRO_API_TOKEN", "")
	_, err = fillPlaceholders("/workspaces/{workspaceId}", &config.Context{Organization: "org-123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "astro workspace switch")
}

// A Workspace id written in the path names a Workspace as --workspace does: a
// token for another is refused before the request, and the token's own, a
// placeholder, or an Organization token is not.
func TestCheckPathWorkspaceRefusesAnotherWorkspaceThanTheTokens(t *testing.T) {
	t.Setenv("ASTRO_API_TOKEN", testUtil.MintAPIToken(t, time.Time{}, "organizationId:org", "workspaceId:ws-mine"))
	err := checkPathWorkspace("/organizations/org/workspaces/ws-other/deployments")
	require.Error(t, err)
	assert.Equal(t, "ASTRO_API_TOKEN is a Workspace token for ws-mine, but the request path names ws-other: use a token for ws-other", err.Error())
	assert.True(t, cliout.IsUsage(err))

	require.NoError(t, checkPathWorkspace("/organizations/org/workspaces/ws-mine/deployments"))
	require.NoError(t, checkPathWorkspace("/organizations/org/workspaces/{workspaceId}/deployments"))
	require.NoError(t, checkPathWorkspace("/organizations/org/deployments"))
	require.NoError(t, checkPathWorkspace("/organizations/org/deployments?filter=/workspaces/ws-other"))

	t.Setenv("ASTRO_API_TOKEN", testUtil.MintAPIToken(t, time.Time{}, "organizationId:org"))
	require.NoError(t, checkPathWorkspace("/organizations/org/workspaces/ws-other/deployments"))
}
