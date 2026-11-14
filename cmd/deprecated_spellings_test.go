package cmd

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
)

// deprecatedAliasUse is one command and the hidden 1.x aliases it takes.
type deprecatedAliasUse struct {
	path    []string
	aliases []*pflag.Flag
}

// deprecatedAliasUses is every runnable command in the tree for c, hidden ones
// too, with the hidden 1.x aliases (utils.AddAlias) it takes, its own and those
// it inherits.
func deprecatedAliasUses(t *testing.T, c treeConfig) []deprecatedAliasUse {
	t.Helper()
	var uses []deprecatedAliasUse
	walkCmd(buildTree(t, c).root, func(cmd *cobra.Command) {
		// A removed command's stub (cliout.RemovedCommand, RemovedAlias) parses
		// no flags: it fails with its removal message and warns of nothing.
		if !cmd.Runnable() || !cmd.HasParent() || cmd.DisableFlagParsing || cmd.Annotations[cliout.RemovedCommandAnnotation] != "" {
			return
		}
		cmd.Flags().AddFlagSet(cmd.InheritedFlags())
		var aliases []*pflag.Flag
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if utils.IsAlias(f) {
				aliases = append(aliases, f)
			}
		})
		if len(aliases) > 0 {
			uses = append(uses, deprecatedAliasUse{strings.Fields(pathBelowRoot(cmd)), aliases})
		}
	})
	return uses
}

// warningLines is the deprecation warnings in stderr, each without its label.
func warningLines(stderr string) []string {
	var lines []string
	for line := range strings.Lines(stderr) {
		if rest, ok := strings.CutPrefix(line, "Warning: "); ok && strings.Contains(rest, " is deprecated: ") {
			lines = append(lines, strings.TrimSuffix(rest, "\n"))
		}
	}
	return lines
}

// Every hidden 1.x alias in every tree warns once per run, however often it
// is typed, and a command's different aliases warn once each, each naming what
// was typed and what replaces it. Nothing of it reaches stdout, which under
// --output json is still one document.
func TestEveryDeprecatedAliasWarnsOncePerRun(t *testing.T) {
	var checked int
	for _, c := range treeConfigs {
		uses := deprecatedAliasUses(t, c)
		require.NotEmpty(t, uses, c.name)
		for _, use := range uses {
			var args, want []string
			for _, f := range use.aliases {
				// Twice, which must warn once.
				args = append(args, "--"+f.Name+"=x", "--"+f.Name+"=x")
				want = append(want, "--"+f.Name+" is deprecated: use --"+utils.AliasOf(f))
			}
			slices.Sort(want)
			for _, asJSON := range []bool{false, true} {
				line := slices.Concat(use.path, args)
				if asJSON {
					line = append(line, "-o", "json")
				}
				root, _ := disarmedTree(t, c)
				target, _, err := root.Find(use.path)
				require.NoError(t, err)
				if asJSON && !cliout.HasOutput(target) {
					continue
				}
				stdout, stderr, _ := executeRoot(root, line...)
				got := warningLines(stderr)
				slices.Sort(got)
				assert.Equal(t, want, got, "[%s] astro %s", c.name, strings.Join(line, " "))
				assert.NotContains(t, stdout, "deprecated", "[%s] astro %s: nothing on stdout", c.name, strings.Join(line, " "))
				if asJSON {
					assert.Equal(t, 1, strings.Count(stdout, "\n"), "[%s] astro %s: stdout is one document: %q", c.name, strings.Join(line, " "), stdout)
					var doc map[string]any
					assert.NoError(t, json.Unmarshal([]byte(stdout), &doc), "%q", stdout)
				}
				checked++
			}
		}
	}
	assert.Greater(t, checked, 100, "the walk found few aliases; has the way an alias is marked changed?")
}

// Each alias alone, once, is one line; the same alias twice is still one.
func TestADeprecatedAliasTypedTwiceWarnsOnce(t *testing.T) {
	for _, tc := range []struct {
		tree treeConfig
		args []string
		want string
	}{
		{astroTree, []string{"deployment", "list", "--workspace-id", "a", "--workspace-id", "a"}, "--workspace-id is deprecated: use --workspace"},
		{astroTree, []string{"deployment", "inspect", "--deployment-name", "a", "--deployment-name", "b"}, "--deployment-name is deprecated: use --deployment"},
		{apcTree, []string{"workspace", "create", "--label", "a", "--label", "a"}, "--label is deprecated: use --name"},
	} {
		root, _ := disarmedTree(t, tc.tree)
		_, stderr, err := executeRoot(root, tc.args...)
		require.ErrorIs(t, err, errRan)
		assert.Equal(t, []string{tc.want}, warningLines(stderr), strings.Join(tc.args, " "))
	}
	// Two different ones are two lines, in the order typed.
	root, _ := disarmedTree(t, astroTree)
	_, stderr, err := executeRoot(root, "deployment", "variable", "list", "--deployment-name", "n", "--deployment-id", "i")
	require.ErrorIs(t, err, errRan)
	assert.Equal(t, []string{
		"--deployment-id is deprecated: use --deployment",
		"--deployment-name is deprecated: use --deployment",
	}, warningLines(stderr))
}

