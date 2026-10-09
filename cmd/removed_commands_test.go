package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

// v1CommandsFile is the 1.x command inventory. Unlike v1_flags.tsv, nothing
// in the binary reads it: a removed command is a stub in the tree, which
// needs no inventory to find.
var v1CommandsFile = filepath.Join("testdata", "v1_commands.tsv")

// v1Command is one line of v1_commands.tsv: a command path without "astro",
// and the 1.x trees that had it.
type v1Command struct {
	trees []string
	path  string
}

func readV1Commands(t *testing.T) []v1Command {
	t.Helper()
	data, err := os.ReadFile(v1CommandsFile)
	require.NoError(t, err)
	var cmds []v1Command
	for line := range strings.Lines(strings.ReplaceAll(string(data), "\r\n", "\n")) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		trees, path, ok := strings.Cut(line, "\t")
		require.True(t, ok, "malformed line in %s: %q", v1CommandsFile, line)
		cmds = append(cmds, v1Command{trees: strings.Split(trees, ","), path: path})
	}
	return cmds
}

// v1CommandTrees pairs each tree of v1_commands.tsv with every v2 tree a
// script written against it now runs. APC is checked at both ends of its
// version gates. Below them (belowGates), a command the platform is too old
// for is a hidden stub that names the version it needs (cmd/apc's removeCmd),
// as 1.x refused it too; the newest tree is where it must exist.
var v1CommandTrees = []struct {
	v1         string
	tree       treeConfig
	belowGates bool
}{
	{v1: v1TreeAstro, tree: astroTree},
	{v1: v1TreeAstroHosted, tree: astroHostedTree},
	{v1: v1TreeAPC, tree: apcTree},
	{v1: v1TreeAPC, tree: treeConfig{name: "apc " + oldestAPCVersion, platform: apcPlatform, apcVersion: oldestAPCVersion}, belowGates: true},
}

// removedStubShort starts the Short of every removed command's stub.
const removedStubShort = "Removed in v2"

// isRemovedCommandStub reports whether cmd is a removed command's stub (`astro
// dev`, `astro run`, `astro deployment pool`, `astro env variable create`,
// ...): hidden, so help teaches only what exists, with flag parsing off, so an
// old invocation's flags reach the guidance, and a Short saying it was
// removed. An APC command the platform is too old for is hidden with flag
// parsing off too, and says what version it needs instead.
func isRemovedCommandStub(cmd *cobra.Command) bool {
	return cmd.Hidden && cmd.DisableFlagParsing && strings.HasPrefix(cmd.Short, removedStubShort)
}

// isVersionGateStub reports whether cmd stands in for an APC command the
// platform is too old for.
func isVersionGateStub(cmd *cobra.Command) bool {
	return cmd.Hidden && cmd.DisableFlagParsing && !isRemovedCommandStub(cmd)
}

// stubTree builds c's tree with every hook and run disarmed, as disarmedTree
// does, except the removed commands' stubs, which keep their own run and
// pre-run: running one is what the test is for. The root's disarmed pre-run
// lets a command through that skips it in production
// (telemetry.SkipPreRunAnnotation), as the real one does; any other pre-run
// that starts is counted.
func stubTree(t *testing.T, c treeConfig) (root *cobra.Command, out *bytes.Buffer, preRuns *int) {
	t.Helper()
	root, out, preRuns = disarmedTreeKeeping(t, c, isRemovedCommandStub)
	disarmed := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Annotations[telemetry.SkipPreRunAnnotation] == "true" {
			return nil
		}
		return disarmed(cmd, args)
	}
	return root, out, preRuns
}

// Every command 1.x had still runs in v2, under its own name or a 1.x name it
// keeps as an alias, or reaches a hidden stub that says what replaced it, so a
// script written for 1.x is never told only "unknown command" or shown a
// group's help. A command dropped without a stub fails here, as does one
// renamed without its 1.x name kept as an alias.
//
// Each stub a 1.x path reaches is run, in text and under --output json, before
// any pre-run: a usage error (exit 2) whose message says it was removed and
// names the replacement or says there is none, and under json the one error
// object on stdout.
func TestEveryV1CommandStillRunsOrSaysWhatReplacedIt(t *testing.T) {
	commands := readV1Commands(t)
	for _, tc := range v1CommandTrees {
		t.Run(tc.tree.name, func(t *testing.T) {
			root, out, preRuns := stubTree(t, tc.tree)
			exists, gated, stubbed := 0, 0, 0
			for _, c := range commands {
				if !slices.Contains(c.trees, tc.v1) {
					continue
				}
				label := tc.tree.name + ": astro " + c.path
				cmd, rest, err := root.Find(strings.Fields(c.path))
				if !assert.NoError(t, err, "%s: unknown command", label) {
					continue
				}
				switch {
				case isRemovedCommandStub(cmd):
					stubbed++
					assertStubSaysWhatReplacedIt(t, root, out, preRuns, strings.Fields(c.path), label)
				case tc.belowGates && isVersionGateStub(cmd):
					gated++
				default:
					if !assert.Empty(t, rest, "%s: resolves to `%s` with %v left over, which names no command: "+
						"give a removed command a hidden stub that says what replaced it, and a renamed one its 1.x name as an alias", label, cmd.CommandPath(), rest) {
						continue
					}
					assert.Contains(t, pathSpellings(cmd), c.path, label)
					exists++
				}
			}
			t.Logf("%s: %d 1.x commands still run, %d need a newer platform, %d reach a removed command's stub", tc.tree.name, exists, gated, stubbed)
			assert.Greater(t, exists, 50, "%s: the inventory matched few commands; is v1_commands.tsv still read right?", tc.tree.name)
			if !tc.belowGates {
				assert.Zero(t, gated)
			}
		})
	}
}

