package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A line with positional words or flag values still gets the document: the
// early answer leaves those to cobra, whose help func answers them.
func TestHelpJSONWithOperandsAndFlagValues(t *testing.T) {
	for args, path := range map[string]string{
		"deployment inspect my-dep --help -o json":         "astro deployment inspect",
		"deployment create foo --help -o json":             "astro deployment create",
		"deployment list --workspace x --help -o json":     "astro deployment list",
		"deployment list --workspace x -o json -h":         "astro deployment list",
		"--verbosity debug deployment list --help -o json": "astro deployment list",
	} {
		t.Run(args, func(t *testing.T) {
			stdout, stderr, err := runHelpLine(t, strings.Fields(args)...)
			require.NoError(t, err)
			assert.Empty(t, stderr)
			assert.Equal(t, path, decodeHelpDocument(t, stdout).Path)
		})
	}
	// A word that names no command under a group is still refused.
	// (--help on the line instead describes the group, as the text help shows it.)
	stdout, _, err := runHelpLine(t, "help", "deployment", "bogus", "-o", "json")
	require.Error(t, err)
	assert.Equal(t, "usage", oneErrorDocument(t, stdout).Kind)
}

// The three ways to ask are one answer, for a leaf and for a group.
func TestHelpJSONIsTheSameWhicheverWayItIsAsked(t *testing.T) {
	for _, path := range [][]string{{"deployment", "list"}, {"deployment"}} {
		viaFlag, _, err := runHelpLine(t, append(append([]string{}, path...), "--help", "-o", "json")...)
		require.NoError(t, err)
		viaHelp, _, err := runHelpLine(t, append(append([]string{"help"}, path...), "-o", "json")...)
		require.NoError(t, err)
		allOut, _, err := runHelpLine(t, "help", "--all", "-o", "json")
		require.NoError(t, err)
		var all helpAllDocument
		require.NoError(t, json.Unmarshal([]byte(allOut), &all))
		var fromAll *helpDocument
		for i := range all.Commands {
			if all.Commands[i].Path == "astro "+strings.Join(path, " ") {
				fromAll = &all.Commands[i]
			}
		}
		require.NotNil(t, fromAll)
		want := decodeHelpDocument(t, viaFlag)
		assert.Equal(t, want, decodeHelpDocument(t, viaHelp), strings.Join(path, " "))
		assert.Equal(t, want, *fromAll, strings.Join(path, " "))
	}
}

func TestDescribingACommandDoesNotChangeIt(t *testing.T) {
	root, _ := disarmedTree(t, astroTree)
	target, _, err := root.Find([]string{"deployment", "list"})
	require.NoError(t, err)
	assert.Nil(t, target.Flags().Lookup(helpName))
	describeCommand(target)
	assert.Nil(t, target.Flags().Lookup(helpName))
}

// A removed 1.x flag does not stop JSON help, as it does not stop the text.
func TestHelpJSONGivesWayToRemovedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "list", "--json", "--help"},
		{"deployment", "list", "--help", "--json"},
		{"deployment", "list", "--json", "-h"},
		{"deployment", "list", "--json", "--help=true"},
		{"deployment", "delete", "dep-id", "-f", "-h"},
		{"deployment", "delete", "dep-id", "-hf"},
		{"init", "--runtime-version", "12.0.0", "--help"},
		{"local", "start", "-n", "-h"},
		{"init", "-n", "my-project", "-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := runHelpLine(t, append(append([]string{}, args...), "-o", "json")...)
			require.NoError(t, err)
			assert.Empty(t, stderr)
			root, _ := disarmedTree(t, astroTree)
			target, _, err := root.Find(withoutHelpFlags(args))
			require.NoError(t, err)
			assert.Equal(t, target.CommandPath(), decodeHelpDocument(t, stdout).Path)
		})
	}
}

func TestHelpSearchIgnoresTheBinaryName(t *testing.T) {
	stdout, _, err := runHelpLine(t, "help", "search", "astro", "deployment", "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "astro deployment list\t")
}

func TestHelpArgumentsHandleEveryUseLineInBothTrees(t *testing.T) {
	for _, tree := range rootsUnderTest(t) {
		for _, c := range visibleCommands(tree.root) {
			for _, a := range helpArguments(c.Use) {
				assert.NotEmpty(t, a.Name, "%s: %s", tree.name, c.Use)
				assert.False(t, strings.ContainsAny(a.Name, " |<>[]"), "%s: %s -> %q", tree.name, c.Use, a.Name)
				assert.False(t, strings.HasPrefix(a.Name, "-"), "%s: %s", tree.name, c.Use)
			}
		}
	}
}

func TestHelpArgumentsNestedAndOddTokens(t *testing.T) {
	for use, want := range map[string][]helpArg{
		"add [NAME [-- COMMAND...]]": {{"NAME", false, false}, {"COMMAND", false, true}},
		"set [a|b]":                  {{"a", false, false}},
		"x [--flag value] <ID>":      {{"ID", true, false}},
		"x [flags] [command]":        {},
		"run COMMAND":                {{"COMMAND", true, false}},
		"run -- COMMAND...":          {{"COMMAND", true, true}},
	} {
		assert.Equal(t, want, helpArguments(use), use)
	}
}
