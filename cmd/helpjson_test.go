package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

// runHelpLine runs args through execute on a fresh astro tree, hooks disarmed,
// and returns what reached stdout and stderr.
func runHelpLine(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	// The help command keeps its real hooks: it skips the platform pre-run
	// itself, which is what is under test. Every other command is disarmed.
	root, _, preRuns := disarmedTreeKeeping(t, astroTree, func(c *cobra.Command) bool { return c.Name() == helpName })
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	err = execute(t.Context(), root, args, &out)
	assert.Zero(t, *preRuns, "help must run no pre-run")
	return out.String(), errOut.String(), err
}

func decodeHelpDocument(t *testing.T, stdout string) helpDocument {
	t.Helper()
	var doc helpDocument
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&doc), "stdout: %q", stdout)
	assert.False(t, dec.More(), "more than one document on stdout")
	return doc
}

func flagNamed(doc *helpDocument, name string) (helpFlag, bool) {
	for _, f := range doc.Flags {
		if f.Name == name {
			return f, true
		}
	}
	return helpFlag{}, false
}

func TestHelpJSONDescribesALeaf(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "list", "--help", "-o", "json"},
		{"deployment", "list", "-o", "json", "--help"},
		{"deployment", "list", "--help", "--output=json"},
		{"help", "deployment", "list", "-o", "json"},
		{"deployment", "ls", "--help", "-o", "json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := runHelpLine(t, args...)
			require.NoError(t, err)
			assert.Empty(t, stderr)
			doc := decodeHelpDocument(t, stdout)
			assert.Equal(t, "astro deployment list", doc.Path)
			assert.NotEmpty(t, doc.Summary)
			assert.NotEmpty(t, doc.Description)
			assert.Equal(t, "astro deployment list [flags]", doc.Usage)
			assert.Contains(t, doc.Aliases, "ls")
			assert.Empty(t, doc.Subcommands)
			assert.NotEmpty(t, doc.Examples)

			all, ok := flagNamed(&doc, "all")
			require.True(t, ok)
			assert.Equal(t, helpFlag{
				Name: "all", Shorthand: "a", Type: "bool", Default: "false",
				Description: all.Description, Group: "",
			}, all)
			// Inherited flags are listed, as global.
			noColor, ok := flagNamed(&doc, "no-color")
			require.True(t, ok, "inherited flags must appear")
			assert.Equal(t, globalFlagGroup, noColor.Group)
		})
	}
}

// Like text help, JSON help gives way to nothing: a removed 1.x flag on the
// line does not stop it.
func TestHelpJSONGivesWayToARemovedFlag(t *testing.T) {
	stdout, stderr, err := runHelpLine(t, "deployment", "list", "--json", "-h", "-o", "json")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.Equal(t, "astro deployment list", decodeHelpDocument(t, stdout).Path)
}

func TestHelpJSONDescribesAGroup(t *testing.T) {
	stdout, stderr, err := runHelpLine(t, "deployment", "--help", "-o", "json")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	doc := decodeHelpDocument(t, stdout)
	assert.Equal(t, "astro deployment", doc.Path)
	names := []string{}
	for _, s := range doc.Subcommands {
		names = append(names, s.Name)
		assert.NotEmpty(t, s.Summary)
	}
	assert.Contains(t, names, "list")
}

func TestHelpJSONSubcommandsFollowHelpOrder(t *testing.T) {
	root, _ := disarmedTree(t, astroTree)
	root.InitDefaultHelpCmd()
	doc := describeCommand(root)
	var want []string
	for _, c := range listedInOrder(root) {
		want = append(want, c.Name())
	}
	var got []string
	for _, s := range doc.Subcommands {
		got = append(got, s.Name)
	}
	assert.Equal(t, want, got)
	assert.NotEmpty(t, got)
}

