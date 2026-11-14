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

// A 1.x spelling v2 keeps as a hidden alias says on stderr, in one line, that
// it is deprecated and what to type instead, and is recorded, all of a run's
// in one event, so a script gets updated and we can see when the alias can go.
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
			[]string{"--workspace-id is deprecated: use --workspace"},
		},
		{
			astroTree,
			[]string{"deployment", "inspect", "--deployment-name", "dep"},
			&renamedFlagEvent{"astro deployment inspect", []string{"--deployment-name"}, []string{"--deployment"}},
			[]string{"--deployment-name is deprecated: use --deployment"},
		},
		{
			apcTree,
			[]string{"deployment", "service-account", "list", "--deployment-id", "dep-id"},
			&renamedFlagEvent{"astro deployment service-account list", []string{"--deployment-id"}, []string{"--deployment"}},
			[]string{"--deployment-id is deprecated: use --deployment"},
		},
		{
			apcTree,
			[]string{"workspace", "create", "--label", "etl"},
			&renamedFlagEvent{"astro workspace create", []string{"--label"}, []string{"--name"}},
			[]string{"--label is deprecated: use --name"},
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
				"--deployment-id is deprecated: use --deployment",
				"--deployment-name is deprecated: use --deployment",
				"--workspace-id is deprecated: use --workspace",
			},
		},
		// "" still means the default through the alias, as in 1.x, and the
		// spelling is deprecated all the same.
		{
			astroTree,
			[]string{"deployment", "list", "--workspace-id", ""},
			&renamedFlagEvent{"astro deployment list", []string{"--workspace-id"}, []string{"--workspace"}},
			[]string{"--workspace-id is deprecated: use --workspace"},
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
			// Each warning is one line, led by a plain "Warning:" here, where
			// stderr is no terminal and so gets no color, and the run's own
			// failure follows.
			var warnings []string
			for line := range strings.Lines(stderr) {
				if strings.HasPrefix(line, "Warning: ") {
					warnings = append(warnings, strings.TrimSuffix(strings.TrimPrefix(line, "Warning: "), "\n"))
				}
			}
			assert.Equal(t, tc.hints, warnings, stderr)
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
	assert.NotContains(t, stderr, "is deprecated")
	assert.Empty(t, *got)
}

// Nothing turns the warning off: the variable that used to hide it does
// nothing now, and the event goes as before.
func TestNothingHidesTheDeprecationWarning(t *testing.T) {
	t.Setenv("ASTRO_RENAME_HINTS_DISABLED", "1")
	got := recordRenamedFlags(t)
	root, _ := disarmedTree(t, astroTree)
	_, stderr, err := executeRoot(root, "deployment", "list", "--workspace-id", "ws")
	require.ErrorIs(t, err, errRan)
	assert.Contains(t, stderr, "--workspace-id is deprecated: use --workspace")
	assert.Equal(t, []renamedFlagEvent{{"astro deployment list", []string{"--workspace-id"}, []string{"--workspace"}}}, *got)
}
