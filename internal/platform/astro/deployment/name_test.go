package deployment

import (
	"testing"

	"github.com/stretchr/testify/assert"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

func TestNameError(t *testing.T) {
	prod, staging := "prod", "staging"
	two := []astrov1.Deployment{
		{Id: "id-1", Name: "etl", WorkspaceId: "ws-1", WorkspaceName: &prod},
		{Id: "id-2", Name: "etl", WorkspaceId: "ws-2", WorkspaceName: &staging},
	}

	t.Run("several in a workspace name their ids", func(t *testing.T) {
		err := &NameError{Name: "etl", Workspace: "ws-1", Matches: two, Flag: "--deployment"}
		assert.EqualError(t, err, `2 Deployments are named "etl" in workspace ws-1: pass --deployment with one's id (id-1, id-2)`)
		assert.True(t, err.IsUsageError())
		assert.NotErrorIs(t, err, errInvalidDeployment)
	})

	// Looked up with no workspace, the listing is the whole organization, so
	// the same name can be in several workspaces, and each id says which.
	t.Run("several across the organization name their workspaces", func(t *testing.T) {
		noName := two[1]
		noName.WorkspaceName = nil
		err := &NameError{Name: "etl", Matches: []astrov1.Deployment{two[0], noName}, Flag: "--clone"}
		assert.EqualError(t, err, `2 Deployments are named "etl" in the organization: pass --clone with one's id (id-1 in workspace prod, id-2 in workspace ws-2)`)
	})

	t.Run("none is the lookup's not-found error", func(t *testing.T) {
		err := &NameError{Name: "etl", Workspace: "ws-1", Flag: "--deployment"}
		assert.EqualError(t, err, `no Deployment named "etl" in workspace ws-1 that your account or API token can see`)
		assert.ErrorIs(t, err, errInvalidDeployment)
		assert.True(t, err.IsUsageError())
	})
}
