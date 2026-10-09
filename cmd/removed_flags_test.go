package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrocontext "github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The trees the removed-flag tests run: Astro for a non-hosted organization
// and for a hosted one, and APC at the newest platform version, where every
// version-gated command is mounted. Each test builds its own, so it runs
// against that tree's state.
var (
	astroTree       = treeConfig{name: "astro", platform: cloudPlatform}
	astroHostedTree = treeConfig{name: "astro hosted", platform: cloudPlatform, hosted: true}
	apcTree         = treeConfig{name: "apc " + newestAPCVersion, platform: apcPlatform, apcVersion: newestAPCVersion}
)

// v1Trees pairs each tree of v1_flags.tsv with the v2 tree a script written
// against it now runs.
var v1Trees = []struct {
	v1   string
	tree treeConfig
}{
	{v1TreeAstro, astroTree},
	{v1TreeAstroHosted, astroHostedTree},
	{v1TreeAPC, apcTree},
}

var errRan = errors.New("the command got past flag parsing")

// disarmedTree builds c's tree with every hook and run replaced, so a run
// that gets past flag parsing ends with errRan before it logs in, prompts or
// asks an API anything. preRuns counts the pre-runs that started.
func disarmedTree(t *testing.T, c treeConfig) (root *cobra.Command, preRuns *int) {
	t.Helper()
	root = buildTree(t, c).root
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	preRuns = new(int)
	var disarm func(*cobra.Command)
	disarm = func(cmd *cobra.Command) {
		cmd.PersistentPreRun, cmd.PreRun, cmd.PreRunE = nil, nil, nil
		cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
			*preRuns++
			return errRan
		}
		if cmd.Runnable() {
			cmd.Run = nil
			cmd.RunE = func(*cobra.Command, []string) error { return errRan }
		}
		for _, sub := range cmd.Commands() {
			disarm(sub)
		}
	}
	disarm(root)
	return root, preRuns
}

// removedFlagCase is a run passing a removed flag on a command that had it
// in 1.x and does not now, and what it must be told.
type removedFlagCase struct {
	tree treeConfig
	args []string // the command, and its arguments before the flag
	flag string   // the removed flag's name
	// value is a value to pass the flag, for one that took one in 1.x;
	// empty for a 1.x boolean.
	value     string
	shorthand string
	want      string
}

