package local

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The Deployments are listed in the project's workspace with the run's token,
// which is for one Workspace: a token for another is refused before the list.
func TestLinkAddUnderATokenForAnotherWorkspaceListsNothing(t *testing.T) {
	testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws_of_the_token")
	dir, _ := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "", PickedDeployment{ID: "clx9", Name: "Orders Prod (EU)", WorkspaceID: "ws_A"}, "")
	err := execute(t, d, "link", "add")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ASTRO_API_TOKEN is a Workspace token for ws_of_the_token, but the project's link names ws_A")
	assert.Empty(t, run.deploymentWorkspaces, "the Deployments were listed")
}
