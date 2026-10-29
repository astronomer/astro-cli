package apc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// The commands that delete something for good ask first, as Astro's do: a
// yes deletes, --yes skips the question, --output json without --yes is
// refused before anything is read or sent, and a no changes nothing.
func TestRemovalsAskFirst(t *testing.T) {
	for _, c := range []struct {
		name     string
		args     []string
		method   string
		returns  any
		question string
	}{
		{
			"workspace delete",
			[]string{"workspace", "delete", "ws-1"},
			"DeleteWorkspace", &houston.Workspace{ID: "ws-1", Label: "alpha"},
			"Are you sure you want to delete Workspace ws-1?",
		},
		{
			"workspace service-account delete",
			[]string{"workspace", "service-account", "delete", "sa-1", "--workspace", "ws-1"},
			"DeleteWorkspaceServiceAccount", &houston.ServiceAccount{ID: "sa-1", Label: "ci"},
			"Are you sure you want to delete service account sa-1?",
		},
		{
			"deployment service-account delete",
			[]string{"deployment", "service-account", "delete", "sa-1", "--deployment", "dep-1"},
			"DeleteDeploymentServiceAccount", &houston.ServiceAccount{ID: "sa-1", Label: "ci"},
			"Are you sure you want to delete service account sa-1?",
		},
	} {
		client := func() *mocks.ClientInterface {
			api := newAPCClient()
			api.On(c.method, mock.Anything).Return(c.returns, nil).Maybe()
			return api
		}

		t.Run(c.name+": a yes deletes", func(t *testing.T) {
			api := client()
			run := runAPC(t, api, "y\n", c.args...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Contains(t, run.stderr, c.question)
			api.AssertCalled(t, c.method, mock.Anything)
		})

		t.Run(c.name+": --yes and -y skip the question", func(t *testing.T) {
			for _, yes := range []string{"--yes", "-y"} {
				api := client()
				run := runAPC(t, api, "", append(append([]string{}, c.args...), yes)...)
				require.Equal(t, 0, run.code, "%s: stderr:\n%s", yes, run.stderr)
				assert.NotContains(t, run.stderr, c.question, yes)
				api.AssertCalled(t, c.method, mock.Anything)
			}
		})

		t.Run(c.name+": --output json without --yes is refused", func(t *testing.T) {
			api := client()
			run := runAPC(t, api, "y\n", append(append([]string{}, c.args...), "-o", "json")...)
			assert.Equal(t, 1, run.code)
			var got errorObject
			decodeOne(t, run.stdout, &got)
			assert.Contains(t, got.Error, "--yes")
			assert.False(t, run.stdinRead, "a refused prompt read stdin")
			api.AssertNotCalled(t, c.method, mock.Anything)
		})

		t.Run(c.name+": a no changes nothing", func(t *testing.T) {
			api := client()
			run := runAPC(t, api, "n\n", c.args...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Contains(t, run.stdout, "Canceling")
			api.AssertNotCalled(t, c.method, mock.Anything)
		})
	}
}
