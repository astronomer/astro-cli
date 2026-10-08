package cliout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

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
