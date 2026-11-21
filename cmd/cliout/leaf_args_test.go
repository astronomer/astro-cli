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
	assert.EqualError(t, UndeclaredArgs(one, []string{"a", "b"}), `unexpected argument "b": get takes at most 1 argument`)
}

// leafArgsError reads cobra's English, so the messages it reads are pinned: a
// cobra upgrade that rewords one fails here, not by quietly bringing back
// "unknown command" for a stray argument.
func TestCobraArgsMessagesAreRecognized(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	for name, tc := range map[string]struct {
		validate cobra.PositionalArgs
		args     []string
		want     string
	}{
		"NoArgs":         {cobra.NoArgs, []string{"a"}, `unexpected argument "a": x takes no arguments`},
		"MaximumNArgs 1": {cobra.MaximumNArgs(1), []string{"a", "b"}, `unexpected argument "b": x takes at most 1 argument`},
		"MaximumNArgs 2": {cobra.MaximumNArgs(2), []string{"a", "b", "c"}, `unexpected argument "c": x takes at most 2 arguments`},
		"ExactArgs":      {cobra.ExactArgs(2), []string{"a", "b", "c"}, `unexpected argument "c": x takes exactly 2 arguments`},
		"RangeArgs":      {cobra.RangeArgs(1, 2), []string{"a", "b", "c"}, `unexpected argument "c": x takes 1 to 2 arguments`},
	} {
		err := tc.validate(cmd, tc.args)
		require.Error(t, err, name)
		assert.EqualError(t, leafArgsError(cmd, tc.args, err), tc.want, name)
	}
	// Too few is not a stray argument, so cobra's own message stays.
	err := cobra.ExactArgs(2)(cmd, []string{"a"})
	assert.Equal(t, err, leafArgsError(cmd, []string{"a"}, err))
}