// removedFlagCases holds at least one case for every entry in removedFlags
// (TestRemovedFlagsSayWhatReplacedThem checks that), with the message it
// gives there.
var removedFlagCases = []removedFlagCase{
	{tree: astroTree, args: []string{"deployment", "create", "--name", "x"}, flag: "deployment-file", value: "deployment.yaml", want: errDeploymentFileRemoved},
	{tree: astroHostedTree, args: []string{"deployment", "update", "dep-id"}, flag: "deployment-file", value: "deployment.yaml", want: errDeploymentFileRemoved},
	{tree: astroTree, args: []string{"deployment", "inspect", "dep-id"}, flag: "template", shorthand: "t", want: errInspectTemplateRemoved},
	{tree: astroTree, args: []string{"organization", "switch", "my-org"}, flag: "login-link", shorthand: "l", want: errLoginLinkRemoved},
	{tree: astroTree, args: []string{"api", "airflow", "GET", "/dags"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: astroTree, args: []string{"api", "airflow", "ls"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: apcTree, args: []string{"api", "airflow", "describe", "get_dags"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: astroTree, args: []string{"api", "airflow"}, flag: "deployment-id", value: "dep-id", want: errDeploymentIDAPIFlag},
	{tree: apcTree, args: []string{"api", "airflow", "ls"}, flag: "deployment-id", value: "dep-id", want: errDeploymentIDAPIFlag},
	{tree: astroTree, args: []string{"workspace", "list"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroHostedTree, args: []string{"deployment", "list", "-a"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroTree, args: []string{"api", "cloud", "ls"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroTree, args: []string{"deployment", "list"}, flag: "template", value: "{{.}}", want: errTemplateFlagRemoved},
	{tree: astroTree, args: []string{"organization", "team", "list"}, flag: "template", value: "{{.}}", want: errTemplateFlagRemoved},
	{
		tree: astroTree, args: []string{"env", "variable", "list"}, flag: "format", value: "yaml",
		want: "--format was removed in Astro CLI v2: use -o (--output), which takes text, json or dotenv; there is no yaml, so use -o json",
	},
	{
		tree: astroTree, args: []string{"env", "connection", "get", "my_conn"}, flag: "format", value: "json",
		want: "--format was removed in Astro CLI v2: use -o (--output), which takes text or json; there is no yaml, so use -o json",
	},
	{tree: astroTree, args: []string{"deployment", "delete", "dep-id"}, flag: "force", shorthand: "f", want: "--force was removed in Astro CLI v2: use --yes (-y)"},
	{tree: astroHostedTree, args: []string{"deployment", "bundle", "delete"}, flag: "force", shorthand: "f", want: "--force was removed in Astro CLI v2: use --yes (-y)"},
	{tree: apcTree, args: []string{"deployment", "update", "dep-id"}, flag: "force", shorthand: "f", want: "--force was removed in Astro CLI v2: use --yes (-y)"},
}

// spellings is every way the case's flag can be typed: --name, --name=value
// and --name value (or the boolean alone), the shorthand, and the shorthand
// first in a group, before a letter that would ask for help.
func (c *removedFlagCase) spellings() [][]string {
	value := c.value
	if value == "" {
		value = "true"
	}
	out := [][]string{{"--" + c.flag + "=" + value}}
	if c.value != "" {
		out = append(out, []string{"--" + c.flag, c.value})
	} else {
		out = append(out, []string{"--" + c.flag})
	}
	if c.shorthand != "" {
		out = append(out, []string{"-" + c.shorthand}, []string{"-" + c.shorthand + "h"})
	}
	return out
}

// Every entry, every spelling, in text and under --output json: a usage error
// with the entry's message, before any pre-run, and under json the one error
// object on stdout.
func TestRemovedFlagsSayWhatReplacedThem(t *testing.T) {
	reached := map[*removedFlag]bool{}
	for i := range removedFlagCases {
		c := &removedFlagCases[i]
		require.True(t, strings.HasPrefix(c.want, "--"+c.flag+" was removed in Astro CLI v2"), "message for --%s: %s", c.flag, c.want)
		for _, spelling := range c.spellings() {
			for _, asJSON := range []bool{false, true} {
				args := append(append([]string{}, c.args...), spelling...)
				if asJSON {
					args = append(args, "-o", "json")
				}
				name := c.tree.name + ": astro " + strings.Join(args, " ")
				t.Run(name, func(t *testing.T) {
					root, preRuns := disarmedTree(t, c.tree)
					target, _, err := root.Find(c.args)
					require.NoError(t, err)
					if f := findRemovedFlag(target, c.flag); f != nil {
						reached[f] = true
					}
					if asJSON && !cliout.HasOutput(target) {
						t.Skip("no --output here: an `astro api` request prints the API's response, and bundle delete prints nothing")
					}

					stdout, stderr, err := executeRoot(root, args...)
					require.Error(t, err)
					assert.Equal(t, c.want, err.Error())
					assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err))
					assert.Zero(t, *preRuns, "a pre-run started before the refusal")
					if !asJSON {
						assert.Empty(t, stdout)
						assert.Contains(t, stderr, "Error: "+c.want)
						return
					}
					assert.Empty(t, stderr)
					require.Equal(t, 1, strings.Count(stdout, "\n"), "stdout is not one line: %q", stdout)
					var obj map[string]any
					require.NoError(t, json.Unmarshal([]byte(stdout), &obj), stdout)
					assert.Equal(t, map[string]any{"error": c.want, "code": float64(cliout.ExitUsage), "kind": string(cliout.KindUsage)}, obj)
				})
			}
		}
	}
	for i := range removedFlags {
		assert.True(t, reached[&removedFlags[i]], "removedFlags[%d] (--%s) has no case in removedFlagCases", i, removedFlags[i].name)
	}
}

// --force's message changes when the run passed --yes already, ahead of it:
// pflag has set -y by the time it reaches the f of -yf. After it (-fy), the
// run has not got there yet, and is told to use what it is about to pass.
func TestRemovedForceSaysWhenYesIsAlreadyThere(t *testing.T) {
	const use = "--force was removed in Astro CLI v2: use --yes (-y)"
	const passed = "--force was removed in Astro CLI v2: --yes (-y), which you passed, already skips the confirmation, so drop "
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{[]string{"-yf"}, passed + "-f"},
		{[]string{"-y", "-f"}, passed + "-f"},
		{[]string{"--yes", "--force"}, passed + "--force"},
		{[]string{"--yes=true", "--force"}, passed + "--force"},
		{[]string{"-fy"}, use},
		{[]string{"--force", "--yes"}, use},
		{[]string{"--yes=false", "--force"}, use},
		{[]string{"--yes=false", "-f"}, use},
		{[]string{"-fh"}, use},
	} {
		args := append([]string{"deployment", "delete", "dep-id"}, tc.flags...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, preRuns := disarmedTree(t, astroTree)
			_, _, err := executeRoot(root, args...)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.True(t, cliout.IsUsage(err))
			assert.Zero(t, *preRuns)
		})
	}
}

// Each entry's under names a command a tree has, so an entry cannot quietly
// stop applying when a command is renamed.
func TestRemovedFlagsNameRealCommands(t *testing.T) {
	trees := rootsUnderTest(t)
	for _, f := range removedFlags {
		for _, path := range f.under {
			found := false
			for _, tree := range trees {
				cmd, _, err := tree.root.Find(strings.Fields(path))
				if err == nil && isUnder(cmd, path) && strings.HasSuffix(cmd.CommandPath(), path) {
					found = true
				}
			}
			assert.True(t, found, "--%s: no tree has `astro %s`", f.name, path)
		}
	}
}

// A command that still has the flag runs with it, and one that did not have
// it in 1.x is told what cobra always told it: "was removed" would be false
// there, or name the wrong flag (-f meant other things on other commands).
func TestRemovedFlagsLeaveOtherCommandsAlone(t *testing.T) {
	for _, tc := range []struct {
		tree treeConfig
		args []string
		want string // the error; errRan when flag parsing succeeded
	}{
		// These still have the flag.
		{astroTree, []string{"deploy", "--force"}, errRan.Error()},
		{astroTree, []string{"deploy", "-f"}, errRan.Error()},
		{astroTree, []string{"local", "stop", "--force"}, errRan.Error()},
		{astroTree, []string{"login", "--login-link"}, errRan.Error()},
		{astroTree, []string{"api", "airflow", "--template", "{{.}}"}, errRan.Error()},
		{apcTree, []string{"deployment", "team", "list", "--deployment-id", "dep-id"}, errRan.Error()},
		// 1.x had no `local` commands (`astro dev` was), and so none of
		// these flags on them.
		{astroTree, []string{"local", "stop", "-f"}, "unknown shorthand flag: 'f' in -f"},
		{astroTree, []string{"local", "reset", "-f"}, "unknown shorthand flag: 'f' in -f"},
		{astroTree, []string{"local", "reset", "--force"}, "unknown flag: --force"},
		// Commands 1.x had, without these flags.
		{astroTree, []string{"version", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"deploy", "--json"}, "unknown flag: --json"},
		{astroTree, []string{"deployment", "logs", "dep-id", "--deployment-id", "x"}, "unknown flag: --deployment-id"},
		{apcTree, []string{"deployment", "create", "--deployment-file", "f.yaml"}, "unknown flag: --deployment-file"},
		{apcTree, []string{"deployment", "delete", "dep-id", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"deployment", "list", "-t"}, "unknown shorthand flag: 't' in -t"},
	} {
		t.Run(tc.tree.name+": astro "+strings.Join(tc.args, " "), func(t *testing.T) {
			root, _ := disarmedTree(t, tc.tree)
			_, _, err := executeRoot(root, tc.args...)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
		})
	}
}

// A removed flag is still an unknown flag to telemetry: the events are what
// tell us when nobody passes one any more, and its entry can go.
func TestRemovedFlagsStillReachTelemetry(t *testing.T) {
	type event struct{ command, flag string }
	var got []event
	orig := recordUnknownFlag
	recordUnknownFlag = func(cmd *cobra.Command, flag string) { got = append(got, event{cmd.CommandPath(), flag}) }
	t.Cleanup(func() { recordUnknownFlag = orig })

	for _, args := range [][]string{
		{"deployment", "delete", "dep-id", "--force"},
		{"deployment", "delete", "dep-id", "-f"},
		{"workspace", "list", "--json", "-o", "json"},
	} {
		root, _ := disarmedTree(t, astroTree)
		_, _, err := executeRoot(root, args...)
		require.ErrorContains(t, err, "was removed in Astro CLI v2", args)
	}
	assert.Equal(t, []event{
		{"astro deployment delete", "--force"},
		{"astro deployment delete", "-f"},
		{"astro workspace list", "--json"},
	}, got)
}

// Asking for help gets the help, wherever the removed flag stands. Only a
// shorthand group that reaches the removed letter before the h is refused,
// as pflag stops there.
func TestRemovedFlagsGiveWayToHelp(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "list", "--json", "--help"},
		{"deployment", "list", "--help", "--json"},
		{"deployment", "list", "--json", "-h"},
		{"deployment", "list", "--json", "-h", "-o", "json"},
		{"deployment", "delete", "dep-id", "-f", "-h"},
		{"deployment", "delete", "dep-id", "-hf"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, preRuns := disarmedTree(t, astroTree)
			var out strings.Builder
			root.SetOut(&out)
			stdout, _, err := executeRootKeepingOut(root, args...)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			target, _, err := root.Find(args)
			require.NoError(t, err)
			assert.Contains(t, out.String(), "Usage:\n  "+target.UseLine(), "no help for %s", target.CommandPath())
			assert.Zero(t, *preRuns)
		})
	}
	// A -h that is a value is still read as asking for help, and the worst
	// that does is show help: the flag stays refused for a run that is going
	// to run.
	root, _ := disarmedTree(t, astroTree)
	_, _, err := executeRoot(root, "deployment", "list", "--json", "--", "-h")
	require.EqualError(t, err, errJSONFlagRemoved)
}

