package cliout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/input"
)

var errBoom = errors.New("boom")

const kindBoom ProblemKind = "boom"

var testKinds = Kinds{{Kind: kindBoom, Match: Sentinel(errBoom)}}

// run is what a user gets from a tree: the error, stdout, stderr.
type run struct {
	err            error
	stdout, stderr string
}

// testTree is a root with one group and a leaf that fails with fail, and a
// second leaf with no --output flag at all.
func testTree(fail error) *cobra.Command {
	var output Format
	root := &cobra.Command{Use: "astro"}
	group := &cobra.Command{Use: "thing"}
	leaf := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return fail },
	}
	AddOutputFlag(leaf, &output)
	leaf.Flags().Bool("all", false, "")
	plain := &cobra.Command{
		Use:  "plain",
		RunE: func(*cobra.Command, []string) error { return fail },
	}
	var name string
	required := &cobra.Command{
		Use:  "create",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	AddOutputFlag(required, &output)
	required.Flags().StringVar(&name, "name", "", "")
	_ = required.MarkFlagRequired("name")
	group.AddCommand(leaf, plain, required)
	root.AddCommand(group)
	return root
}

func execTree(ctx context.Context, root *cobra.Command, args ...string) run {
	var stdout, stderr bytes.Buffer
	root.SetOut(&stderr) // cobra's Println goes to OutOrStderr; keep it off stdout
	root.SetErr(&stderr)
	err := Execute(ctx, root, args, &stdout, testKinds)
	return run{err: err, stdout: stdout.String(), stderr: stderr.String()}
}

func decodeOne(t *testing.T, s string) ErrorObject {
	t.Helper()
	require.Equal(t, 1, strings.Count(s, "\n"), "want exactly one json line, got %q", s)
	var obj ErrorObject
	require.NoError(t, json.Unmarshal([]byte(s), &obj), "stdout is not an error object: %q", s)
	return obj
}

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"text", "json"} {
		f, err := ParseFormat(ok)
		require.NoError(t, err)
		assert.Equal(t, Format(ok), f)
	}
	_, err := ParseFormat("yaml")
	require.Error(t, err)
	assert.True(t, IsUsage(err), "an unknown --output value is a usage error")
}

// An extra is accepted only where the command declared it, and the error
// lists what this command takes, extras included.
func TestParseFormatExtras(t *testing.T) {
	f, err := ParseFormat("dotenv", "dotenv")
	require.NoError(t, err)
	assert.Equal(t, Format("dotenv"), f)

	for _, c := range []struct {
		in     string
		extras []Format
		want   string
	}{
		{"dotenv", nil, `unknown output format "dotenv" (supported: text, json)`},
		{"yaml", []Format{"dotenv"}, `unknown output format "yaml" (supported: text, json, dotenv)`},
		{"table", []Format{"yaml"}, `unknown output format "table" (supported: text, json, yaml)`},
		{"", nil, `unknown output format "" (supported: text, json)`},
	} {
		_, err := ParseFormat(c.in, c.extras...)
		require.EqualError(t, err, c.want)
		assert.True(t, IsUsage(err), "%q: an unknown --output value is a usage error", c.in)
	}
}

// The help names the formats the command takes, extras included.
func TestAddOutputFlagUsage(t *testing.T) {
	for _, c := range []struct {
		extras []Format
		want   string
	}{
		{nil, "Output format: text or json"},
		{[]Format{"dotenv"}, "Output format: text, json or dotenv"},
		{[]Format{"yaml", "toml"}, "Output format: text, json, yaml or toml"},
	} {
		var v Format
		cmd := &cobra.Command{Use: "x"}
		AddOutputFlag(cmd, &v, c.extras...)
		f := cmd.PersistentFlags().Lookup("output")
		require.NotNil(t, f)
		assert.Equal(t, c.want, f.Usage)
		assert.Equal(t, "o", f.Shorthand)
		assert.Equal(t, "text", f.DefValue)
	}
}

