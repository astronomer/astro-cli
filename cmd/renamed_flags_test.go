package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/utils"
)

// renamedFlagEvent is one run's queued event: the 1.x spellings it gave, and
// the flags they stand for.
type renamedFlagEvent struct {
	command          string
	renamed, current []string
}

func recordRenamedFlags(t *testing.T) *[]renamedFlagEvent {
	t.Helper()
	got := &[]renamedFlagEvent{}
	orig := utils.RecordRenamedFlags
	utils.RecordRenamedFlags = func(cmd *cobra.Command, renamed, current []string) {
		*got = append(*got, renamedFlagEvent{cmd.CommandPath(), renamed, current})
	}
	t.Cleanup(func() { utils.RecordRenamedFlags = orig })
	return got
}

// A 1.x spelling v2 keeps as a hidden alias says on stderr what the flag is
// called now, by the shorthand too, and is recorded, all of a run's in one
// event, so a script gets updated and we can see when the alias can go.
func TestRenamedFlagsSayWhatTheyAreCalledNow(t *testing.T) {
	for _, tc := range []struct {
		tree  treeConfig
		args  []string
		want  *renamedFlagEvent
		hints []string
	}{
		{
			astroTree,
			[]string{"deployment", "list", "--workspace-id", "ws"},
			&renamedFlagEvent{"astro deployment list", []string{"--workspace-id"}, []string{"--workspace"}},
			[]string{"Flag --workspace-id has been renamed to --workspace. The old spelling will be removed in a future release."},
		},
		{
			astroTree,
			[]string{"deployment", "inspect", "-n", "dep"},
			&renamedFlagEvent{"astro deployment inspect", []string{"--deployment-name"}, []string{"--deployment"}},
			[]string{"Flag --deployment-name (-n) has been renamed to --deployment. --deployment takes Deployment names, ids and link names. The old spelling"},
		},
		{
			apcTree,
			[]string{"deployment", "service-account", "list", "-d", "dep-id"},
			&renamedFlagEvent{"astro deployment service-account list", []string{"--deployment-id"}, []string{"--deployment"}},
			[]string{"Flag --deployment-id (-d) has been renamed to --deployment. --deployment takes Deployment ids and link names, so check that the value names the same Deployment before switching."},
		},
		{
			apcTree,
			[]string{"workspace", "create", "-l", "etl"},
			&renamedFlagEvent{"astro workspace create", []string{"--label"}, []string{"--name"}},
			[]string{"Flag --label (-l) has been renamed to --name (-n)."},
		},
		{
			astroTree,
			[]string{"deployment", "worker-queue", "create", "--deployment-id", "dep-id", "--deployment-name", "dep", "--workspace-id", "ws"},
			&renamedFlagEvent{
				"astro deployment worker-queue create",
				[]string{"--deployment-id", "--deployment-name", "--workspace-id"},
				[]string{"--deployment", "--deployment", "--workspace"},
			},
			[]string{
				"Flags --deployment-id (-d) and --deployment-name are replaced by --deployment, which takes one value. " +
					"--deployment takes Deployment names, ids and link names, so check that the value names the same Deployment before switching.",
				"Flag --workspace-id has been renamed to --workspace.",
			},
		},
		// "" still means the default through the alias, and --workspace
		// refuses it: recorded, but no hint to switch.
		{
			astroTree,
			[]string{"deployment", "list", "--workspace-id", ""},
			&renamedFlagEvent{"astro deployment list", []string{"--workspace-id"}, []string{"--workspace"}},
			nil,
		},
		{astroTree, []string{"deployment", "list", "--workspace", "ws"}, nil, nil},
	} {
		t.Run(tc.tree.name+": astro "+strings.Join(tc.args, " "), func(t *testing.T) {
			got := recordRenamedFlags(t)
			root, _ := disarmedTree(t, tc.tree)
			stdout, stderr, err := executeRoot(root, tc.args...)
			require.ErrorIs(t, err, errRan)
			assert.Empty(t, stdout)
			if tc.want == nil {
				assert.Empty(t, *got)
			} else {
				assert.Equal(t, []renamedFlagEvent{*tc.want}, *got)
			}
			for _, h := range tc.hints {
				assert.Contains(t, stderr, h)
			}
			// Each hint is its own line, led by a plain "Warning:" here, where
			// stderr is no terminal and so gets no color.
			assert.Equal(t, len(tc.hints), strings.Count(stderr, "Warning: Flag"), stderr)
			assert.Equal(t, len(tc.hints), strings.Count(stderr, "will be removed in a future release"), stderr)
		})
	}
}

// A run refused over its flags says nothing about renames and records
// nothing: it never used the alias.
func TestARefusedRunNotesNoRenamedFlags(t *testing.T) {
	got := recordRenamedFlags(t)
	root, _ := disarmedTree(t, astroTree)
	_, stderr, err := executeRoot(root, "deployment", "list", "--workspace", "a", "--workspace-id", "b")
	require.ErrorContains(t, err, "disagree")
	assert.NotContains(t, stderr, "has been renamed")
	assert.Empty(t, *got)
}

// ASTRO_RENAME_HINTS_DISABLED hides the hint, for a script that must still
// run on 1.x, where the new spelling does not exist, but not the event.
func TestRenameHintsCanBeHidden(t *testing.T) {
	t.Setenv(utils.RenameHintsDisabledEnv, "1")
	got := recordRenamedFlags(t)
	root, _ := disarmedTree(t, astroTree)
	_, stderr, err := executeRoot(root, "deployment", "list", "--workspace-id", "ws")
	require.ErrorIs(t, err, errRan)
	assert.NotContains(t, stderr, "has been renamed")
	assert.Equal(t, []renamedFlagEvent{{"astro deployment list", []string{"--workspace-id"}, []string{"--workspace"}}}, *got)
}
