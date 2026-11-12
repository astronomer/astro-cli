package cliout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/input"
)

// promptTree is a root whose leaves ask the way the CLI's commands do: a
// confirmation a flag can skip, and a free-text answer. Each records what it
// was told, so a test can see a prompt that was answered.
func promptTree(answered *string) *cobra.Command {
	var output Format
	var force bool
	root := &cobra.Command{Use: "astro"}
	AddOutputFlag(root, &output)
	input.AddFlag(root.PersistentFlags())
	del := &cobra.Command{
		Use: "delete",
		RunE: func(*cobra.Command, []string) error {
			if force {
				return nil
			}
			yes, err := input.Confirm("Delete prod?", input.AnsweredBy("--force"))
			if err != nil {
				return err
			}
			*answered = fmt.Sprint(yes)
			return nil
		},
	}
	del.Flags().BoolVarP(&force, "force", "f", false, "")
	create := &cobra.Command{
		Use: "create",
		RunE: func(*cobra.Command, []string) error {
			name, err := input.Text("Name: ")
			if err != nil {
				return fmt.Errorf("creating: %w", err)
			}
			*answered = name
			return nil
		},
	}
	root.AddCommand(del, create)
	return root
}

// stdinHolding points os.Stdin at a pipe holding answer for the rest of the
// test, and returns a check that nothing read it.
func stdinHolding(t *testing.T, answer string) (unread func() bool) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(answer)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	stdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin; r.Close() })
	return func() bool {
		buf := make([]byte, len(answer))
		n, _ := r.Read(buf)
		return n == len(answer)
	}
}

// Under --output json a command asks nothing: the prompt is refused before it
// reads stdin, and the run publishes one input_required object naming the
// question and the flag that answers it.
func TestAJSONRunNeverPrompts(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			"a confirmation",
			[]string{"delete", "-o", "json"},
			`this command needs to ask "Delete prod?"; with --output json it cannot — pass --force`,
		},
		{
			"free text, wrapped by its command",
			[]string{"create", "--output=json"},
			`creating: this command needs to ask "Name"; with --output json it cannot — pass the answer as a flag`,
		},
		{
			"the flag before the command",
			[]string{"-ojson", "create"},
			`creating: this command needs to ask "Name"; with --output json it cannot — pass the answer as a flag`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unread := stdinHolding(t, "y\n")
			answered := ""
			r := execTree(context.Background(), promptTree(&answered), tc.args...)

			require.Error(t, r.err)
			obj := decodeOne(t, r.stdout)
			assert.Equal(t, KindInputRequired, obj.Kind)
			assert.Equal(t, ExitFailure, obj.Code)
			assert.Equal(t, ExitFailure, ExitCode(context.Background(), r.err))
			assert.Equal(t, tc.want, obj.Error)
			assert.Empty(t, r.stderr, "json mode writes nothing on stderr")
			assert.Empty(t, answered, "the prompt was answered")
			assert.True(t, unread(), "a json run read stdin")
		})
	}
}

// The flag that answers the question is still the way through.
func TestAJSONRunAnsweredByAFlagSucceeds(t *testing.T) {
	stdinHolding(t, "")
	answered := ""
	r := execTree(context.Background(), promptTree(&answered), "delete", "--force", "-o", "json")
	require.NoError(t, r.err)
	assert.Empty(t, r.stdout)
}

// Text mode asks exactly as before.
func TestATextRunStillPrompts(t *testing.T) {
	stdinHolding(t, "y\n")
	answered := ""
	r := execTree(context.Background(), promptTree(&answered), "delete")
	require.NoError(t, r.err)
	assert.Equal(t, "true", answered)

	stdinHolding(t, "prod\n")
	r = execTree(context.Background(), promptTree(&answered), "create", "-o", "text")
	require.NoError(t, r.err)
	assert.Equal(t, "prod", answered)
}

// The guard lasts the run and no longer: a prompt after Execute returns, or in
// a run with no json, is asked.
func TestTheGuardEndsWithTheRun(t *testing.T) {
	stdinHolding(t, "")
	answered := ""
	execTree(context.Background(), promptTree(&answered), "delete", "-o", "json")
	assert.NoError(t, input.MayAsk("after"))
}

