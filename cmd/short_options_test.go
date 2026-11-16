package cmd

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/utils"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// walkCmd calls fn on cmd and every command below it, hidden ones included.
func walkCmd(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		walkCmd(sub, fn)
	}
}

// declaredFlags is every flag cmd declares itself, local and persistent,
// visible and hidden, and cobra's -h/--help, which a command grows when it
// is first shown. A flag a parent declared is the parent's.
func declaredFlags(cmd *cobra.Command) []*pflag.Flag {
	cmd.InitDefaultHelpFlag()
	var flags []*pflag.Flag
	seen := map[string]bool{}
	visit := func(f *pflag.Flag) {
		if !seen[f.Name] {
			seen[f.Name] = true
			flags = append(flags, f)
		}
	}
	cmd.LocalFlags().VisitAll(visit)
	cmd.PersistentFlags().VisitAll(visit)
	return flags
}

// flagsOf is every flag cmd answers to, its own and its parents'.
func flagsOf(cmd *cobra.Command) []*pflag.Flag {
	var flags []*pflag.Flag
	for c := cmd; c != nil; c = c.Parent() {
		for _, f := range declaredFlags(c) {
			if c == cmd || cmd.InheritedFlags().Lookup(f.Name) != nil {
				flags = append(flags, f)
			}
		}
	}
	return flags
}

// forEveryFlag calls fn on each distinct flag with a short option in every
// tree: the command that declares it, as its path says, and the flag.
func forEveryDeclaredFlag(t *testing.T, fn func(tree string, cmd *cobra.Command, f *pflag.Flag)) {
	t.Helper()
	testUtil.SetupOSArgsForGinkgo()
	for _, tree := range rootsUnderTest(t) {
		walkCmd(tree.root, func(cmd *cobra.Command) {
			for _, f := range declaredFlags(cmd) {
				fn(tree.name, cmd, f)
			}
		})
	}
}

// A short option means one thing everywhere. Every flag with one, visible or
// hidden, in every tree, is in utils.ShortOptions under its own long name; a
// new flag that takes a letter nobody listed fails here, as does one that
// takes a listed letter for another meaning. The API passthrough commands
// (utils.PassthroughPaths) are held to their family's table instead, with the
// main table behind it for the letters the family does not have, so a new
// command under astro api, astro af or astro local api takes the exemption by
// its path and is held to it.
func TestShortOptionsFollowTheTable(t *testing.T) {
	problems := map[string]bool{}
	letters := map[string]bool{}
	forEveryDeclaredFlag(t, func(tree string, cmd *cobra.Command, f *pflag.Flag) {
		if f.Shorthand == "" {
			return
		}
		family, exempt := passthroughOptions(pathBelowRoot(cmd))
		if exempt {
			if _, own := family[f.Shorthand]; own {
				letters[familyKey(family)+f.Shorthand] = true
				if family[f.Shorthand] == f.Name {
					return
				}
				problems[fmt.Sprintf("[%s] %s: --%s has -%s, which is --%s in its passthrough table", tree, cmd.CommandPath(), f.Name, f.Shorthand, family[f.Shorthand])] = true
				return
			}
		}
		letters[f.Shorthand] = true
		if utils.ShortOptions[f.Shorthand] == f.Name {
			return
		}
		want := "no flag has -" + f.Shorthand
		if long, ok := utils.ShortOptions[f.Shorthand]; ok {
			want = "-" + f.Shorthand + " is --" + long
		}
		problems[fmt.Sprintf("[%s] %s: --%s has -%s, but %s", tree, cmd.CommandPath(), f.Name, f.Shorthand, want)] = true
	})
	for _, p := range slices.Sorted(mapsKeys(problems)) {
		t.Error(p)
	}
	for letter, long := range utils.ShortOptions {
		assert.True(t, letters[letter], "-%s (--%s) is in the table but no flag in any tree takes it", letter, long)
	}
	for _, family := range []map[string]string{utils.APIShortOptions, utils.AfShortOptions} {
		for letter, long := range family {
			assert.True(t, letters[familyKey(family)+letter], "-%s (--%s) is in a passthrough table but no command of its family takes it", letter, long)
		}
	}
}

// familyKey names a passthrough table, by what only it holds.
func familyKey(family map[string]string) string {
	if _, ok := family["q"]; ok {
		return "api:"
	}
	return "af:"
}

// Every exempt path is a command of a tree, so a rename cannot leave the
// exemption standing on nothing, and every exempt command is in a family.
func TestPassthroughPathsNameRealCommands(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()
	seen := map[string]bool{}
	for _, tree := range rootsUnderTest(t) {
		walkCmd(tree.root, func(cmd *cobra.Command) {
			if _, ok := passthroughOptions(pathBelowRoot(cmd)); ok {
				for path := range utils.PassthroughPaths {
					if pathBelowRoot(cmd) == path {
						seen[path] = true
					}
				}
			}
		})
	}
	for path := range utils.PassthroughPaths {
		assert.True(t, seen[path], "astro %s is exempt but no tree has it", path)
	}
}