// assertStubSaysWhatReplacedIt runs args, which reach a removed command's
// stub, in text and under --output json. What it publishes is read from the
// writer the root was built with (out) and the one it is run with, which are
// both os.Stdout in production.
func assertStubSaysWhatReplacedIt(t *testing.T, root *cobra.Command, out *bytes.Buffer, preRuns *int, args []string, label string) {
	t.Helper()
	run := func(args ...string) (stdout, stderr string, err error) {
		out.Reset()
		stdout, stderr, err = executeRoot(root, args...)
		return out.String() + stdout, stderr, err
	}
	before := *preRuns
	stdout, stderr, err := run(args...)
	if !assert.Error(t, err, label) {
		return
	}
	assert.NotErrorIs(t, err, errRan, "%s: ran a disarmed command", label)
	msg := err.Error()
	assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err), "%s: not a usage error: %v", label, err)
	assert.Equal(t, before, *preRuns, "%s: a pre-run started before the stub", label)
	_, guidance, removed := strings.Cut(msg, "was removed in Astro CLI v2")
	if assert.True(t, removed, "%s: does not say it was removed in Astro CLI v2: %s", label, msg) {
		assert.True(t, namesReplacement(guidance), "%s: names no replacement, and does not say there is none: %s", label, msg)
	}
	assert.Empty(t, stdout, label)
	assert.Contains(t, stderr, "Error: "+msg, label)

	stdout, stderr, err = run(append(slices.Clone(args), "-o", "json")...)
	if !assert.Error(t, err, label+" -o json") {
		return
	}
	assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err), "%s -o json: not a usage error: %v", label, err)
	assert.Equal(t, before, *preRuns, "%s -o json: a pre-run started before the stub", label)
	assert.Empty(t, stderr, label+" -o json")
	if !assert.Equal(t, 1, strings.Count(stdout, "\n"), "%s -o json: stdout is not one line: %q", label, stdout) {
		return
	}
	var obj map[string]any
	if !assert.NoError(t, json.Unmarshal([]byte(stdout), &obj), "%s -o json: %s", label, stdout) {
		return
	}
	assert.Equal(t, err.Error(), obj["error"], label+" -o json")
	assert.Contains(t, obj["error"], "was removed in Astro CLI v2", label+" -o json")
	assert.Equal(t, float64(cliout.ExitUsage), obj["code"], label+" -o json")
	assert.Equal(t, string(cliout.KindUsage), obj["kind"], label+" -o json")
	if args[0] != "dev" {
		// `astro dev` publishes the mapping from every 1.x dev command to
		// its replacement besides (cmd/local/testdata/schema/dev-removed.json).
		assert.Len(t, obj, 3, "%s -o json: not the error object: %s", label, stdout)
	}
}

// namesReplacement reports whether the guidance after "was removed in Astro
// CLI v2" names an astro command to use instead, or says there is none.
func namesReplacement(guidance string) bool {
	for _, s := range []string{"`astro ", "use:  astro ", "no direct replacement", "no replacement"} {
		if strings.Contains(guidance, s) {
			return true
		}
	}
	return false
}

// The inventory parses, names only the trees there are, and is in order, so a
// regenerated one diffs line by line.
func TestV1CommandsFileIsWellFormed(t *testing.T) {
	data, err := os.ReadFile(v1CommandsFile)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "\r", "v1_commands.tsv has CRLF line ends: is testdata still text eol=lf in .gitattributes?")
	commands := readV1Commands(t)
	require.Greater(t, len(commands), 200)
	known := []string{v1TreeAstro, v1TreeAstroHosted, v1TreeAPC}
	seen := map[string]int{}
	var paths []string
	for _, c := range commands {
		for _, tree := range c.trees {
			assert.Contains(t, known, tree, c.path)
			seen[tree]++
		}
		paths = append(paths, c.path)
	}
	assert.Len(t, seen, len(known), "a tree has no commands")
	assert.True(t, slices.IsSorted(paths), "v1_commands.tsv is not sorted by command path")
	assert.Len(t, slices.Compact(slices.Clone(paths)), len(paths), "v1_commands.tsv lists a path twice")
}

// The stubs a 1.x command reaches are the stubs there are: one nothing in the
// inventory reaches is either a 1.x command the inventory lacks, or no 1.x
// command at all, and so not guarded by the test above.
func TestEveryRemovedCommandStubIsA1xCommand(t *testing.T) {
	commands := readV1Commands(t)
	reached := map[string]bool{}
	var all []string
	for _, tc := range v1CommandTrees {
		root := buildTree(t, tc.tree).root
		walkCmd(root, func(cmd *cobra.Command) {
			if isRemovedCommandStub(cmd) && !slices.Contains(all, cmd.CommandPath()) {
				all = append(all, cmd.CommandPath())
			}
		})
		for _, c := range commands {
			if !slices.Contains(c.trees, tc.v1) {
				continue
			}
			if cmd, _, err := root.Find(strings.Fields(c.path)); err == nil && isRemovedCommandStub(cmd) {
				reached[cmd.CommandPath()] = true
			}
		}
	}
	require.NotEmpty(t, all)
	for _, path := range all {
		assert.True(t, reached[path], "%s is a removed command's stub that no 1.x command in v1_commands.tsv reaches", path)
	}
}
