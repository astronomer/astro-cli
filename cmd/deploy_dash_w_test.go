package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// -w was --wait on astro deploy, and means --workspace everywhere else, so
// astro deploy and astro deploy bundle never give it to --workspace: an old
// astro deploy -w <ID> fails, saying so, in every spelling and in both
// formats, for as long as v2 lives (the removedFlags entry with shorthand w).
func TestDeployRefusesDashWAndNeverReadsItAsAWorkspace(t *testing.T) {
	for _, tc := range []treeConfig{astroTree, astroHostedTree} {
		for _, line := range [][]string{
			{"deploy", "-w"},
			{"deploy", "-w", "cl123"},
			{"deploy", "cl123", "-w"},
			{"deploy", "bundle", "-w"},
			{"deploy", "bundle", "./dbt", "-w"},
			{"deploy", "bundle", "./dbt", "--deployment", "cl123", "-w"},
		} {
			for _, asJSON := range []bool{false, true} {
				args := slices.Clone(line)
				if asJSON {
					args = append(args, "-o", "json")
				}
				t.Run(tc.name+": astro "+strings.Join(args, " "), func(t *testing.T) {
					root, preRuns := disarmedTree(t, tc)
					stdout, stderr, err := executeRoot(root, args...)
					require.Error(t, err)
					want := "-w was removed in Astro CLI v2: use --wait"
					assert.Equal(t, want, err.Error())
					assert.True(t, cliout.IsUsage(err))
					assert.Zero(t, *preRuns, "a pre-run started before the refusal")
					if asJSON {
						assert.Empty(t, stderr)
						assert.Contains(t, stdout, want)
					} else {
						assert.Empty(t, stdout)
					}
				})
			}
		}
		t.Run(tc.name+": --workspace is still there, without a letter", func(t *testing.T) {
			root, _ := disarmedTree(t, tc)
			for _, path := range [][]string{{"deploy"}, {"deploy", "bundle"}} {
				cmd, _, err := root.Find(path)
				require.NoError(t, err)
				f := cmd.Flags().Lookup("workspace")
				require.NotNil(t, f, cmd.CommandPath())
				assert.Empty(t, f.Shorthand, cmd.CommandPath())
				assert.Nil(t, cmd.Flags().ShorthandLookup("w"), cmd.CommandPath())
			}
		})
	}
	t.Run("the entry is there for the lifetime of v2", func(t *testing.T) {
		var found bool
		for _, f := range removedFlags {
			found = found || (f.shorthand == "w" && f.name == "wait")
		}
		assert.True(t, found, "removedFlags has no entry for -w on astro deploy")
	})
}