func TestExitCode(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	bg := context.Background()

	assert.Equal(t, 0, ExitCode(bg, nil))
	assert.Equal(t, ExitFailure, ExitCode(bg, errBoom))
	assert.Equal(t, ExitUsage, ExitCode(bg, Usage(errBoom)))
	assert.Equal(t, 7, ExitCode(bg, fmt.Errorf("wrapped: %w", &ExitError{Code: 7})))
	// An interrupt wins over whatever the unwinding command returned.
	assert.Equal(t, ExitInterrupted, ExitCode(canceled, &ExitError{Code: 7}))
	assert.Equal(t, ExitInterrupted, ExitCode(canceled, Usage(errBoom)))
	// Ctrl-C at a raw-mode prompt is a keypress, not a signal, and still an interrupt.
	assert.Equal(t, ExitInterrupted, ExitCode(bg, fmt.Errorf("reading the value: %w", input.ErrInterrupted)))
}

func TestAJSONFailureIsOneObjectOnStdout(t *testing.T) {
	r := execTree(context.Background(), testTree(fmt.Errorf("listing: %w", errBoom)), "thing", "list", "-o", "json")
	require.Error(t, r.err)
	obj := decodeOne(t, r.stdout)
	assert.Equal(t, "listing: boom", obj.Error)
	assert.Equal(t, ExitFailure, obj.Code)
	assert.Equal(t, kindBoom, obj.Kind)
	assert.Empty(t, r.stderr, "json mode writes nothing on stderr")
}

func TestAnUnclassifiedJSONFailureOmitsKind(t *testing.T) {
	r := execTree(context.Background(), testTree(errors.New("other")), "thing", "list", "-o", "json")
	require.Error(t, r.err)
	assert.NotContains(t, r.stdout, "kind")
}

// A runtime failure is the error alone: the command was invoked rightly, so
// neither its usage nor a pointer to its help follows.
func TestATextFailureIsTheErrorAlone(t *testing.T) {
	r := execTree(context.Background(), testTree(errBoom), "thing", "list")
	require.Error(t, r.err)
	assert.Empty(t, r.stdout)
	assert.Equal(t, "Error: boom\n", r.stderr)
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), r.err))
}

