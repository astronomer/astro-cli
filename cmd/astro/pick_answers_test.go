package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A team or user picker refused for want of a way to ask names the option its
// own command really has: a positional argument on most, a flag on
// `organization team user`. Run from commands of both kinds.
func TestTeamAndUserPickersNameTheOptionTheirCommandHas(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		notWant string
	}{
		{"workspace user remove, an email argument", []string{"workspace", "user", "remove"}, "pass the user's email as an argument", "--user-id"},
		{"workspace team remove, a team ID argument", []string{"workspace", "team", "remove"}, "pass the team ID as an argument", "--team-id"},
		{"organization team user remove, flags", []string{"organization", "team", "user", "remove"}, "pass --team-id", "as an argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CI", "true")
			root := newWorkspaceCmd
			if tc.args[0] == "organization" {
				root = newOrganizationCmd
			}
			r := execAstroCmd(t, new(astrov1_mocks.ClientWithResponsesInterface), "", root, tc.args...)
			require.Error(t, r.err)
			assert.True(t, input.IsRequired(r.err), "%v", r.err)
			assert.Contains(t, r.err.Error(), tc.want)
			assert.NotContains(t, r.err.Error(), tc.notWant)
		})
	}
}

// At the end of its input (not in CI), the shared picker names what the
// command that opened it said answers it: an email argument for a workspace
// user command, flags for the organization team user commands.
func TestEndedPickersNameTheOptionTheirCommandSaid(t *testing.T) {
	t.Cleanup(input.ClearHints)
	users := []astrov1.User{{Id: "u1", Username: "a@example.com"}}

	require.NoError(t, mayPick("a user", userEmailAnswer))
	func() {
		defer testUtil.MockUserInput(t, "")()
		_, err := user.SelectUser(users, "workspace", "ws")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pass "+userEmailAnswer)
		assert.NotContains(t, err.Error(), "--user-id")
	}()

	input.ClearHints()
	require.NoError(t, mayPickTeamAndUser("a user"))
	func() {
		defer testUtil.MockUserInput(t, "")()
		_, err := user.SelectUser(users, "organization", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pass "+userIDFlag)
		assert.NotContains(t, err.Error(), "as an argument")
	}()
}