// input_required is cliout's own kind, found through any wrap, and a usage
// error still wins over it.
func TestInputRequiredKind(t *testing.T) {
	refused := input.Required(errors.New("pass --deployment"))
	assert.Equal(t, KindInputRequired, testKinds.Of(fmt.Errorf("deploying: %w", refused)))
	assert.Equal(t, KindInputRequired, testKinds.Of(errors.Join(errBoom, refused)), "it wins over a table's row")
	assert.Equal(t, KindUsage, testKinds.Of(Usage(refused)))
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), refused))
}

// Execute hands the run's context to the questions: the first Ctrl-C (the
// context canceled) ends a (y/n) waiting for a line at once, and the run
// exits 130 with the error on a line of its own.
func TestAnInterruptEndsAQuestionAtOnce(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin; w.Close(); r.Close() })

	root := &cobra.Command{Use: "astro"}
	root.AddCommand(&cobra.Command{
		Use: "delete",
		RunE: func(*cobra.Command, []string) error {
			return input.ConfirmOrAbort("Delete prod?", input.AnsweredBy("--yes"))
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan run, 1)
	go func() { done <- execTree(ctx, root, "delete") }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case got := <-done:
		require.ErrorIs(t, got.err, input.ErrInterrupted)
		assert.Equal(t, ExitInterrupted, ExitCode(ctx, got.err))
		assert.Empty(t, got.stdout)
	case <-time.After(2 * time.Second):
		t.Fatal("the question went on waiting after the interrupt")
	}
}

// A command that ends declined, as every converted confirmation does, is
// reported like any failure: in text, "Error: aborted" on stderr and nothing
// on stdout; under --output json, the one error object, kind aborted, and
// nothing on stderr. Exit 1 either way.
func TestADeclinedRunFailsAborted(t *testing.T) {
	tree := func() *cobra.Command {
		var output Format
		root := &cobra.Command{Use: "astro"}
		AddOutputFlag(root, &output)
		root.AddCommand(&cobra.Command{
			Use:  "delete",
			RunE: func(*cobra.Command, []string) error { return fmt.Errorf("deleting: %w", input.ErrAborted) },
		})
		return root
	}

	r := execTree(context.Background(), tree(), "delete")
	assert.ErrorIs(t, r.err, input.ErrAborted)
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), r.err))
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "Error: deleting: aborted")

	r = execTree(context.Background(), tree(), "delete", "-o", "json")
	obj := decodeOne(t, r.stdout)
	assert.Equal(t, KindAborted, obj.Kind)
	assert.Equal(t, ExitFailure, obj.Code)
	assert.Equal(t, "deleting: aborted", obj.Error)
	assert.Empty(t, r.stderr)
}

// aborted is cliout's own kind for a confirmation answered no, found through
// any wrap, exit 1 (not 130, an interrupt). A question that could not be asked
// is input_required, never aborted.
func TestAbortedKind(t *testing.T) {
	declined := fmt.Errorf("deleting: %w", input.ErrAborted)
	assert.Equal(t, KindAborted, testKinds.Of(declined))
	assert.Equal(t, KindAborted, testKinds.Of(errors.Join(errBoom, input.ErrAborted)), "it wins over a table's row")
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), declined))
	assert.Equal(t, KindInputRequired, testKinds.Of(input.Required(fmt.Errorf("asked: %w", input.ErrAborted))))
}

// What answers a shared picker is per command and per run: a hint one Execute
// run set is gone in the next, in the same process.
func TestHintsDoNotOutliveTheRun(t *testing.T) {
	t.Cleanup(input.ClearHints)
	hinted := &cobra.Command{
		Use: "hint",
		RunE: func(*cobra.Command, []string) error {
			input.HintAnswer("a thing", "--thing")
			assert.Equal(t, "--thing", input.ApplyHint(&input.RequiredError{About: "a thing"}).Flag)
			return nil
		},
	}
	var seen string
	plain := &cobra.Command{
		Use: "plain",
		RunE: func(*cobra.Command, []string) error {
			seen = input.ApplyHint(&input.RequiredError{About: "a thing"}).Flag
			return nil
		},
	}
	root := &cobra.Command{Use: "astro"}
	root.AddCommand(hinted, plain)

	require.NoError(t, execTree(context.Background(), root, "hint").err)
	assert.Empty(t, input.ApplyHint(&input.RequiredError{About: "a thing"}).Flag, "cleared when the run ended")

	input.HintAnswer("a thing", "--leaked")
	require.NoError(t, execTree(context.Background(), root, "plain").err)
	assert.Empty(t, seen, "cleared when the next run began")
}