// A declined confirmation and an interrupt are the error alone, and so is a
// usage error a run returns after it was interrupted: it exits 130, and
// nothing about how it was invoked needs fixing.
func TestAnAbortOrInterruptIsTheErrorAlone(t *testing.T) {
	for name, fail := range map[string]error{
		"aborted":     input.ErrAborted,
		"interrupted": input.ErrInterrupted,
	} {
		t.Run(name, func(t *testing.T) {
			r := execTree(context.Background(), testTree(fail), "thing", "list")
			assert.Equal(t, "Error: "+name+"\n", r.stderr)
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	root := testTree(nil)
	leaf, _, err := root.Find([]string{"thing", "list"})
	require.NoError(t, err)
	leaf.RunE = func(*cobra.Command, []string) error {
		cancel()
		return Usage(errBoom)
	}
	r := execTree(canceled, root, "thing", "list")
	assert.Equal(t, ExitInterrupted, ExitCode(canceled, r.err))
	assert.Equal(t, "Error: boom\n", r.stderr)
}

// A usage error names what was wrong and points at the failing command's
// help, never printing the help itself.
func TestATextUsageErrorPointsAtHelp(t *testing.T) {
	cases := map[string]struct {
		args   []string
		stderr string
	}{
		"unknown flag": {
			[]string{"thing", "list", "--bogus"},
			"Error: unknown flag: --bogus\nRun astro thing list --help for usage.\n",
		},
		"unknown shorthand": {
			[]string{"thing", "list", "-z"},
			"Error: unknown shorthand flag: 'z' in -z\nRun astro thing list --help for usage.\n",
		},
		"bad flag value": {
			[]string{"thing", "list", "--all=maybe"},
			"Error: invalid argument \"maybe\" for \"--all\" flag: strconv.ParseBool: parsing \"maybe\": invalid syntax\n" +
				"Run astro thing list --help for usage.\n",
		},
		"wrong argument count": {
			[]string{"thing", "list", "extra"},
			"Error: unknown command \"extra\" for \"astro thing list\"\nRun astro thing list --help for usage.\n",
		},
		"missing required flag": {
			[]string{"thing", "create"},
			"Error: required flag(s) \"name\" not set\nRun astro thing create --help for usage.\n",
		},
		"unknown subcommand of a group": {
			[]string{"thing", "bogus"},
			"Error: unknown command \"bogus\" for \"astro thing\"\nRun astro thing --help for usage.\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := execTree(context.Background(), testTree(nil), c.args...)
			require.Error(t, r.err)
			assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
			assert.Empty(t, r.stdout)
			assert.Equal(t, c.stderr, r.stderr)
		})
	}
}

// An unknown flag that one of the command's flags is clearly close to
// suggests it, between the error and the pointer to help. At most one: two
// equally close are a guess, and a shorthand is close to every other letter.
func TestAnUnknownFlagSuggestsTheOneItIsCloseTo(t *testing.T) {
	cases := map[string]struct {
		args    []string
		suggest string
	}{
		"a typo":                             {[]string{"thing", "list", "--outptu", "json"}, "--output"},
		"a typo with its value":              {[]string{"thing", "list", "--alll=true"}, "--all"},
		"a different case":                   {[]string{"thing", "list", "--OUTPUT", "json"}, "--output"},
		"a prefix":                           {[]string{"thing", "create", "--na", "x"}, "--name"},
		"an inherited flag":                  {[]string{"thing", "list", "--verbos"}, "--verbose"},
		"nothing close":                      {[]string{"thing", "list", "--bogus"}, ""},
		"two equally close":                  {[]string{"thing", "list", "--al"}, ""},
		"a shorthand":                        {[]string{"thing", "list", "-p"}, ""},
		"a hidden flag is never offered":     {[]string{"thing", "list", "--secre"}, ""},
		"a deprecated flag is never offered": {[]string{"thing", "list", "--olds"}, ""},
		"a short name is held closer":        {[]string{"thing", "list", "--ws"}, ""},
		"a short name one edit away":         {[]string{"thing", "list", "--i"}, "--id"},
		"two edits on a short name":          {[]string{"thing", "list", "--bogs"}, ""},
		"swapped neighbors":                  {[]string{"thing", "list", "--tgas"}, "--tags"},
		"swapped neighbors mid-word":         {[]string{"thing", "list", "--froce"}, "--force"},
		"a long prefix":                      {[]string{"thing", "list", "--workspac", "x"}, "--workspace"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := testTree(nil)
			root.PersistentFlags().Bool("verbose", false, "")
			leaf, _, err := root.Find([]string{"thing", "list"})
			require.NoError(t, err)
			leaf.Flags().Bool("ali", false, "")
			leaf.Flags().Bool("secret", false, "")
			require.NoError(t, leaf.Flags().MarkHidden("secret"))
			leaf.Flags().Bool("old", false, "")
			require.NoError(t, leaf.Flags().MarkDeprecated("old", "gone"))
			leaf.Flags().Bool("id", false, "")
			leaf.Flags().String("tags", "", "")
			leaf.Flags().Bool("force", false, "")
			leaf.Flags().String("workspace", "", "")

			r := execTree(context.Background(), root, c.args...)
			require.Error(t, r.err)
			assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
			lines := strings.Split(strings.TrimSuffix(r.stderr, "\n"), "\n")
			if c.suggest == "" {
				assert.Len(t, lines, 2, "stderr: %q", r.stderr)
				assert.NotContains(t, r.stderr, "Did you mean")
				return
			}
			require.Len(t, lines, 3, "stderr: %q", r.stderr)
			assert.Equal(t, "Did you mean "+c.suggest+"?", lines[1])
			assert.Equal(t, "Run astro "+c.args[0]+" "+c.args[1]+" --help for usage.", lines[2])
		})
	}
}

// Flag suggestions follow the settings command suggestions do: none when a
// command on the way disables them.
func TestFlagSuggestionsHonourDisableSuggestions(t *testing.T) {
	for _, on := range []string{"root", "parent"} {
		t.Run(on, func(t *testing.T) {
			root := testTree(nil)
			disabled := root
			if on == "parent" {
				group, _, err := root.Find([]string{"thing"})
				require.NoError(t, err)
				disabled = group
			}
			disabled.DisableSuggestions = true
			// A prefix, so it would be suggested at any distance, 0 included.
			r := execTree(context.Background(), root, "thing", "list", "--outp", "json")
			assert.Equal(t, "Error: unknown flag: --outp\nRun astro thing list --help for usage.\n", r.stderr)
		})
	}
}

// Under --output json a usage error is the error object alone: no
// suggestion, no pointer to help, nothing on stderr.
func TestAJSONUsageErrorCarriesNoSuggestion(t *testing.T) {
	r := execTree(context.Background(), testTree(nil), "thing", "list", "--alll", "-o", "json")
	require.Error(t, r.err)
	assert.Empty(t, r.stderr)
	assert.Equal(t, ErrorObject{Error: "unknown flag: --alll", Code: ExitUsage, Kind: KindUsage}, decodeOne(t, r.stdout))
}

// A command invoked wrongly points at its help even when it silenced its
// usage up front, as all of astro local does: that kept cobra's usage block
// out, and its help is still where the right invocation is.
func TestAMisinvokedSilencedCommandStillPointsAtHelp(t *testing.T) {
	cases := map[string]struct {
		args   []string
		stderr string
	}{
		"unknown flag": {
			[]string{"thing", "list", "--alll"},
			"Error: unknown flag: --alll\nDid you mean --all?\nRun astro thing list --help for usage.\n",
		},
		"wrong argument count": {
			[]string{"thing", "list", "extra"},
			"Error: unknown command \"extra\" for \"astro thing list\"\nRun astro thing list --help for usage.\n",
		},
		"bad --output value": {
			[]string{"thing", "list", "-o", "yaml"},
			"Error: unknown output format \"yaml\" (supported: text, json)\nRun astro thing list --help for usage.\n",
		},
		"missing required flag": {
			[]string{"thing", "create"},
			"Error: required flag(s) \"name\" not set\nRun astro thing create --help for usage.\n",
		},
		"unknown subcommand of a group with no RunE": {
			[]string{"thing", "bogus"},
			"Error: unknown command \"bogus\" for \"astro thing\"\nRun astro thing --help for usage.\n",
		},
		"unknown subcommand of a group answering with GroupHelp": {
			[]string{"runner", "srtat"},
			"Error: unknown command \"srtat\" for \"astro runner\"\n\nDid you mean this?\n\tstart\n\n" +
				"Run astro runner --help for usage.\n",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := testTree(nil)
			// A group with a RunE of its own, as astro local is.
			runner := &cobra.Command{Use: "runner", Args: cobra.ArbitraryArgs, RunE: GroupHelp}
			runner.AddCommand(&cobra.Command{Use: "start", RunE: func(*cobra.Command, []string) error { return nil }})
			root.AddCommand(runner)
			// Every command below the root silenced, as astro local silences
			// its whole tree when it builds it.
			for _, c := range root.Commands() {
				walk(c, func(c *cobra.Command) { c.SilenceUsage = true })
			}
			r := execTree(context.Background(), root, c.args...)
			assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
			assert.Equal(t, c.stderr, r.stderr)
		})
	}
}

