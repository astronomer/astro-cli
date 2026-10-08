package cliout

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// groupRun is what a user gets from a group run: the error, its exit code,
// stdout and stderr.
type groupRun struct {
	err            error
	code           int
	stdout, stderr string
}

// newGroupTree is a root with one group, "thing", holding a visible leaf
// "list", a hidden one "secret", and a leaf "parenthelp" whose RunE prints
// its parent's help. withRunE gives the group GroupHelp as its RunE; without
// it the group is not runnable, as most groups are built.
func newGroupTree(withRunE bool) *cobra.Command {
	var output Format
	root := &cobra.Command{Use: "astro"}
	group := &cobra.Command{Use: "thing", Short: "Manage things"}
	if withRunE {
		group.Args = cobra.ArbitraryArgs
		group.RunE = GroupHelp
	}
	AddOutputFlag(group, &output)
	group.AddCommand(
		&cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "secret", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "parenthelp", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Parent().Help() }},
	)
	root.AddCommand(group)
	return root
}

func runGroup(t *testing.T, root *cobra.Command, args ...string) groupRun {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	ctx := context.Background()
	err := Execute(ctx, root, args, &stdout, testKinds)
	code := 0
	if err != nil {
		code = ExitCode(ctx, err)
	}
	return groupRun{err: err, code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// Both routes a group can take answer alike, so each case runs against both.
func forBothGroups(t *testing.T, fn func(t *testing.T, root func() *cobra.Command)) {
	for _, withRunE := range []bool{true, false} {
		name := "no RunE"
		if withRunE {
			name = "GroupHelp RunE"
		}
		t.Run(name, func(t *testing.T) { fn(t, func() *cobra.Command { return newGroupTree(withRunE) }) })
	}
}

// Bare in text mode, or with --help in either mode: its help, and success.
func TestABareGroupPrintsHelpInText(t *testing.T) {
	forBothGroups(t, func(t *testing.T, tree func() *cobra.Command) {
		for _, args := range [][]string{{"thing"}, {"thing", "--help"}, {"thing", "-o", "json", "--help"}} {
			r := runGroup(t, tree(), args...)
			require.NoError(t, r.err, args)
			assert.Contains(t, r.stdout, "Manage things", args)
		}
	})
}

// Bare under --output json: a usage error naming the visible subcommands, as
// the one object on stdout. Help is prose, which a json run's stdout must not
// carry.
func TestABareGroupIsAUsageErrorUnderJSON(t *testing.T) {
	forBothGroups(t, func(t *testing.T, tree func() *cobra.Command) {
		r := runGroup(t, tree(), "thing", "-o", "json")

		require.Error(t, r.err)
		assert.Equal(t, 2, r.code)
		obj := decodeOne(t, r.stdout)
		assert.Equal(t, 2, obj.Code)
		assert.Equal(t, KindUsage, obj.Kind)
		assert.Equal(t, `"astro thing" needs a subcommand: list, parenthelp`, obj.Error)
		assert.Empty(t, r.stderr)
	})
}

// An argument that names no subcommand is a usage error in both modes, with
// cobra's suggestions: in text mode too, where cobra on its own would print
// the help and succeed for a group with no RunE.
func TestAGroupRefusesAnUnknownSubcommandInBothModes(t *testing.T) {
	forBothGroups(t, func(t *testing.T, tree func() *cobra.Command) {
		r := runGroup(t, tree(), "thing", "lst")
		require.Error(t, r.err)
		assert.Equal(t, 2, r.code)
		assert.Contains(t, r.stderr, `unknown command "lst" for "astro thing"`)
		assert.Contains(t, r.stderr, "Did you mean this?\n\tlist")

		r = runGroup(t, tree(), "thing", "lst", "-o", "json")
		require.Error(t, r.err)
		obj := decodeOne(t, r.stdout)
		assert.Equal(t, 2, obj.Code)
		assert.Equal(t, KindUsage, obj.Kind)
		assert.Contains(t, obj.Error, "Did you mean this?\n\tlist")
	})
}

// Suggestions follow cobra's own refusal: none when the tree disables them,
// cobra's default distance when the group sets none, and the group's own
// distance left as it was.
func TestUnknownSubcommandSuggestsAsCobraDoes(t *testing.T) {
	group := func(t *testing.T, root *cobra.Command) *cobra.Command {
		t.Helper()
		g, _, err := root.Find([]string{"thing"})
		require.NoError(t, err)
		return g
	}

	root := newGroupTree(false)
	g := group(t, root)
	g.SuggestionsMinimumDistance = 7
	msg := UnknownSubcommand(g, "lsit").Error()
	assert.Contains(t, msg, "Did you mean this?\n\tlist", "two edits away is within cobra's default")
	assert.Equal(t, 7, g.SuggestionsMinimumDistance, "the group's own distance is put back")

	root = newGroupTree(false)
	root.SuggestionsMinimumDistance = 1
	assert.NotContains(t, UnknownSubcommand(group(t, root), "lsit").Error(), "Did you mean",
		"the root's distance is used, as cobra's own refusal uses it")

	root = newGroupTree(false)
	root.DisableSuggestions = true
	assert.NotContains(t, UnknownSubcommand(group(t, root), "lst").Error(), "Did you mean",
		"disabled on the root, as cobra reads it, so for the whole tree")

	root = newGroupTree(false)
	g = group(t, root)
	g.DisableSuggestions = true
	assert.NotContains(t, UnknownSubcommand(g, "lst").Error(), "Did you mean")
}

// A group whose every subcommand is hidden says so, rather than ending its
// message in an empty list.
func TestABareGroupWithNothingVisibleSaysSo(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	var output Format
	group := &cobra.Command{Use: "thing"}
	AddOutputFlag(group, &output)
	group.AddCommand(&cobra.Command{Use: "secret", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(group)

	r := runGroup(t, root, "thing", "-o", "json")

	assert.Equal(t, `"astro thing" has no command to run`, decodeOne(t, r.stdout).Error)
}

// Only cobra's own answer to a bare group is intercepted. A command that
// prints a group's help of its own accord, under json, gets the help printed
// and succeeds: it is not a bare group.
func TestHelpACommandPrintsIsNotABareGroup(t *testing.T) {
	forBothGroups(t, func(t *testing.T, tree func() *cobra.Command) {
		r := runGroup(t, tree(), "thing", "parenthelp", "-o", "json")

		require.NoError(t, r.err)
		assert.Equal(t, 0, r.code)
		assert.Contains(t, r.stdout, "Manage things")
	})
}