// writtenToStderr is what f wrote to the process's stderr, where the pkg/input
// prompts write their questions.
func writtenToStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = saved }()
	f()
	require.NoError(t, w.Close())
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(b)
}

// The run's interaction mode is resolved once, by Execute: json, a truthy CI
// or --non-interactive refuse every question before it is written or read,
// naming the flag that answers it; --non-interactive=false asks despite CI,
// but never under json; any other run asks.
func TestTheInteractionModeIsResolvedOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ci     string
		args   []string
		refuse string // the reason the refusal gives; "" when the run asks
		// noStdin and noStderr: that stream is not a terminal.
		noStdin, noStderr bool
	}{
		{"nothing set asks, at a terminal", "", []string{"delete"}, "", false, false},
		{"stdin a pipe", "", []string{"delete"}, "stdin is not a terminal", true, false},
		{"stderr not a terminal", "", []string{"delete"}, "stderr is not a terminal", false, true},
		{"stdin a pipe, --non-interactive=false still refuses", "", []string{"delete", "--non-interactive=false"}, "stdin is not a terminal", true, false},
		{"stderr not a terminal, CI=true and --non-interactive=false still refuses", "true", []string{"delete", "--non-interactive=false"}, "stderr is not a terminal", false, true},
		{"CI=true", "true", []string{"delete"}, "with CI set it cannot", false, false},
		{"CI=1", "1", []string{"delete"}, "with CI set it cannot", false, false},
		{"CI=false asks", "false", []string{"delete"}, "", false, false},
		{"CI=0 asks", "0", []string{"delete"}, "", false, false},
		{"--non-interactive", "", []string{"delete", "--non-interactive"}, "with --non-interactive it cannot", false, false},
		{"--non-interactive before the command", "", []string{"--non-interactive", "delete"}, "with --non-interactive it cannot", false, false},
		{"--non-interactive=false with CI asks", "true", []string{"delete", "--non-interactive=false"}, "", false, false},
		{"-o json with --non-interactive=false refuses", "", []string{"delete", "-o", "json", "--non-interactive=false"}, "with --output json it cannot", false, false},
		{"-o json with CI=false refuses", "false", []string{"delete", "-o", "json"}, "with --output json it cannot", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CI", tc.ci)
			defer input.SetTerminals(!tc.noStdin, !tc.noStderr)()
			unread := stdinHolding(t, "y\n")
			answered := ""
			var r run
			asked := writtenToStderr(t, func() {
				r = execTree(context.Background(), promptTree(&answered), tc.args...)
			})

			if tc.refuse == "" {
				require.NoError(t, r.err)
				assert.Equal(t, "true", answered)
				assert.Contains(t, asked, "Delete prod? (y/n) ")
				return
			}
			require.Error(t, r.err)
			assert.True(t, input.IsRequired(r.err), "%v", r.err)
			assert.Equal(t, KindInputRequired, testKinds.Of(r.err))
			assert.Contains(t, r.err.Error(), tc.refuse)
			assert.Contains(t, r.err.Error(), "pass --force")
			assert.Empty(t, asked, "the question was written")
			assert.Empty(t, answered)
			assert.True(t, unread(), "the answer was read")
		})
	}
}

// A command line that resolves to no command is still guarded by the
// environment's part of the rule, so nothing that runs under it can ask in CI.
func TestTheGuardAppliesWhenNoCommandResolves(t *testing.T) {
	answered := ""
	root := promptTree(&answered)
	require.Nil(t, findTarget(root, []string{"nonesuch"}), "the line resolves to no command")

	t.Setenv("CI", "true")
	assert.Equal(t, "with CI set it cannot", refuseToAsk(root, []string{"nonesuch"})())
	t.Setenv("CI", "")
	assert.Empty(t, refuseToAsk(root, []string{"nonesuch"})())
}