// A root that silenced its usage silences the pointer for the whole tree,
// and Execute puts the setting back after the run that forced it on.
func TestASilencedRootPrintsTheErrorAlone(t *testing.T) {
	root := testTree(nil)
	root.SilenceUsage = true
	for _, args := range [][]string{{"--bogus"}, {"thing", "list", "--alll"}, {"bogus"}} {
		r := execTree(context.Background(), root, args...)
		assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
		assert.Equal(t, "Error: "+r.err.Error()+"\n", r.stderr, "%v", args)
	}
	assert.True(t, root.SilenceUsage)
}

// A usage error a silenced command returns from its run is judged by its own
// mark, not by its wording: astro local's refusal of a 1.x verb reads like
// cobra's unknown command, and still gets no pointer.
func TestARunsUsageErrorIsNotTakenForCobrasByItsWording(t *testing.T) {
	root := testTree(nil)
	leaf, _, err := root.Find([]string{"thing", "list"})
	require.NoError(t, err)
	leaf.SilenceUsage = true
	leaf.RunE = func(*cobra.Command, []string) error {
		return Usage(errors.New(`unknown command "init" for "astro thing list". Use astro init`))
	}
	r := execTree(context.Background(), root, "thing", "list")
	assert.Equal(t, "Error: unknown command \"init\" for \"astro thing list\". Use astro init\n", r.stderr)
}

