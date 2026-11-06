package cliout

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A dropped alias reaches its stub, which fails as a usage error naming the
// command it spelled, and never runs that command; the command's own spellings
// still run it.
func TestRemovedAliasNamesTheCommandAndRunsNothing(t *testing.T) {
	build := func(ran *bool) *cobra.Command {
		root := &cobra.Command{Use: "astro", SilenceErrors: true, SilenceUsage: true}
		group := &cobra.Command{Use: "deployment", Aliases: []string{"de"}}
		del := &cobra.Command{
			Use:     "delete [DEPLOYMENT_ID]",
			Aliases: []string{"rm"},
			RunE:    func(*cobra.Command, []string) error { *ran = true; return nil },
		}
		del.Flags().Bool("force", false, "")
		group.AddCommand(del, RemovedAlias("de", del))
		root.AddCommand(group)
		return root
	}

	for _, args := range [][]string{
		{"deployment", "de", "d1", "--force"},
		{"de", "de", "d1", "-f", "--deployment-id", "d1"},
	} {
		var ran bool
		root := build(&ran)
		root.SetArgs(args)
		err := root.Execute()
		require.Error(t, err, args)
		assert.False(t, ran, "%v ran the delete", args)
		assert.True(t, IsUsage(err), "%v: not a usage error: %v", args, err)
		assert.Equal(t, "astro deployment de was removed in Astro CLI v2: de is no longer short for delete. "+
			"Use astro deployment delete, or its alias rm.", err.Error())
	}

	for _, spelling := range []string{"delete", "rm"} {
		var ran bool
		root := build(&ran)
		root.SetArgs([]string{"de", spelling, "d1", "--force"})
		require.NoError(t, root.Execute(), spelling)
		assert.True(t, ran, spelling)
	}

	var ran bool
	stub, _, err := build(&ran).Find([]string{"deployment", "de"})
	require.NoError(t, err)
	assert.True(t, IsRemovedCommand(stub))
	assert.True(t, stub.Hidden)
	assert.Equal(t, "Removed in v2 — use delete or rm", stub.Short)
}
