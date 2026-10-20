package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hiddenFlagAllowed is each hidden flag that is not a flag 1.x had, and why
// it is hidden anyway. A key is "<command path below astro> --<flag>", and
// covers the command that registers the flag there and every command below it.
var hiddenFlagAllowed = map[string]string{
	"api cloud --spec-url":               "a developer override of the API spec, not for users",
	"api cloud --spec-token-env-var":     "a developer override of the API spec, not for users",
	"deployment variable create --key":   "hidden in 1.x too, so the inventory of visible flags leaves it out",
	"deployment variable create --value": "hidden in 1.x too, so the inventory of visible flags leaves it out",
	"deployment variable update --key":   "hidden in 1.x too, so the inventory of visible flags leaves it out",
	"deployment variable update --value": "hidden in 1.x too, so the inventory of visible flags leaves it out",
	"local env --secret":                 "deprecated in v2, so pflag hides it; --secret=false still means --plain",
	"env variable link --deployment-id": "1.x's link create had it, and link set took over; it shadows the noun's scope " +
		"flag of that name, so it names the link target",
	"env connection link --deployment-id":       "it shadows the noun's scope flag of that name, so it names the link target",
	"env airflow-variable link --deployment-id": "it shadows the noun's scope flag of that name, so it names the link target",
	"workspace team remove --workspace-id":      "came with the v2 --workspace; 1.x had it on APC only",
	"workspace team update --workspace-id":      "came with the v2 --workspace; 1.x had it on APC only",
}

// A hidden flag is a 1.x spelling a command keeps working for scripts written
// for 1.x, or it is on hiddenFlagAllowed with its reason. A new flag gets a v2
// spelling and shows in help: hiding one 1.x never had is either an alias on a
// command 1.x did not have, which no 1.x script can need, or a flag nobody can
// discover.
//
// A flag is checked at the command that registers it. A local flag is a 1.x
// flag when v1_flags.tsv has it on that command; a persistent one when it has
// it on that command or any below it, which inherit it. Its shorthand is
// 1.x's, or none where the shorthand moved to the v2 spelling (-w is
// --workspace now, as it was --workspace-id).
func TestEveryHiddenFlagIsA1xFlagOrSaysWhy(t *testing.T) {
	inventory, err := parseV1Flags(v1FlagsFile)
	require.NoError(t, err)
	used := map[string]bool{}
	for _, tc := range v1Trees {
		var walk func(c *cobra.Command)
		walk = func(c *cobra.Command) {
			for _, sub := range c.Commands() {
				walk(sub)
			}
			if c.DisableFlagParsing {
				return
			}
			c.LocalFlags().VisitAll(func(f *pflag.Flag) {
				if !f.Hidden {
					return
				}
				persistent := c.PersistentFlags().Lookup(f.Name) != nil
				if hadIn1x(inventory, tc.v1, pathSpellings(c), persistent, f) {
					return
				}
				path := strings.TrimPrefix(strings.TrimPrefix(c.CommandPath(), "astro"), " ")
				if key, ok := allowedHiddenFlag(path, f.Name); ok {
					used[key] = true
					return
				}
				key := path + " --" + f.Name
				assert.Failf(t, "a hidden flag 1.x did not have",
					"%s: astro %s (-%s) is hidden, and 1.x did not have it there. Show it, or say why it is hidden in hiddenFlagAllowed", tc.tree.name, key, f.Shorthand)
			})
		}
		walk(buildTree(t, tc.tree).root)
	}
	for key := range hiddenFlagAllowed {
		assert.True(t, used[key], "hiddenFlagAllowed names astro %s, which is no hidden flag 1.x did not have any more: delete the entry", key)
	}
}

// allowedHiddenFlag is the hiddenFlagAllowed key that covers --name
// registered at path: the key at path, or at a command above it.
func allowedHiddenFlag(path, name string) (string, bool) {
	words := strings.Fields(path)
	for i := len(words); i > 0; i-- {
		key := strings.Join(words[:i], " ") + " --" + name
		if _, ok := hiddenFlagAllowed[key]; ok {
			return key, true
		}
	}
	return "", false
}

// hadIn1x reports whether tree's inventory has f on a command at one of paths,
// or, for a persistent flag, on one below them.
func hadIn1x(inventory []v1Flag, tree string, paths []string, persistent bool, f *pflag.Flag) bool {
	return slices.ContainsFunc(inventory, func(v v1Flag) bool {
		if v.name != f.Name || !slices.Contains(v.trees, tree) || (f.Shorthand != "" && f.Shorthand != v.shorthand) {
			return false
		}
		return slices.ContainsFunc(paths, func(p string) bool {
			return v.path == p || (persistent && strings.HasPrefix(v.path, p+" "))
		})
	})
}