// A usage error the command returns once it runs points at its help unless
// the command silenced its usage first, which is how it says its help would
// not settle it (a Deployment name several Deployments share).
func TestAUsageErrorFromTheRunHonoursSilenceUsage(t *testing.T) {
	ambiguous := Usage(errors.New("2 Deployments are named prod"))

	r := execTree(context.Background(), testTree(ambiguous), "thing", "list")
	assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
	assert.Equal(t, "Error: 2 Deployments are named prod\nRun astro thing list --help for usage.\n", r.stderr)

	root := testTree(nil)
	leaf, _, err := root.Find([]string{"thing", "list"})
	require.NoError(t, err)
	leaf.RunE = func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return ambiguous
	}
	r = execTree(context.Background(), root, "thing", "list")
	assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
	assert.Equal(t, "Error: 2 Deployments are named prod\n", r.stderr)
}

// A removed command's stub is hidden and its message names what replaced it,
// so its refusal is the message alone.
func TestARemovedCommandIsItsMessageAlone(t *testing.T) {
	root := testTree(nil)
	root.AddCommand(RemovedCommand("dev", nil, "Removed in v2", func(*cobra.Command, []string) error {
		return Usage(errors.New("astro dev was removed in Astro CLI v2. Use astro local instead."))
	}))
	r := execTree(context.Background(), root, "dev", "start", "--bogus")
	require.Error(t, r.err)
	assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
	assert.Equal(t, "Error: astro dev was removed in Astro CLI v2. Use astro local instead.\n", r.stderr)
}

func TestASilencedRootPrintsNothing(t *testing.T) {
	root := testTree(errBoom)
	root.SilenceErrors = true
	root.SilenceUsage = true
	r := execTree(context.Background(), root, "thing", "list")
	require.Error(t, r.err)
	assert.Empty(t, r.stderr)
	assert.True(t, root.SilenceErrors, "Execute restores the root's own setting")
}

func TestACommandWithNoOutputFlagStaysText(t *testing.T) {
	r := execTree(context.Background(), testTree(errBoom), "thing", "plain", "-o", "json")
	require.Error(t, r.err)
	assert.Empty(t, r.stdout, "a command with no --output never switches to json")
}

// What a command reported itself is not reported again, in either mode.
func TestAlreadyReportedFailuresAddNothing(t *testing.T) {
	for name, fail := range map[string]error{
		"exit error":  &ExitError{Code: 3},
		"json shown":  JSONShown(errBoom),
		"wrapped one": fmt.Errorf("x: %w", &ExitError{Code: 3}),
	} {
		for _, args := range [][]string{{"thing", "list"}, {"thing", "list", "-o", "json"}} {
			t.Run(name+" "+strings.Join(args, " "), func(t *testing.T) {
				r := execTree(context.Background(), testTree(fail), args...)
				require.Error(t, r.err)
				assert.Empty(t, r.stdout)
				assert.Empty(t, r.stderr)
			})
		}
	}
}

func TestUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"unknown flag":          {"thing", "list", "--bogus"},
		"bad flag value":        {"thing", "list", "--all=maybe"},
		"wrong argument count":  {"thing", "list", "extra"},
		"unknown root command":  {"bogus"},
		"missing required flag": {"thing", "create"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			r := execTree(context.Background(), testTree(nil), args...)
			require.Error(t, r.err)
			assert.True(t, IsUsage(r.err), "%v: %v", args, r.err)
			assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
			assert.Contains(t, r.stderr, "Error: ")
		})
	}
}