// executeRootKeepingOut runs root like executeRoot, but leaves the out writer
// the test set on it, where cobra prints help.
func executeRootKeepingOut(root *cobra.Command, args ...string) (stdout, stderr string, err error) {
	var out, errOut strings.Builder
	root.SetErr(&errOut)
	err = execute(context.Background(), root, args, &out)
	return out.String(), errOut.String(), err
}

// A shell asking for completions after a removed flag gets them: cobra
// parses the flags of the command it completes without the flag error func,
// and would fail the completion on any flag it does not know.
func TestRemovedFlagsLeaveCompletionsAlone(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string // a completion offered, or "" for none at all
	}{
		{[]string{"__complete", "deployment", "list", "--json", "-"}, "--all"},
		{[]string{"__complete", "deployment", "list", "--json", "--o"}, "--output"},
		{[]string{"__completeNoDesc", "deployment", "list", "--template", "{{.}}", "--o"}, "--output"},
		{[]string{"__complete", "deployment", "delete", "-f", "--y"}, "--yes"},
		// A removed flag before the subcommand: cobra would take an
		// unknown flag to have a value, here ls, and complete api airflow.
		{[]string{"__complete", "api", "airflow", "--json", "ls", "--fi"}, "--filter"},
		{[]string{"__complete", "api", "airflow", "--json", "ls", "--refresh", "--fi"}, "--filter"},
		{[]string{"__complete", "api", "airflow", "--api-url", "http://localhost:8080", "ls", "--fi"}, "--filter"},
		{[]string{"__complete", "api", "airflow", "--api-url=http://localhost:8080", "ls", "--fi"}, "--filter"},
		{[]string{"__complete", "api", "airflow", "--deployment-id", "dep-id", "describe", "--re"}, "--refresh"},
		{[]string{"__complete", "deployment", "-f", "delete", "--y"}, "--yes"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root, _ := disarmedTree(t, astroTree)
			// cobra's __complete runs under the root's pre-run, which the real
			// one lets through and the disarmed one would not.
			root.PersistentPreRunE = nil
			var out strings.Builder
			root.SetOut(&out)
			_, stderr, err := executeRootKeepingOut(root, tc.args...)
			require.NoError(t, err)
			assert.NotContains(t, stderr, "Error")
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			require.NotEmpty(t, lines)
			directive := lines[len(lines)-1]
			assert.NotEqual(t, ":1", directive, "cobra reported an error: %s", stderr)
			if tc.want != "" {
				offered := make([]string, 0, len(lines)-1)
				for _, l := range lines[:len(lines)-1] {
					offered = append(offered, strings.SplitN(l, "\t", 2)[0])
				}
				assert.Contains(t, offered, tc.want)
			}
		})
	}
	// Every command 1.x had takes its removed flags for a completion, without
	// a name or shorthand clashing with one it has (pflag panics on that),
	// and then parses each of them.
	for _, tc := range v1Trees {
		root := buildTree(t, tc.tree).root
		for _, f := range v1Flags() {
			if !slices.Contains(f.trees, tc.v1) {
				continue
			}
			cmd, _, err := root.Find(strings.Fields(f.path))
			if err != nil || pathBelowRoot(cmd) != f.path || cmd.DisableFlagParsing {
				continue
			}
			removed := cmd.Flag(f.name) == nil
			arg := "--" + f.name
			if !f.isBool {
				arg += "=x"
			}
			require.NotPanics(t, func() { acceptForCompletion(root, append(strings.Fields(f.path), arg)) }, "%s: %s", tc.tree.name, f.path)
			if !removed {
				continue
			}
			assert.NoError(t, cmd.ParseFlags([]string{arg}), "%s: astro %s %s", tc.tree.name, f.path, arg)
		}
	}
}

