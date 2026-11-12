package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
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
