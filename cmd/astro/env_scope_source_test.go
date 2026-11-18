package astro

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/astrosession"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The env scope records where its Workspace came from, so the scope mismatch
// names it: the flag, the token's claim, or (not named) the saved login.
func TestEnvScopeRecordsTheWorkspacesSource(t *testing.T) {
	reset := func(t *testing.T) {
		t.Helper()
		prevWS, prevDep := envWorkspaceID, envDeploymentID
		t.Cleanup(func() { envWorkspaceID, envDeploymentID = prevWS, prevDep })
		envWorkspaceID, envDeploymentID = "", ""
	}

	t.Run("the flag", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		reset(t)
		envWorkspaceID = "clflagworkspace00000001"
		scope, err := envScope(&cobra.Command{})
		require.NoError(t, err)
		assert.Equal(t, astrosession.SourceFlag, scope.Source)
		assert.Equal(t, "clflagworkspace00000001", scope.WorkspaceID)
	})
	t.Run("--deployment alone names no Workspace and no source", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		reset(t)
		envDeploymentID = "cldeployment000000001"
		scope, err := envScope(&cobra.Command{})
		require.NoError(t, err)
		assert.Equal(t, astrosession.SourceNone, scope.Source)
		assert.Empty(t, scope.WorkspaceID)
	})
	t.Run("the project link", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		reset(t)
		c := &cobra.Command{}
		c.SetContext(withProjectFollow(context.Background(), projectFollow{workspace: projectWS}))
		scope, err := envScope(c)
		require.NoError(t, err)
		assert.Equal(t, astrosession.SourceProject, scope.Source)
		assert.Equal(t, projectWS, scope.WorkspaceID)
	})
	t.Run("the token's claim", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:"+claimWS)
		reset(t)
		scope, err := envScope(&cobra.Command{})
		require.NoError(t, err)
		assert.Equal(t, astrosession.SourceToken, scope.Source)
		assert.Equal(t, claimWS, scope.WorkspaceID)
	})
	t.Run("the saved login is not named", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		reset(t)
		scope, err := envScope(&cobra.Command{})
		require.NoError(t, err)
		assert.Equal(t, astrosession.SourceContext, scope.Source)
		assert.Empty(t, scope.Source.Label("--workspace"))
	})
}

// A Workspace that cannot be found hands back the run's own client, not one
// that vouches for an empty Workspace's source.
func TestDeploymentWorkspaceFailureReturnsThePlainClient(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	testUtil.UseAPIToken(t, "organizationId:org")
	prev := workspaceID
	t.Cleanup(func() { workspaceID = prev })
	workspaceID = ""
	ws, client, err := deploymentWorkspace(&cobra.Command{}, "etl", "")
	require.Error(t, err)
	assert.Empty(t, ws)
	assert.Equal(t, astroV1Client, client)
}