func TestHelpJSONArgumentsComeFromTheUseLine(t *testing.T) {
	for use, want := range map[string][]helpArg{
		"get <DAG_ID> <RUN_ID>":      {{"DAG_ID", true, false}, {"RUN_ID", true, false}},
		"add [NAME] [-- COMMAND...]": {{"NAME", false, false}, {"COMMAND", false, true}},
		"rm <NAME...> [flags]":       {{"NAME", true, true}},
		"list":                       {},
	} {
		assert.Equal(t, want, helpArguments(use), use)
	}
}

func TestHelpJSONLeavesOutHiddenFlagsAndCommands(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	run := func(*cobra.Command, []string) {}
	visible := &cobra.Command{Use: "visible", Short: "Visible one", Run: run}
	visible.Flags().String("shown", "", "A shown flag")
	visible.Flags().String("secret", "", "A hidden flag")
	require.NoError(t, visible.Flags().MarkHidden("secret"))
	hidden := &cobra.Command{Use: "hidden", Short: "Hidden one", Hidden: true, Run: run}
	hiddenGroup := &cobra.Command{Use: "gone", Short: "Gone", Hidden: true}
	hiddenGroup.AddCommand(&cobra.Command{Use: "child", Short: "Child", Run: run})
	root.AddCommand(visible, hidden, hiddenGroup)
	installHelp(root, cloudPlatform, "")

	doc := describeCommand(visible)
	_, shown := flagNamed(&doc, "shown")
	_, secret := flagNamed(&doc, "secret")
	assert.True(t, shown)
	assert.False(t, secret)

	rootDoc := describeCommand(root)
	require.Len(t, rootDoc.Subcommands, 1)
	assert.Equal(t, "visible", rootDoc.Subcommands[0].Name)

	// A hidden command still describes itself, but is not listed or searched.
	assert.Equal(t, "astro hidden", describeCommand(hidden).Path)
	assert.Len(t, describeAll(root).Commands, 2)
	assert.Empty(t, searchCommands(root, "child", 10))
	assert.Empty(t, searchCommands(root, "hidden one", 10))
}

func TestHelpJSONForAHiddenCommandStillWorks(t *testing.T) {
	stdout, _, err := runHelpLine(t, "dev", "--help", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, "astro dev", decodeHelpDocument(t, stdout).Path)
}

func TestHelpWithoutJSONIsTheTextPage(t *testing.T) {
	stdout, _, err := runHelpLine(t, "deployment", "list", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Usage:\n  astro deployment list")
	assert.False(t, strings.HasPrefix(stdout, "{"))
	stdout, _, err = runHelpLine(t, "help", "deployment", "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Usage:\n  astro deployment list")
}

func TestHelpJSONOfAnUnknownCommandFails(t *testing.T) {
	stdout, _, err := runHelpLine(t, "help", "nosuchcommand", "-o", "json")
	require.Error(t, err)
	assert.True(t, cliout.IsUsage(err))
	// Under -o json the failure is the one error object.
	assert.Equal(t, "usage", oneErrorDocument(t, stdout).Kind)
}

func TestHelpAllCoversEveryVisibleCommand(t *testing.T) {
	stdout, stderr, err := runHelpLine(t, "help", "--all", "-o", "json")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	var doc helpAllDocument
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))

	root, _ := disarmedTree(t, astroTree)
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var want []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		want = append(want, c.CommandPath())
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() || sub.Name() == helpName || sub.IsAdditionalHelpTopicCommand() {
				walk(sub)
			}
		}
	}
	walk(root)
	var got []string
	for _, d := range doc.Commands {
		got = append(got, d.Path)
	}
	assert.ElementsMatch(t, want, got)
	assert.Equal(t, "astro", got[0], "depth first from the root")
	for _, d := range doc.Commands {
		assert.False(t, strings.HasPrefix(d.Path, "astro dev "), "hidden commands are not listed")
	}
	assert.Greater(t, len(got), 50)
}

