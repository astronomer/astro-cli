package cliout

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outputTree is a root whose pre-run counts into preRuns, over a group that
// registers --output for its family (with extras) and a leaf under it.
func outputTree(preRuns, runs *int, extras ...Format) (root *cobra.Command, output *Format) {
	output = new(Format)
	root = &cobra.Command{
		Use:               "astro",
		PersistentPreRunE: func(*cobra.Command, []string) error { *preRuns++; return nil },
	}
	group := &cobra.Command{Use: "group"}
	AddOutputFlag(group, output, extras...)
	group.AddCommand(&cobra.Command{
		Use:  "leaf",
		RunE: func(*cobra.Command, []string) error { *runs++; return nil },
	})
	root.AddCommand(group)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root, output
}

// The flag refuses a format the command does not offer while cobra parses
// flags: before any pre-run (where the CLI refreshes its token and records
// telemetry) and before the command runs. The refusal is ParseFormat's own
// usage error, without pflag's "invalid argument" in front of it, however
// the command is run.
func TestAnUnknownFormatIsRefusedBeforeAnyPreRun(t *testing.T) {
	for _, args := range [][]string{
		{"group", "leaf", "-o", "yaml"},
		{"group", "leaf", "--output", "yaml"},
		{"group", "leaf", "--output=yaml"},
		{"group", "leaf", "-oyaml"},
	} {
		var preRuns, runs int
		root, _ := outputTree(&preRuns, &runs)
		root.SetArgs(args)
		err := root.Execute()
		require.EqualError(t, err, `unknown output format "yaml" (supported: text, json)`, args)
		assert.True(t, IsUsage(err), "%v: %v is not a usage error", args, err)
		assert.Zero(t, preRuns, "%v: the pre-run ran before the refusal", args)
		assert.Zero(t, runs, args)
	}
}

// The extras a command declares are formats it offers, and the default is text.
func TestTheOutputFlagHonorsExtras(t *testing.T) {
	var preRuns, runs int
	root, output := outputTree(&preRuns, &runs, "yaml")
	assert.Equal(t, FormatText, *output)
	root.SetArgs([]string{"group", "leaf", "-o", "yaml"})
	require.NoError(t, root.Execute())
	assert.Equal(t, Format("yaml"), *output)
	assert.Equal(t, 1, runs)

	root, _ = outputTree(&preRuns, &runs, "yaml")
	root.SetArgs([]string{"group", "leaf", "-o", "dotenv"})
	require.EqualError(t, root.Execute(), `unknown output format "dotenv" (supported: text, json, yaml)`)
}

// Under Execute the refusal is reported as before: exit 2, and the error object
// only when the run asked for json, which a refused -o did not.
func TestAnUnknownFormatUnderExecute(t *testing.T) {
	var preRuns, runs int
	root, _ := outputTree(&preRuns, &runs)
	var out, errOut bytes.Buffer
	root.SetErr(&errOut)
	err := Execute(t.Context(), root, []string{"group", "leaf", "-o", "yaml"}, &out, nil)
	assert.Equal(t, ExitUsage, ExitCode(t.Context(), err))
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), `Error: unknown output format "yaml" (supported: text, json)`)
	assert.Zero(t, preRuns)
}

// groupOf finds the group, which registered --output, in an outputTree.
func groupOf(t *testing.T, root *cobra.Command) *cobra.Command {
	t.Helper()
	group, _, err := root.Find([]string{"group"})
	require.NoError(t, err)
	return group
}

var errHasNoValues = errors.New("this listing has no values")

// A command with more to say about a refused value says it, still as a usage
// error before any pre-run, for the commands under it that share its
// --output too; a value it has nothing to add about, and other flag errors,
// are reported as they would be without it.
func TestOnBadFormatExplainsTheRefusal(t *testing.T) {
	var preRuns, runs int
	root, _ := outputTree(&preRuns, &runs)
	OnBadFormat(groupOf(t, root), func(value string, refused error) error {
		if value == "dotenv" {
			return errHasNoValues
		}
		return refused
	})

	root.SetArgs([]string{"group", "leaf", "-o", "dotenv"})
	err := root.Execute()
	require.ErrorIs(t, err, errHasNoValues)
	assert.True(t, IsUsage(err))

	root.SetArgs([]string{"group", "leaf", "-o", "yaml"})
	require.EqualError(t, root.Execute(), `unknown output format "yaml" (supported: text, json)`)

	root.SetArgs([]string{"group", "leaf", "--bogus"})
	err = root.Execute()
	require.EqualError(t, err, "unknown flag: --bogus")
	assert.True(t, IsUsage(err))
	assert.Zero(t, preRuns)
	assert.Zero(t, runs)
}