// markedUsage is an error a package that cannot import cliout marks as a
// usage error itself, or declines to.
type markedUsage bool

func (m markedUsage) Error() string      { return "marked" }
func (m markedUsage) IsUsageError() bool { return bool(m) }

// An error that marks itself is a usage error wrapped or not, and one that
// declines is not.
func TestAnErrorCanMarkItselfAUsageError(t *testing.T) {
	assert.True(t, IsUsage(markedUsage(true)))
	assert.True(t, IsUsage(fmt.Errorf("deleting: %w", markedUsage(true))))
	assert.False(t, IsUsage(markedUsage(false)))
	assert.Equal(t, ExitUsage, ExitCode(context.Background(), markedUsage(true)))
}

// A flag error func the root already has is not replaced by Execute's: it is
// handed the error first, and what it returns is still reported as a usage
// error, in either mode.
func TestTheRootsOwnFlagErrorFuncStillRuns(t *testing.T) {
	for _, args := range [][]string{
		{"thing", "list", "--bogus"},
		{"thing", "list", "--bogus", "-o", "json"},
		{"thing", "plain", "--bogus"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := testTree(nil)
			var seen []string
			root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
				seen = append(seen, c.Name()+": "+err.Error())
				return err
			})
			r := execTree(context.Background(), root, args...)
			require.Error(t, r.err)
			assert.True(t, IsUsage(r.err))
			assert.Equal(t, []string{args[1] + ": unknown flag: --bogus"}, seen)
		})
	}

	root := testTree(nil)
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return nil })
	r := execTree(context.Background(), root, "thing", "plain", "--bogus")
	require.EqualError(t, r.err, "unknown flag: --bogus", "a func returning nil does not turn the error into success")
	assert.True(t, IsUsage(r.err))
}

// Find's failure is cobra's error and suggestions, then the same pointer to
// help as every other usage error.
func TestAnUnknownRootCommandPointsAtHelp(t *testing.T) {
	r := execTree(context.Background(), testTree(nil), "bogus")
	assert.Equal(t, "Error: unknown command \"bogus\" for \"astro\"\nRun astro --help for usage.\n", r.stderr)
}

// The root failing itself points at the root's help, and Execute puts the
// root's own SilenceUsage back after the run that forced it on.
func TestARootUsageErrorPointsAtTheRootsHelp(t *testing.T) {
	root := testTree(nil)
	r := execTree(context.Background(), root, "--bogus")
	require.Error(t, r.err)
	assert.Equal(t, "Error: unknown flag: --bogus\nRun astro --help for usage.\n", r.stderr)
	assert.False(t, root.SilenceUsage, "Execute restores the root's own setting")
}

// cobra adds its completion commands during execution, after markUsageErrors
// has walked the tree, so their validators are never wrapped. Their argument
// errors are usage errors all the same, because cobra.NoArgs words its error
// "unknown command ...", one of the untyped messages IsUsage recognizes.
func TestCobrasCompletionCommandsHaveUsageErrors(t *testing.T) {
	r := execTree(context.Background(), testTree(nil), "completion", "bash", "extra")
	require.Error(t, r.err)
	assert.True(t, IsUsage(r.err), "completion bash extra: %v", r.err)
	assert.Equal(t, ExitUsage, ExitCode(context.Background(), r.err))
}

// A usage error in json mode is the usage object, whether or not flag parsing
// got as far as --output before it stopped.
func TestAJSONUsageErrorIsAnObject(t *testing.T) {
	for _, args := range [][]string{
		{"thing", "list", "-o", "json", "--bogus"},
		{"thing", "list", "--bogus", "-o", "json"},
		{"thing", "list", "--bogus", "--output=json"},
		{"thing", "list", "--bogus", "-ojson"},
		{"thing", "list", "extra", "-o", "json"},
		{"thing", "create", "-o", "json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := execTree(context.Background(), testTree(nil), args...)
			require.Error(t, r.err)
			obj := decodeOne(t, r.stdout)
			assert.Equal(t, KindUsage, obj.Kind)
			assert.Equal(t, ExitUsage, obj.Code)
			assert.Empty(t, r.stderr)
		})
	}
}

