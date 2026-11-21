package cliout

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stray argument to a leaf is not an unknown command, whether the leaf
// declares nothing or states its own cobra validator.
func TestLeafStrayArgumentIsNotAnUnknownCommand(t *testing.T) {
	for name, args := range map[string]cobra.PositionalArgs{"undeclared": nil, "NoArgs": cobra.NoArgs} {
		root := &cobra.Command{Use: "astro"}
		leaf := &cobra.Command{Use: "version", Args: args, RunE: func(*cobra.Command, []string) error { return nil }}
		root.AddCommand(leaf)
		r := execTree(context.Background(), root, "version", "extra")
		require.Error(t, r.err, name)
		assert.True(t, IsUsage(r.err), name)
		assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err), name)
		assert.Equal(t, "Error: unexpected argument \"extra\": astro version takes no arguments\nRun astro version --help for usage.\n", r.stderr, name)
	}
	one := &cobra.Command{Use: "get [NAME]", RunE: func(*cobra.Command, []string) error { return nil }}
	assert.EqualError(t, UndeclaredArgs(one, []string{"a", "b"}), `unexpected argument "b": get takes at most 1`)
}
