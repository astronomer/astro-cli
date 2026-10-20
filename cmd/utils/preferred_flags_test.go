package utils

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

func aliasedFlagSet(t *testing.T, args ...string) (fs *pflag.FlagSet, workspace *string) {
	t.Helper()
	fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
	workspace = new(string)
	fs.StringVar(workspace, "workspace", "", "")
	AddAlias(fs, "workspace-id", "w", "workspace")
	require.NoError(t, fs.Parse(args))
	return fs, workspace
}

func TestAddAliasHidesTheOldSpelling(t *testing.T) {
	fs, _ := aliasedFlagSet(t)
	assert.True(t, fs.Lookup("workspace-id").Hidden)
	assert.True(t, IsAlias(fs.Lookup("workspace-id")))
	assert.False(t, fs.Lookup("workspace").Hidden)
	assert.False(t, IsAlias(fs.Lookup("workspace")))
	assert.Equal(t, "w", fs.Lookup("workspace-id").Shorthand)
}

func TestApplyAliases(t *testing.T) {
	t.Run("the alias reaches the flag it stands for", func(t *testing.T) {
		for _, args := range [][]string{{"--workspace-id", "ws"}, {"-w", "ws"}} {
			fs, workspace := aliasedFlagSet(t, args...)
			require.NoError(t, ApplyAliases(fs))
			assert.Equal(t, "ws", *workspace)
			assert.True(t, fs.Changed("workspace"), "a required check on --workspace is met")
		}
	})

	t.Run("the preferred spelling alone is left as it is", func(t *testing.T) {
		fs, workspace := aliasedFlagSet(t, "--workspace", "ws")
		require.NoError(t, ApplyAliases(fs))
		assert.Equal(t, "ws", *workspace)
		assert.False(t, fs.Changed("workspace-id"))
	})

	t.Run("both spellings agreeing is no conflict", func(t *testing.T) {
		fs, workspace := aliasedFlagSet(t, "--workspace", "ws", "--workspace-id", "ws")
		require.NoError(t, ApplyAliases(fs))
		assert.Equal(t, "ws", *workspace)
	})

	t.Run("both spellings disagreeing is a usage error", func(t *testing.T) {
		fs, _ := aliasedFlagSet(t, "--workspace", "a", "--workspace-id", "b")
		err := ApplyAliases(fs)
		require.Error(t, err)
		assert.True(t, cliout.IsUsage(err))
		assert.EqualError(t, err, `--workspace "a" and --workspace-id "b" disagree: pass only --workspace`)
	})

	t.Run("two aliases of one flag need not agree, and the first is handed on", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		deployment := new(string)
		fs.StringVar(deployment, "deployment", "", "")
		AddAlias(fs, "deployment-id", "d", "deployment")
		AddAlias(fs, "deployment-name", "n", "deployment")
		require.NoError(t, fs.Parse([]string{"-n", "prod", "-d", "cl123"}))
		require.NoError(t, ApplyAliases(fs))
		assert.Equal(t, "cl123", *deployment)
	})
}

// BeforeArgs runs ahead of a parent's persistent pre-run, which is where the
// Astro tree's project lookup reads the old spellings, and keeps the
// command's own argument check.
func TestBeforeArgsRunsAheadOfThePreRuns(t *testing.T) {
	var order []string
	root := &cobra.Command{
		Use: "root",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			order = append(order, "pre-run")
			return nil
		},
	}
	leaf := &cobra.Command{
		Use:  "leaf",
		Args: cobra.MaximumNArgs(1),
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(leaf)
	BeforeArgs(root, func(*cobra.Command, []string) error {
		order = append(order, "hook")
		return nil
	})

	root.SetArgs([]string{"leaf", "x"})
	require.NoError(t, root.Execute())
	assert.Equal(t, []string{"hook", "pre-run"}, order)

	root.SetArgs([]string{"leaf", "x", "y"})
	assert.ErrorContains(t, root.Execute(), "accepts at most 1 arg(s)")
	// The group itself is not runnable and keeps cobra's own argument check.
	assert.Nil(t, root.Args)
}