func TestArgsAskForJSON(t *testing.T) {
	yes := [][]string{
		{"-o", "json"},
		{"--output", "json"},
		{"--output=json"},
		{"-o=json"},
		{"-ojson"},
	}
	for _, args := range yes {
		assert.True(t, ArgsAskForJSON(args, "o"), "%v", args)
	}
	no := [][]string{
		{"-o", "text"},
		{"--", "-o", "json"},
		{"-o"},
		{"--outputjson"},
		{"--other", "json"},
	}
	for _, args := range no {
		assert.False(t, ArgsAskForJSON(args, "o"), "%v", args)
	}
	// With no shorthand registered, -o is some other flag.
	assert.False(t, ArgsAskForJSON([]string{"-o", "json"}, ""))
	assert.True(t, ArgsAskForJSON([]string{"--output", "json"}, ""))
}

// cobra returns some usage errors untyped, and IsUsage recognizes them by
// cobra's own wording. This drives each one through the cobra this module
// pins, so an upgrade that rewords one fails here instead of quietly turning
// exit 2 back into 1.
func TestCobraUsageMessagesAreRecognized(t *testing.T) {
	newCmd := func(setup func(*cobra.Command)) *cobra.Command {
		root := &cobra.Command{Use: "astro", SilenceErrors: true, SilenceUsage: true}
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		leaf.Flags().String("a", "", "")
		leaf.Flags().String("b", "", "")
		setup(leaf)
		root.AddCommand(leaf)
		return root
	}
	cases := map[string]struct {
		setup func(*cobra.Command)
		args  []string
	}{
		"unknown command": {func(*cobra.Command) {}, []string{"nope"}},
		"required flag":   {func(c *cobra.Command) { _ = c.MarkFlagRequired("a") }, []string{"leaf"}},
		"required together": {
			func(c *cobra.Command) { c.MarkFlagsRequiredTogether("a", "b") },
			[]string{"leaf", "--a", "x"},
		},
		"one required": {
			func(c *cobra.Command) { c.MarkFlagsOneRequired("a", "b") },
			[]string{"leaf"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := newCmd(tc.setup)
			root.SetArgs(tc.args)
			err := root.Execute()
			require.Error(t, err)
			assert.True(t, IsUsage(err), "cobra's wording changed? %q", err)
		})
	}
}

// Mutually exclusive flags fail with "if any flags in the group ... none of the
// others can be", which shares its opening with "required together" — the
// prefix covers both.
func TestMutuallyExclusiveFlagsAreUsage(t *testing.T) {
	root := &cobra.Command{Use: "astro", SilenceErrors: true, SilenceUsage: true}
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().String("a", "", "")
	leaf.Flags().String("b", "", "")
	leaf.MarkFlagsMutuallyExclusive("a", "b")
	root.AddCommand(leaf)
	root.SetArgs([]string{"leaf", "--a", "x", "--b", "y"})
	err := root.Execute()
	require.Error(t, err)
	assert.True(t, IsUsage(err), "%q", err)
}

func TestKindsOf(t *testing.T) {
	first := errors.New("first")
	k := Kinds{
		{Kind: "a", Match: Sentinel(first)},
		{Kind: "b", Match: Sentinel(errBoom)},
	}
	assert.Equal(t, ProblemKind("b"), k.Of(fmt.Errorf("x: %w", errBoom)))
	assert.Equal(t, ProblemKind("a"), k.Of(errors.Join(errBoom, first)), "the first row wins")
	assert.Equal(t, KindUsage, k.Of(Usage(errBoom)), "usage wins over every row")
	assert.Equal(t, ProblemKind(""), k.Of(errors.New("other")))
	assert.Equal(t, ProblemKind(""), k.Of(nil))
}