// A completion request that passes no flag its command lacks never reads the
// inventory, and one that does reads it.
func TestCompletionsReadTheInventoryOnlyForAnUnknownFlag(t *testing.T) {
	read := false
	orig := v1Flags
	v1Flags = func() []v1Flag { read = true; return orig() }
	t.Cleanup(func() { v1Flags = orig })

	for _, tc := range []struct {
		args []string
		read bool
	}{
		{[]string{"deployment", "list", "--all", "-o", "json"}, false},
		{[]string{"api", "airflow", "--deployment", "dep-id", "ls", "--refresh"}, false},
		{[]string{"deployment", "delete", "dep-id", "-y"}, false},
		{[]string{"deployment", "list", "--json"}, true},
		{[]string{"api", "airflow", "--json", "ls"}, true},
		{[]string{"deployment", "delete", "dep-id", "-yf"}, true},
	} {
		read = false
		root := buildTree(t, astroTree).root
		acceptForCompletion(root, tc.args)
		assert.Equal(t, tc.read, read, strings.Join(tc.args, " "))
	}
}

// Every root says which 1.x tree a script on its machine was written for,
// and whether that is Astro's for a hosted organization is read from the
// context when it is asked, not when the root is built.
func TestEveryTreeNamesItsV1Tree(t *testing.T) {
	for _, tc := range v1Trees {
		root := buildTree(t, tc.tree).root
		assert.Equal(t, tc.v1, v1TreeOf(root), tc.tree.name)
	}
	root := buildTree(t, astroTree).root
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
	ctx, err := astrocontext.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, ctx.SetContextKey("organization_product", "HOSTED"))
	assert.Equal(t, v1TreeAstroHosted, v1TreeOf(root), "a root built before the context said hosted")
}

