package utils

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

func preferredFlagSet(t *testing.T, args ...string) (fs *pflag.FlagSet, id, name *string) {
	t.Helper()
	fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
	id, name = new(string), new(string)
	fs.StringVar(id, "deployment-id", "", "")
	fs.StringVar(name, "deployment-name", "", "")
	AddPreferredFlag(fs, "deployment", "", "", "deployment-id", "deployment-name")
	require.NoError(t, fs.Parse(args))
	return fs, id, name
}

func TestAddPreferredFlagHidesTheOldSpellings(t *testing.T) {
	fs, _, _ := preferredFlagSet(t)
	assert.True(t, fs.Lookup("deployment-id").Hidden)
	assert.True(t, fs.Lookup("deployment-name").Hidden)
	assert.False(t, fs.Lookup("deployment").Hidden)
}

func TestApplyPreferredFlags(t *testing.T) {
	toName := func(*pflag.FlagSet, string, string) (string, bool) { return "deployment-name", true }

	t.Run("the preferred value reaches the old spelling the route picks", func(t *testing.T) {
		fs, id, name := preferredFlagSet(t, "--deployment", "prod")
		require.NoError(t, ApplyPreferredFlags(fs, toName))
		assert.Empty(t, *id)
		assert.Equal(t, "prod", *name)
		assert.True(t, fs.Changed("deployment-name"))
	})

	t.Run("with no route it reaches the first old spelling", func(t *testing.T) {
		fs, id, name := preferredFlagSet(t, "--deployment", "prod")
		require.NoError(t, ApplyPreferredFlags(fs, nil))
		assert.Equal(t, "prod", *id)
		assert.Empty(t, *name)
	})

	t.Run("an old spelling alone marks the preferred flag given", func(t *testing.T) {
		fs, id, _ := preferredFlagSet(t, "--deployment-id", "cl123")
		require.NoError(t, ApplyPreferredFlags(fs, toName))
		assert.Equal(t, "cl123", *id)
		assert.True(t, fs.Changed("deployment"))
		assert.Equal(t, "cl123", fs.Lookup("deployment").Value.String())
	})

	t.Run("both spellings agreeing is no conflict", func(t *testing.T) {
		fs, _, name := preferredFlagSet(t, "--deployment", "prod", "--deployment-name", "prod")
		require.NoError(t, ApplyPreferredFlags(fs, toName))
		assert.Equal(t, "prod", *name)
	})

	t.Run("both spellings disagreeing is a usage error", func(t *testing.T) {
		fs, _, _ := preferredFlagSet(t, "--deployment", "a", "--deployment-id", "b")
		err := ApplyPreferredFlags(fs, toName)
		require.Error(t, err)
		assert.True(t, cliout.IsUsage(err))
		assert.EqualError(t, err, `--deployment "a" and --deployment-id "b" disagree: pass only --deployment`)
	})

	t.Run("a route may take the value itself", func(t *testing.T) {
		fs, id, name := preferredFlagSet(t, "--deployment", "prod")
		var took string
		require.NoError(t, ApplyPreferredFlags(fs, func(_ *pflag.FlagSet, _, v string) (string, bool) {
			took = v
			return "", true
		}))
		assert.Equal(t, "prod", took)
		assert.Empty(t, *id)
		assert.Empty(t, *name)
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