func TestHelpAllWithoutJSONListsPathsAndSummaries(t *testing.T) {
	stdout, _, err := runHelpLine(t, "help", "--all")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Contains(t, lines, "astro deployment list\tList all Deployments running in your Astronomer Workspace")
	for _, line := range lines {
		assert.Len(t, strings.Split(line, "\t"), 2, line)
	}
	_, _, err = runHelpLine(t, "help", "--all", "deployment")
	require.Error(t, err)
	assert.True(t, cliout.IsUsage(err))
}

func TestHelpSearchRanksPathBeforeDescription(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	run := func(*cobra.Command, []string) {}
	byDescription := &cobra.Command{Use: "zeta", Short: "Does things", Long: "Also can create a Deployment.", Run: run}
	bySummary := &cobra.Command{Use: "beta", Short: "Create a Deployment", Run: run}
	byPath := &cobra.Command{Use: "deployment", Short: "Group"}
	byPath.AddCommand(&cobra.Command{Use: "create", Short: "Make one", Run: run})
	byExample := &cobra.Command{Use: "alpha", Short: "Other", Example: "  astro alpha --name create-deployment", Run: run}
	root.AddCommand(byDescription, bySummary, byPath, byExample)

	var got []string
	for _, r := range searchCommands(root, "CREATE deployment", 10) {
		got = append(got, r.Path)
	}
	assert.Equal(t, []string{"astro deployment create", "astro beta", "astro zeta", "astro alpha"}, got)
	assert.Empty(t, searchCommands(root, "create nonexistentword", 10))
	assert.Len(t, searchCommands(root, "create deployment", 2), 2)
}

func TestHelpSearchOutput(t *testing.T) {
	stdout, stderr, err := runHelpLine(t, "help", "search", "list", "deployments")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.LessOrEqual(t, len(lines), searchLimit)
	assert.True(t, strings.HasPrefix(lines[0], "astro "), lines[0])
	assert.Contains(t, lines[0], "\t")

	stdout, stderr, err = runHelpLine(t, "help", "search", "list", "deployments", "-o", "json")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	var doc helpSearchDocument
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.NotEmpty(t, doc.Commands)
	assert.LessOrEqual(t, len(doc.Commands), searchLimit)
}

func TestHelpSearchWithNoMatchExitsZero(t *testing.T) {
	stdout, stderr, err := runHelpLine(t, "help", "search", "zzqqxx", "nothing")
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "No commands match \"zzqqxx nothing\".\n", stderr)

	stdout, stderr, err = runHelpLine(t, "help", "search", "zzqqxx", "-o", "json")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.JSONEq(t, `{"commands":[]}`, stdout)
}

func TestHelpSearchNeedsAQuery(t *testing.T) {
	_, _, err := runHelpLine(t, "help", "search")
	require.Error(t, err)
	assert.True(t, cliout.IsUsage(err))
}

// `astro help search` is the search only because no command is named search.
// A top-level command of that name would be shadowed, so this fails the day
// one is added: rename the word the help command takes, or the command.
func TestNoCommandIsNamedLikeTheHelpSearchWord(t *testing.T) {
	for _, tree := range []treeConfig{astroTree, apcTree} {
		root, _ := disarmedTree(t, tree)
		root.InitDefaultHelpCmd()
		assert.Nil(t, childNamed(root, helpSearchWord), "%s: a command named search would shadow astro help search", tree.name)
	}
}

// The words typed after `help search` are not sent to telemetry: help skips
// the tracking pre-run, and a search that works is not a usage error, so the
// unknown-command event never reads it either.
func TestHelpSearchQueryIsNotTracked(t *testing.T) {
	root, _ := disarmedTree(t, astroTree)
	root.InitDefaultHelpCmd()
	helpCmd := childNamed(root, helpName)
	require.NotNil(t, helpCmd)
	assert.Equal(t, "true", helpCmd.Annotations[telemetry.SkipPreRunAnnotation])
	assert.Nil(t, findUnknownCommand(root, []string{"help", "search", "my-secret-token", "deploy"}))
	assert.Nil(t, findUnknownCommand(root, []string{"help", "search"}))
}