// A run is its own: a second run of the same tree warns again, since the
// warning is once per run, not once per process.
func TestADeprecationWarnsAgainInTheNextRun(t *testing.T) {
	root, _ := disarmedTree(t, astroTree)
	for range 2 {
		_, stderr, err := executeRoot(root, "deployment", "list", "--workspace-id", "a")
		require.ErrorIs(t, err, errRan)
		assert.Len(t, warningLines(stderr), 1)
	}
}

// The warning is the same whether the run succeeds or fails, and a removed
// spelling, which fails, does not warn.
func TestARemovedSpellingFailsWithoutWarning(t *testing.T) {
	root, _ := disarmedTree(t, astroTree)
	_, stderr, err := executeRoot(root, "deployment", "hibernate", "-n", "x", "--workspace-id", "ws")
	require.Error(t, err)
	assert.Equal(t, "-n was removed in Astro CLI v2: use --deployment", err.Error())
	assert.Empty(t, warningLines(stderr))
	assert.NotContains(t, stderr, "deprecated")
}

// A spelling is deprecated only where it is marked: the flags that are not
// aliases but are marked (cliout.DeprecateFlag) are listed here with the test
// that runs each, so a new one cannot go unchecked.
var deprecatedFlagsOutsideAliases = map[string]string{
	"secret": "TestTheSecretFlagIsADeprecatedAlias and TestADeprecatedSecretWarnsOnce in cmd/local",
}

func TestEveryDeprecatedFlagIsAnAliasOrHasItsOwnTest(t *testing.T) {
	seen := map[string]bool{}
	forEveryDeclaredFlag(t, func(tree string, cmd *cobra.Command, f *pflag.Flag) {
		if _, ok := cliout.DeprecatedUse(f); !ok {
			return
		}
		if utils.IsAlias(f) {
			assert.Equal(t, "use --"+utils.AliasOf(f), mustUse(f), "[%s] %s --%s", tree, cmd.CommandPath(), f.Name)
			return
		}
		seen[f.Name] = true
		_, ok := deprecatedFlagsOutsideAliases[f.Name]
		assert.True(t, ok, "[%s] %s --%s is deprecated, and no test is listed for it in deprecatedFlagsOutsideAliases", tree, cmd.CommandPath(), f.Name)
		assert.True(t, f.Hidden, "[%s] %s --%s: a deprecated spelling is not shown in help", tree, cmd.CommandPath(), f.Name)
	})
	for name := range deprecatedFlagsOutsideAliases {
		assert.True(t, seen[name], "deprecatedFlagsOutsideAliases lists --%s, which no command has deprecated", name)
	}
}

func mustUse(f *pflag.Flag) string {
	use, _ := cliout.DeprecatedUse(f)
	return use
}

// Every kept 1.x command alias is a supported spelling, like ls, and says
// nothing: a command is deprecated only where it is hidden and runs for the
// scripts of 1.x (APC's astro deployment logs scheduler, webserver and
// workers, which warn: see cmd/apc), so this list holds all of them and
// grows only with a decision.
var keptButDeprecatedCommands = map[string]bool{
	"astro deployment logs scheduler": true,
	"astro deployment logs webserver": true,
	"astro deployment logs workers":   true,
}

func TestOnlyKept1xSubcommandsAreDeprecatedCommands(t *testing.T) {
	seen := map[string]bool{}
	for _, tree := range rootsUnderTest(t) {
		walkCmd(tree.root, func(cmd *cobra.Command) {
			if cmd.Deprecated != "" {
				t.Errorf("[%s] %s uses cobra's Deprecated: use cliout.WarnDeprecated", tree.name, cmd.CommandPath())
			}
			if keptButDeprecatedCommands[cmd.CommandPath()] {
				seen[cmd.CommandPath()] = true
				assert.True(t, cmd.Hidden, "[%s] %s", tree.name, cmd.CommandPath())
			}
		})
	}
	for path := range keptButDeprecatedCommands {
		assert.True(t, seen[path], "%s is listed as a deprecated command but no tree has it", path)
	}
}
