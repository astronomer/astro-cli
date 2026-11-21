package httputil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStateCommandIsOnlyNamedWhereItShowsTheState(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/organizations/o/workspaces":            "astro workspace list",
		"/v1/organizations/o/workspaces/w":          "",
		"/v1/organizations/o/workspaces/w/whatever": "",
		"/v1/organizations/o/invites":               "",
		"/v1/organizations/o/users/u":               "astro organization user list",
		"/v1/organizations/o/deployments/d/deploys": "astro deployment inspect d",
	} {
		assert.Equal(t, want, stateCommand(path), path)
	}
}
