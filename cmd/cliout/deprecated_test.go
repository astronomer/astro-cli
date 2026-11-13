package cliout

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarnDeprecatedSaysEachSpellingOncePerRun(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	leaf := &cobra.Command{Use: "leaf"}
	root.AddCommand(leaf)
	var w bytes.Buffer

	WarnDeprecated(&w, leaf, "--deployment-id", "use --deployment")
	WarnDeprecated(&w, leaf, "--deployment-id", "use --deployment")
	WarnDeprecated(&w, root, "--deployment-id", "use --deployment")
	WarnDeprecated(&w, leaf, "--workspace-id", "use --workspace")
	WarnDeprecated(&w, leaf, "astro deployment logs scheduler", "use astro deployment logs --scheduler")

	assert.Equal(t, "Warning: --deployment-id is deprecated: use --deployment\n"+
		"Warning: --workspace-id is deprecated: use --workspace\n"+
		"Warning: astro deployment logs scheduler is deprecated: use astro deployment logs --scheduler\n", w.String())
}

func TestWarnDeprecatedFlagsWarnsOfTheFlagsTheRunGave(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("deployment-id", "", "")
	cmd.Flags().String("other", "", "")
	for _, alias := range []string{"workspace-id", "deployment-id"} {
		DeprecateFlag(cmd.Flags().Lookup(alias), "use --"+strings.TrimSuffix(alias, "-id"))
	}
	assert.True(t, cmd.Flags().Lookup("workspace-id").Hidden, "a deprecated spelling is not shown in help")
	_, ok := DeprecatedUse(cmd.Flags().Lookup("other"))
	assert.False(t, ok)

	require.NoError(t, cmd.ParseFlags([]string{"--other", "x", "--workspace-id", "a", "--workspace-id", "b", "--workspace", "c"}))
	var w bytes.Buffer
	WarnDeprecatedFlags(&w, cmd)
	assert.Equal(t, "Warning: --workspace-id is deprecated: use --workspace\n", w.String())
}

// A run is its own: Execute starts each with nothing said.
func TestExecuteClearsWhatTheLastRunSaid(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	var w bytes.Buffer
	leaf := &cobra.Command{Use: "leaf", RunE: func(cmd *cobra.Command, _ []string) error {
		WarnDeprecated(&w, cmd, "--old", "use --new")
		WarnDeprecated(&w, cmd, "--old", "use --new")
		return nil
	}}
	root.AddCommand(leaf)
	for range 2 {
		require.NoError(t, Execute(t.Context(), root, []string{"leaf"}, &bytes.Buffer{}, nil))
	}
	assert.Equal(t, 2, strings.Count(w.String(), "--old is deprecated: use --new"), w.String())
}
