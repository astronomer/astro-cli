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
	root := build(&ran)
	stub, _, err := root.Find([]string{"deployment", "de"})
	require.NoError(t, err)
	assert.True(t, IsRemovedCommand(stub))
	assert.True(t, stub.Hidden)
	assert.Equal(t, "delete", RemovedAliasTarget(stub).Name())
	// Until Execute walks the built tree, the Short names nothing; then the
	// command, by its whole path.
	assert.Equal(t, "Removed in v2", stub.Short)
	markUsageErrors(root)
	assert.Equal(t, "Removed in v2 — use astro deployment delete, or its alias rm", stub.Short)
}

// The guidance names the command by its whole path with every alias it has,
// however many.
func TestRemovedAliasNamesEveryAlias(t *testing.T) {
	for _, tc := range []struct {
		aliases []string
		want    string
	}{
		{nil, "astro context delete"},
		{[]string{"rm"}, "astro context delete, or its alias rm"},
		{[]string{"rm", "remove"}, "astro context delete, or one of its aliases rm, remove"},
	} {
		root := &cobra.Command{Use: "astro", SilenceErrors: true, SilenceUsage: true}
		group := &cobra.Command{Use: "context"}
		del := &cobra.Command{Use: "delete", Aliases: tc.aliases, RunE: func(*cobra.Command, []string) error { return nil }}
		stub := RemovedAlias("de", del)
		group.AddCommand(del, stub)
		root.AddCommand(group)
		markUsageErrors(root)
		assert.Equal(t, "Removed in v2 — use "+tc.want, stub.Short)
		root.SetArgs([]string{"context", "de"})
		assert.EqualError(t, root.Execute(), "astro context de was removed in Astro CLI v2: de is no longer short for delete. Use "+tc.want+".")
	}
}