// A 1.x path a v2 command answers to by an alias finds the flags 1.x had
// there, as does the v2 name.
func TestV1FlagsMatchAliasedPaths(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	group := &cobra.Command{Use: "deployment", Aliases: []string{"deployments"}}
	renamed := &cobra.Command{Use: "variable", Aliases: []string{"airflow-variable"}}
	root.AddCommand(group)
	group.AddCommand(renamed)
	assert.Equal(t, []string{
		"deployment variable", "deployment airflow-variable",
		"deployments variable", "deployments airflow-variable",
	}, pathSpellings(renamed))

	flags := []v1Flag{
		{trees: []string{v1TreeAstro}, path: "deployment airflow-variable", name: "json", isBool: true},
		{trees: []string{v1TreeAPC}, path: "deployment airflow-variable", name: "force", isBool: true},
		{trees: []string{v1TreeAstro}, path: "deployment pool", name: "template"},
	}
	on := v1FlagsAt(flags, v1TreeAstro, pathSpellings(renamed), false)
	require.Len(t, on, 1)
	assert.Equal(t, "json", on[0].name)
	assert.Len(t, v1FlagsAt(flags, v1TreeAstro, pathSpellings(group), true), 2)
	assert.Empty(t, v1FlagsAt(flags, v1TreeAstro, pathSpellings(group), false))
}

