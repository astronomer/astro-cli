package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type renamedFlagEvent struct{ command, flag, current string }

func recordRenamedFlags(t *testing.T) *[]renamedFlagEvent {
	t.Helper()
	got := &[]renamedFlagEvent{}
	orig := recordRenamedFlag
	recordRenamedFlag = func(cmd *cobra.Command, flag, current string) {
		*got = append(*got, renamedFlagEvent{cmd.CommandPath(), flag, current})
	}
	t.Cleanup(func() { recordRenamedFlag = orig })
	return got
}

// A 1.x spelling v2 keeps as a hidden alias says on stderr what the flag is
// called now, and is recorded, so a script gets updated and we can see when
// the alias can go. The v2 spelling says nothing.
func TestRenamedFlagsSayWhatTheyAreCalledNow(t *testing.T) {
	for _, tc := range []struct {
		tree treeConfig
		args []string
		want []renamedFlagEvent
	}{
		{astroTree, []string{"deployment", "list", "--workspace-id", "ws"}, []renamedFlagEvent{
			{"astro deployment list", "--workspace-id", "--workspace"},
		}},
		{astroTree, []string{"deployment", "inspect", "-n", "dep"}, []renamedFlagEvent{
			{"astro deployment inspect", "--deployment-name", "--deployment"},
		}},
		{apcTree, []string{"workspace", "create", "--label", "etl"}, []renamedFlagEvent{
			{"astro workspace create", "--label", "--name"},
		}},
		{astroTree, []string{"deployment", "list", "--workspace", "ws"}, nil},
	} {
		t.Run(tc.tree.name+": astro "+strings.Join(tc.args, " "), func(t *testing.T) {
			got := recordRenamedFlags(t)
			root, _ := disarmedTree(t, tc.tree)
			stdout, stderr, err := executeRoot(root, tc.args...)
			require.ErrorIs(t, err, errRan)
			assert.Empty(t, stdout)
			assert.ElementsMatch(t, tc.want, *got)
			for _, e := range tc.want {
				assert.Contains(t, stderr, "Flag "+e.flag+" has been renamed to "+e.current+", and "+e.flag+" will be removed in a future release.")
			}
			if tc.want == nil {
				assert.NotContains(t, stderr, "has been renamed")
			}
		})
	}
}

// ASTRO_RENAME_HINTS_DISABLED hides the hint, for a script that must still
// run on 1.x, where the new spelling does not exist, but not the event.
func TestRenameHintsCanBeHidden(t *testing.T) {
	t.Setenv(renameHintsDisabledEnv, "1")
	got := recordRenamedFlags(t)
	root, _ := disarmedTree(t, astroTree)
	_, stderr, err := executeRoot(root, "deployment", "list", "--workspace-id", "ws")
	require.ErrorIs(t, err, errRan)
	assert.NotContains(t, stderr, "has been renamed")
	assert.Equal(t, []renamedFlagEvent{{"astro deployment list", "--workspace-id", "--workspace"}}, *got)
}