// An explanation that returns nil never swallows the refusal into success.
func TestOnBadFormatReturningNilKeepsTheRefusal(t *testing.T) {
	var preRuns, runs int
	root, _ := outputTree(&preRuns, &runs)
	OnBadFormat(groupOf(t, root), func(string, error) error { return nil })
	root.SetArgs([]string{"group", "leaf", "-o", "yaml"})
	err := root.Execute()
	require.EqualError(t, err, `unknown output format "yaml" (supported: text, json)`)
	assert.True(t, IsUsage(err))
	assert.Zero(t, runs)
}

// The explanation lives on the --output flag, so OnBadFormat on a command
// that did not register one with AddOutputFlag is a programming error.
func TestOnBadFormatNeedsTheCommandsOwnOutputFlag(t *testing.T) {
	explain := func(string, error) error { return nil }
	assert.Panics(t, func() { OnBadFormat(&cobra.Command{Use: "bare"}, explain) }, "no --output")

	foreign := &cobra.Command{Use: "foreign"}
	foreign.PersistentFlags().String("output", "", "")
	assert.Panics(t, func() { OnBadFormat(foreign, explain) }, "an --output that is not cliout's")

	var preRuns, runs int
	root, _ := outputTree(&preRuns, &runs)
	leaf, _, err := root.Find([]string{"group", "leaf"})
	require.NoError(t, err)
	assert.Panics(t, func() { OnBadFormat(leaf, explain) }, "a leaf sharing its parent's --output")
}

// A flag error that is not a refused --output is the parent's to report: the
// parent's func is looked up when the error happens, so one set after
// AddOutputFlag ran (Execute sets the root's) still gets it, and its usage
// error is not wrapped twice.
func TestOtherFlagErrorsGoToTheParentsFuncAtCallTime(t *testing.T) {
	var preRuns, runs int
	root, _ := outputTree(&preRuns, &runs)
	errOwn := errors.New("the root's word on it")
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return Usage(errOwn) })

	root.SetArgs([]string{"group", "leaf", "--bogus"})
	err := root.Execute()
	assert.True(t, IsUsage(err))
	assert.Same(t, errOwn, errors.Unwrap(err), "the root's usage error was wrapped again")

	root.SetArgs([]string{"group", "leaf", "-o", "yaml"})
	require.EqualError(t, root.Execute(), `unknown output format "yaml" (supported: text, json)`)
}

// A parent's flag error func that returns nil cannot turn a flag error into
// success: the original error stands, as a usage error.
func TestAParentFuncReturningNilKeepsTheFlagError(t *testing.T) {
	var preRuns, runs int
	root, _ := outputTree(&preRuns, &runs)
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return nil })

	root.SetArgs([]string{"group", "leaf", "--bogus"})
	err := root.Execute()
	require.EqualError(t, err, "unknown flag: --bogus")
	assert.True(t, IsUsage(err))
	assert.Zero(t, runs)
}

// A Lazy payload is built only when it is published: never in text mode, and
// once in json mode, where the observer sees what it built.
func TestLazyIsBuiltOnlyUnderJSON(t *testing.T) {
	prev := EmitObserver
	var seen []any
	EmitObserver = func(v any) { seen = append(seen, v) }
	t.Cleanup(func() { EmitObserver = prev })

	builds := 0
	lazy := Lazy(func() any { builds++; return map[string]int{"count": 1} })
	text := func(w io.Writer) error { _, err := io.WriteString(w, "one\n"); return err }

	var out bytes.Buffer
	require.NoError(t, Renderer{Format: FormatText, Out: &out}.Emit(lazy, text))
	assert.Equal(t, "one\n", out.String())
	assert.Zero(t, builds, "text mode built the payload")
	assert.Empty(t, seen)

	out.Reset()
	require.NoError(t, Renderer{Format: FormatJSON, Out: &out}.Emit(lazy, text))
	assert.JSONEq(t, `{"count":1}`, out.String())
	assert.Equal(t, 1, builds)
	assert.Equal(t, []any{map[string]int{"count": 1}}, seen)
}