func mapsKeys[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// lettersWithheld are the commands that do not give a table letter to the
// flag that owns it, by "<command path below the root>|<flag>": the letter
// would be read as something else there. Astro CLI 1.x's own meaning for the
// letter is withheld without a line here (letterWas1xs), so only a command 1.x
// did not have appears.
var lettersWithheld = map[string]string{
	"deploy|workspace":        "astro deploy -w was --wait in 1.x: -w is refused on astro deploy and astro deploy bundle in every tree, so an old astro deploy -w <ID> fails instead of naming a Workspace",
	"deploy bundle|workspace": "see astro deploy: -w is refused on both",
}

// sameMeaning reports whether Astro CLI 1.x's flag named v1 is the flag f of
// v2: the same name, or one of the spellings 1.x had for it that v2 keeps as
// a hidden alias of f.
func sameMeaning(v1 string, f *pflag.Flag) bool {
	if v1 == f.Name {
		return true
	}
	switch f.Name {
	case "deployment":
		return v1 == "deployment-id" || v1 == "deployment-name"
	case "workspace":
		return v1 == "workspace-id"
	}
	return false
}

// reached is the commands flag f, declared on cmd, is given on: cmd, and for
// a persistent flag every command below it.
func reached(cmd *cobra.Command, f *pflag.Flag) []*cobra.Command {
	cmds := []*cobra.Command{cmd}
	if cmd.PersistentFlags().Lookup(f.Name) != nil {
		cmds = cmds[:0]
		walkCmd(cmd, func(c *cobra.Command) {
			// A command that declares a flag of its own by the name shadows
			// the persistent one, and has its own letter to answer for.
			c.InheritedFlags()
			if own := c.Flags().Lookup(f.Name); own == f || own == nil {
				cmds = append(cmds, c)
			}
		})
	}
	return cmds
}

// letterWas1xs is the 1.x flag that had letter, on cmd or on a command f
// reaches, with a meaning other than f's, and the command it was on; or nil.
func letterWas1xs(cmd *cobra.Command, letter string, f *pflag.Flag) (*v1Flag, *cobra.Command) {
	for _, c := range reached(cmd, f) {
		if v1 := v1FlagOn(c, letter, true); v1 != nil && !sameMeaning(v1.name, f) && !meaningChanged[pathBelowRoot(c)+"|"+letter] {
			return v1, c
		}
	}
	return nil, nil
}

// meaningChanged are the commands whose table letter means something other
// than it did in 1.x, by "<command path below the root>|<letter>", on purpose.
var meaningChanged = map[string]bool{
	// -o was --output-file's here. The command says where the path goes now
	// when -o is given one (cliout.OnBadFormat), so the old script is told.
	"organization audit-logs export|o": true,
}

// A table letter reaches every flag of its name that can take it. A command
// with --deployment has -d, one with --workspace -w, and so on: only where
// 1.x gave the letter to another flag on that command (astro deploy's -d was
// --dags, so a script is refused instead of naming a Deployment), or where
// lettersWithheld says why, does the flag go without.
func TestTableLettersReachEveryFlagThatCanTakeThem(t *testing.T) {
	long := map[string]string{}
	for letter, name := range utils.ShortOptions {
		long[name] = letter
	}
	withheld := map[string]bool{}
	forEveryDeclaredFlag(t, func(tree string, cmd *cobra.Command, f *pflag.Flag) {
		letter, inTable := long[f.Name]
		if !inTable || f.Shorthand != "" || utils.IsAlias(f) {
			return
		}
		key := pathBelowRoot(cmd) + "|" + f.Name
		if _, ok := lettersWithheld[key]; ok {
			withheld[key] = true
			return
		}
		if v1, _ := letterWas1xs(cmd, letter, f); v1 != nil {
			return
		}
		// A letter another flag of the command holds already (which
		// TestShortOptionsFollowTheTable would have refused) cannot be taken.
		for _, other := range flagsOf(cmd) {
			if other.Shorthand == letter {
				return
			}
		}
		t.Errorf("[%s] %s: --%s has no -%s; every command with --%s takes it, unless 1.x gave -%s to another flag there (lettersWithheld names the rest)", tree, cmd.CommandPath(), f.Name, letter, f.Name, letter)
	})
	for key := range lettersWithheld {
		assert.True(t, withheld[key], "lettersWithheld[%q] names a flag no command has without its letter", key)
	}
}

// A letter keeps its meaning from Astro CLI 1.x, or is gone. No flag takes a
// letter that 1.x gave a different flag on the same command, because the
// script that typed it would be read as saying something else and run, where
// the shorthand not being there refuses it by name (see
// TestEveryV1FlagStillWorksOrSaysWhatReplacedIt).
func TestNoShortOptionIsReadAsAnother1xFlag(t *testing.T) {
	forEveryDeclaredFlag(t, func(tree string, cmd *cobra.Command, f *pflag.Flag) {
		if f.Shorthand == "" || f.Shorthand == "h" {
			return
		}
		if v1, on := letterWas1xs(cmd, f.Shorthand, f); v1 != nil {
			t.Errorf("[%s] %s: -%s is --%s, and was --%s in 1.x on %s", tree, cmd.CommandPath(), f.Shorthand, f.Name, v1.name, on.CommandPath())
		}
	})
}

var architectureTableRow = regexp.MustCompile("^\\| `-(.)` \\| `--([a-z-]+)` \\|$")

// docs/architecture.md prints the table the code holds.
func TestShortOptionsTableIsDocumented(t *testing.T) {
	doc, err := os.ReadFile("../docs/architecture.md")
	require.NoError(t, err)
	got := map[string]string{}
	for line := range strings.Lines(string(doc)) {
		if m := architectureTableRow.FindStringSubmatch(strings.TrimRight(line, "\r\n")); m != nil {
			got[m[1]] = m[2]
		}
	}
	assert.Equal(t, utils.ShortOptions, got, "the short-option table in docs/architecture.md and utils.ShortOptions differ")
}

// passthroughOptions is the family the command at path (below the root, as
// "api airflow ls") is in, and whether it is exempt at all: its path, or one
// above it, is in utils.PassthroughPaths.
func passthroughOptions(path string) (map[string]string, bool) {
	for prefix, letters := range utils.PassthroughPaths {
		if path == prefix || strings.HasPrefix(path, prefix+" ") {
			return letters, true
		}
	}
	return nil, false
}
