package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// The trees the removed-flag tests run: Astro for a non-hosted organization,
// and APC at the newest platform version, where every version-gated command
// is mounted. Each test builds its own, so it runs against that tree's state.
var (
	astroTree = treeConfig{name: "astro", platform: cloudPlatform}
	apcTree   = treeConfig{name: "apc " + newestAPCVersion, platform: apcPlatform, apcVersion: newestAPCVersion}
)

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

// removedFlagCase is a run passing a removed flag on a command that does not
// have it, and what it must be told.
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
// (TestRemovedFlagsSayWhatReplacedThem checks that), on a command the flag
// reaches, with the message it gives there.
var removedFlagCases = []removedFlagCase{
	{tree: astroTree, args: []string{"deployment", "create", "--name", "x"}, flag: "deployment-file", value: "deployment.yaml", want: errDeploymentFileRemoved},
	{tree: astroTree, args: []string{"deployment", "update", "dep-id"}, flag: "deployment-file", value: "deployment.yaml", want: errDeploymentFileRemoved},
	{tree: astroTree, args: []string{"deployment", "inspect", "dep-id"}, flag: "template", shorthand: "t", want: errInspectTemplateRemoved},
	{tree: astroTree, args: []string{"organization", "switch", "my-org"}, flag: "login-link", shorthand: "l", want: errLoginLinkRemoved},
	{tree: astroTree, args: []string{"api", "airflow", "GET", "/dags"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: astroTree, args: []string{"api", "airflow", "ls"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: apcTree, args: []string{"api", "airflow", "describe", "get_dags"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: astroTree, args: []string{"api", "airflow"}, flag: "deployment-id", value: "dep-id", want: errDeploymentIDAPIFlag},
	{tree: apcTree, args: []string{"api", "airflow", "ls"}, flag: "deployment-id", value: "dep-id", want: errDeploymentIDAPIFlag},
	{tree: astroTree, args: []string{"workspace", "list"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroTree, args: []string{"deployment", "list", "-a"}, flag: "json", want: errJSONFlagRemoved},
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
	{tree: astroTree, args: []string{"deployment", "bundle", "delete"}, flag: "force", shorthand: "f", want: "--force was removed in Astro CLI v2: use --yes (-y)"},
	{tree: apcTree, args: []string{"deployment", "delete", "dep-id"}, flag: "force", shorthand: "f", want: "--force was removed in Astro CLI v2: use --yes (-y)"},
}

// spellings is every way the case's flag can be typed: --name, --name=value
// and --name value (or the boolean alone), the shorthand, and the shorthand
// first in a group.
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
					if f := findRemovedFlag(target, c.flag, false); f != nil {
						reached[f] = true
					}
					if asJSON && cliout.Formats(target) == nil {
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

// A command that still has the flag runs with it, and one where the
// replacement does not exist is told what cobra always told it: the message
// would be false there.
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
		// `local stop` has --force, without a -f: "--force was removed" would be false.
		{astroTree, []string{"local", "stop", "-f"}, "unknown shorthand flag: 'f' in -f"},
		// No --yes, no -o, outside api airflow, no `deployment create --clone`.
		{astroTree, []string{"version", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"deploy", "--json"}, "unknown flag: --json"},
		{astroTree, []string{"deployment", "logs", "dep-id", "--deployment-id", "x"}, "unknown flag: --deployment-id"},
		{apcTree, []string{"deployment", "create", "--deployment-file", "f.yaml"}, "unknown flag: --deployment-file"},
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

// v1Flag is a line of testdata/v1_flags.tsv.
type v1Flag struct {
	tree, path, name, shorthand string
	isBool                      bool
}

func readV1Flags(t *testing.T) []v1Flag {
	t.Helper()
	file, err := os.Open("testdata/v1_flags.tsv")
	require.NoError(t, err)
	defer file.Close()
	var flags []v1Flag
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := lines.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		require.Len(t, f, 5, line)
		flags = append(flags, v1Flag{tree: f[0], path: f[1], name: f[2], shorthand: f[3], isBool: f[4] == "bool"})
	}
	require.NoError(t, lines.Err())
	require.NotEmpty(t, flags)
	return flags
}

// Every flag 1.x had, on a command v2 still has, either still parses there or
// fails with what replaced it, so a script written for 1.x is never told only
// "unknown flag". A flag v2 drops without an entry in removedFlags fails here.
// A command v2 dropped altogether is its own tombstone (`astro dev`, `astro
// run`, `env ... create`), and is skipped.
func TestEveryV1FlagStillWorksOrSaysWhatReplacedIt(t *testing.T) {
	flags := readV1Flags(t)
	checked := 0
	for _, tc := range []struct {
		v1   string
		tree treeConfig
	}{{"cloud", astroTree}, {"software", apcTree}} {
		root, preRuns := disarmedTree(t, tc.tree)
		for _, f := range flags {
			if f.tree != tc.v1 {
				continue
			}
			cmd, _, err := root.Find(strings.Fields(f.path))
			if err != nil || cmd.CommandPath() != root.Name()+" "+f.path || cmd.DisableFlagParsing {
				continue
			}
			label := tc.tree.name + ": astro " + f.path + " --" + f.name
			checked++

			if cmd.Flag(f.name) == nil {
				spelling := "--" + f.name
				if !f.isBool {
					spelling += "=x"
				}
				assertSaysWhatReplacedIt(t, root, preRuns, f, spelling, label)
			}
			// The shorthand may have moved with a rename (-d is --deployment
			// now, as -d was --deployment-id); if nothing has it, it says
			// what replaced the flag too.
			if f.shorthand != "" && cmd.LocalFlags().ShorthandLookup(f.shorthand) == nil && cmd.InheritedFlags().ShorthandLookup(f.shorthand) == nil {
				assertSaysWhatReplacedIt(t, root, preRuns, f, "-"+f.shorthand, label+" (-"+f.shorthand+")")
			}
		}
	}
	assert.Greater(t, checked, 500, "the inventory matched few commands; is testdata/v1_flags.tsv still read right?")
}

func assertSaysWhatReplacedIt(t *testing.T, root *cobra.Command, preRuns *int, f v1Flag, spelling, label string) {
	t.Helper()
	root.SetArgs(append(strings.Fields(f.path), spelling))
	before := *preRuns
	_, err := root.ExecuteC()
	if !assert.Error(t, err, label) {
		return
	}
	assert.True(t, strings.HasPrefix(err.Error(), "--"+f.name+" was removed in Astro CLI v2"), "%s: %v", label, err)
	assert.True(t, cliout.IsUsage(err), "%s: not a usage error", label)
	assert.Equal(t, before, *preRuns, "%s: a pre-run started", label)
}