// v1_flags.tsv reads the same with CRLF line ends, as a Windows checkout
// without .gitattributes' eol=lf would embed it, and a file that does not
// parse is an error here rather than a panic in a run.
func TestV1FlagsFileParsesWithCRLF(t *testing.T) {
	lf, err := parseV1Flags(v1FlagsFile)
	require.NoError(t, err)
	crlf, err := parseV1Flags(strings.ReplaceAll(v1FlagsFile, "\n", "\r\n"))
	require.NoError(t, err)
	assert.Equal(t, lf, crlf)

	const malformed = "astro\tdeployment list\tjson\t\tmaybe\n"
	_, err = parseV1Flags(malformed)
	require.ErrorContains(t, err, "malformed line")
	assert.Nil(t, loadV1Flags(malformed))
	assert.NotEmpty(t, loadV1Flags(strings.ReplaceAll(v1FlagsFile, "\n", "\r\n")))

	// With no inventory, a removed flag is an unknown flag, as cobra says,
	// and a completion request is left as cobra would take it.
	orig := v1Flags
	v1Flags = func() []v1Flag { return loadV1Flags(malformed) }
	t.Cleanup(func() { v1Flags = orig })
	root, _ := disarmedTree(t, astroTree)
	_, _, err = executeRoot(root, "deployment", "delete", "dep-id", "--force")
	require.EqualError(t, err, "unknown flag: --force")
	assert.True(t, cliout.IsUsage(err))
	root = buildTree(t, astroTree).root
	assert.NotPanics(t, func() { acceptForCompletion(root, []string{"api", "airflow", "--json", "ls", "--force"}) })
}

// v1_flags.tsv parses, names only the trees there are, and is in order, so a
// regenerated one diffs line by line. It is checked in with LF line ends,
// which .gitattributes keeps on every checkout.
func TestV1FlagsFileIsWellFormed(t *testing.T) {
	assert.NotContains(t, v1FlagsFile, "\r", "v1_flags.tsv has CRLF line ends: is it still text eol=lf in .gitattributes?")
	flags, err := parseV1Flags(v1FlagsFile)
	require.NoError(t, err)
	require.Greater(t, len(flags), 900)
	known := []string{v1TreeAstro, v1TreeAstroHosted, v1TreeAPC}
	seen := map[string]bool{}
	var lines []string
	for _, f := range flags {
		for _, tree := range f.trees {
			assert.Contains(t, known, tree, "%s --%s", f.path, f.name)
			seen[tree] = true
		}
		lines = append(lines, f.path+"\t"+f.name+"\t"+f.shorthand)
	}
	assert.Len(t, seen, len(known), "a tree has no flags")
	assert.True(t, slices.IsSorted(lines), "v1_flags.tsv is not sorted by command path and flag")
}

