package cliout

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// optsOutOfHelp is a usage error from a package that cannot import cliout: it
// marks itself with IsUsageError, and opts out of the pointer unless pointer.
type optsOutOfHelp struct{ pointer bool }

func (optsOutOfHelp) Error() string            { return "which of these ids" }
func (optsOutOfHelp) IsUsageError() bool       { return true }
func (e optsOutOfHelp) OmitsHelpPointer() bool { return !e.pointer }

// The root decides, once, for every command: no command sets SilenceUsage
// here, and cobra's usage block still never appears. A failure that is not a
// usage error is the error alone; a usage error, found before the command ran
// or returned from its run, is followed by the pointer to the command's help,
// unless the error opted out of it, and then only for one the command returned.
func TestTheRootAloneKeepsTheUsageBlockOutAndDecidesThePointer(t *testing.T) {
	const pointer = "Run astro thing list --help for usage.\n"
	for _, tc := range []struct {
		name string
		fail error
		args []string
		want string
	}{
		{"runtime failure", errBoom, nil, "Error: boom\n"},
		{"usage error from the run", Usage(errors.New("bad input")), nil, "Error: bad input\n" + pointer},
		{"usage error that opts out", UsageNoHelp(errors.New("use astro x instead")), nil, "Error: use astro x instead\n"},
		{"marker that does not opt out", optsOutOfHelp{pointer: true}, nil, "Error: which of these ids\n" + pointer},
		{"marker that opts out", optsOutOfHelp{}, nil, "Error: which of these ids\n"},
		{"opt-out wrapped further", fmt.Errorf("context: %w", UsageNoHelp(errors.New("use astro x instead"))), nil, "Error: context: use astro x instead\n"},
		{"usage error found before the run", nil, []string{"--bogus"}, "Error: unknown flag: --bogus\n" + pointer},
		{"a run's opt-out does not reach one found before it", UsageNoHelp(errors.New("unused")), []string{"--bogus"}, "Error: unknown flag: --bogus\n" + pointer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := testTree(tc.fail)
			walk(root, func(c *cobra.Command) { require.False(t, c.SilenceUsage, "nothing in this tree silences usage") })
			r := execTree(context.Background(), root, append([]string{"thing", "list"}, tc.args...)...)
			require.Error(t, r.err)
			assert.NotContains(t, r.stderr, "Usage:")
			assert.Equal(t, tc.want, r.stderr)
		})
	}
}
