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

func TestATextFailureIsCobrasError(t *testing.T) {
	r := execTree(context.Background(), testTree(errBoom), "thing", "list")
	require.Error(t, r.err)
	assert.Empty(t, r.stdout)
	assert.True(t, strings.HasPrefix(r.stderr, "Error: boom\n"), "stderr: %q", r.stderr)
	// Not silenced, so the usage follows the error, as cobra prints it.
	assert.Contains(t, r.stderr, "Usage:")
}

func TestSilencedUsageIsHonoured(t *testing.T) {
	root := testTree(errBoom)
	leaf, _, err := root.Find([]string{"thing", "list"})
	require.NoError(t, err)
	leaf.SilenceUsage = true
	r := execTree(context.Background(), root, "thing", "list")
	assert.Equal(t, "Error: boom\n", r.stderr)
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

// Find's failure carries cobra's pointer to help rather than the usage block.
func TestAnUnknownRootCommandPointsAtHelp(t *testing.T) {
	r := execTree(context.Background(), testTree(nil), "bogus")
	assert.Equal(t, "Error: unknown command \"bogus\" for \"astro\"\nRun 'astro --help' for usage.\n", r.stderr)
}

// Execute forces the root's SilenceUsage on for the run, so the root failing
// itself is the case where that forced value must not be read as its own.
func TestARootFailurePrintsTheRootsUsage(t *testing.T) {
	r := execTree(context.Background(), testTree(nil), "--bogus")
	require.Error(t, r.err)
	assert.True(t, strings.HasPrefix(r.stderr, "Error: unknown flag: --bogus\n"), "stderr: %q", r.stderr)
	assert.Contains(t, r.stderr, "Usage:")

	root := testTree(nil)
	root.SilenceUsage = true
	r = execTree(context.Background(), root, "--bogus")
	assert.Equal(t, "Error: unknown flag: --bogus\n", r.stderr, "a root that silenced its usage keeps it silent")
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
		assert.True(t, argsAskForJSON(args, "o"), "%v", args)
	}
	no := [][]string{
		{"-o", "text"},
		{"--", "-o", "json"},
		{"-o"},
		{"--outputjson"},
		{"--other", "json"},
	}
	for _, args := range no {
		assert.False(t, argsAskForJSON(args, "o"), "%v", args)
	}
	// With no shorthand registered, -o is some other flag.
	assert.False(t, argsAskForJSON([]string{"-o", "json"}, ""))
	assert.True(t, argsAskForJSON([]string{"--output", "json"}, ""))
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