// Every flag 1.x had, on a command v2 still has, either still parses there or
// fails with what replaced it, word for word, so a script written for 1.x is
// never told only "unknown flag". A flag v2 drops without an entry in
// removedFlags fails here, as does an entry no 1.x flag reaches. A command v2
// dropped altogether is its own tombstone (`astro dev`, `astro run`, `env ...
// create`), and is skipped.
func TestEveryV1FlagStillWorksOrSaysWhatReplacedIt(t *testing.T) {
	reached := map[*removedFlag]bool{}
	for _, tc := range v1Trees {
		root, preRuns := disarmedTree(t, tc.tree)
		checked := 0
		for _, f := range v1Flags() {
			if !slices.Contains(f.trees, tc.v1) {
				continue
			}
			label := tc.tree.name + ": astro " + f.path + " --" + f.name
			cmd, rest, err := root.Find(strings.Fields(f.path))
			if !assert.NoError(t, err, label) {
				continue
			}
			if len(rest) > 0 {
				// The words left over name no command under the deepest one
				// that matched: a dropped command, which is its own
				// tombstone (`astro dev`), or a rename that kept no alias.
				assert.True(t, cmd.Hidden, "%s: 1.x's path resolves to `%s` with %v left over; a renamed command should keep its 1.x name as an alias", label, cmd.CommandPath(), rest)
				continue
			}
			// A command renamed with its 1.x name kept as an alias is reached
			// at the 1.x path, and checked there.
			if !assert.Contains(t, pathSpellings(cmd), f.path, label) || cmd.DisableFlagParsing {
				continue
			}
			checked++

			if cmd.Flag(f.name) == nil {
				spelling := "--" + f.name
				arg := spelling
				if !f.isBool {
					arg += "=x"
				}
				reached[assertSaysWhatReplacedIt(t, root, preRuns, cmd, &f, arg, spelling, label)] = true
			}
			// The shorthand may have moved with a rename (-d is --deployment
			// now, as -d was --deployment-id); if nothing has it, it says
			// what replaced the flag too.
			if f.shorthand != "" && cmd.LocalFlags().ShorthandLookup(f.shorthand) == nil && cmd.InheritedFlags().ShorthandLookup(f.shorthand) == nil {
				reached[assertSaysWhatReplacedIt(t, root, preRuns, cmd, &f, "-"+f.shorthand, "-"+f.shorthand, label+" (-"+f.shorthand+")")] = true
			}
		}
		t.Logf("%s: %d flags checked", tc.tree.name, checked)
		assert.Greater(t, checked, 200, "%s: the inventory matched few commands; is v1_flags.tsv still read right?", tc.tree.name)
	}
	for i := range removedFlags {
		assert.True(t, reached[&removedFlags[i]], "removedFlags[%d] (--%s) applies to no flag 1.x had on a command v2 has", i, removedFlags[i].name)
	}
}

// assertSaysWhatReplacedIt runs cmd with arg and checks it is refused with
// the message of the entry for f there, and returns that entry.
func assertSaysWhatReplacedIt(t *testing.T, root *cobra.Command, preRuns *int, cmd *cobra.Command, f *v1Flag, arg, spelling, label string) *removedFlag {
	t.Helper()
	entry := findRemovedFlag(cmd, f.name)
	if !assert.NotNil(t, entry, "%s: no entry in removedFlags applies", label) {
		return nil
	}
	root.SetArgs(append(strings.Fields(f.path), arg))
	before := *preRuns
	_, err := root.ExecuteC()
	if !assert.Error(t, err, label) {
		return entry
	}
	assert.Equal(t, entry.message(cmd, spelling), err.Error(), label)
	assert.True(t, strings.HasPrefix(err.Error(), "--"+f.name+" was removed in Astro CLI v2"), "%s: %v", label, err)
	assert.True(t, cliout.IsUsage(err), "%s: not a usage error", label)
	assert.Equal(t, before, *preRuns, "%s: a pre-run started", label)
	return entry
}
