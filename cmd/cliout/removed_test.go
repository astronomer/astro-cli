package cliout

import (
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tombstoned flag is a usage error however the command is run, not only
// under Execute: a caller running the command with cobra directly gets exit 2
// from ExitCode, and RunE never runs.
func TestARemovedFlagIsAUsageErrorWithoutExecute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		isBool bool
		args   []string
	}{
		{"bool", true, []string{"--gone"}},
		{"string, consuming its value", false, []string{"--gone", "value"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ran := false
			cmd := &cobra.Command{
				Use:  "leaf",
				Args: cobra.NoArgs,
				RunE: func(*cobra.Command, []string) error { ran = true; return nil },
			}
			AddRemovedFlag(cmd, "gone", "", tc.isBool, "--gone was removed: use --new")
			cmd.SetArgs(tc.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.EqualError(t, err, "--gone was removed: use --new")
			assert.True(t, IsUsage(err))
			assert.Equal(t, ExitUsage, ExitCode(context.Background(), err))
			assert.False(t, ran)
			assert.True(t, cmd.Flag("gone").Hidden)
		})
	}
}

// The command's own Args still apply when the removed flag is absent.
func TestARemovedFlagKeepsTheCommandsArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "leaf", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return nil }}
	AddRemovedFlag(cmd, "gone", "", true, "removed")
	cmd.SetArgs([]string{"extra"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.ErrorContains(t, cmd.Execute(), "unknown command")
}
