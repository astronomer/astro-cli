package astro

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// addListLimitFlag registers --limit on a list command: the most results
// wanted in all, with paging stopping once they are in hand. Left off, a list
// pages through everything. What is kept is the first rows in the order the
// command lists them, so a limited run returns the same ones every time.
//
// what is the plural noun the help names, and may carry a note on the order
// ("bundles, in the order the API returns them") where the API takes no sort.
func addListLimitFlag(cmd *cobra.Command, target *int, what string) {
	cmd.Flags().IntVar(target, "limit", 0, "Show at most this many "+what+"; at least 1 (default: all of them)")
}

// addListLimit is addListLimitFlag for a command that keeps the value to
// itself: it registers --limit and returns the reader the command's RunE calls
// first, which is 0 when the flag is left off and a usage error when it is
// below 1.
func addListLimit(cmd *cobra.Command, what string) func() (int, error) {
	var n int
	addListLimitFlag(cmd, &n, what)
	return func() (int, error) {
		if err := checkListLimit(cmd, "limit", n); err != nil {
			return 0, err
		}
		return n, nil
	}
}

// checkListLimit refuses a --limit that is not a whole number of one or more
// (a value that is not an integer is refused by the flag parser itself).
// Zero is not "everything": it is far likelier a mistake than a request, so
// leaving the flag off is the way to ask for all.
func checkListLimit(cmd *cobra.Command, flag string, n int) error {
	if cmd.Flags().Changed(flag) && n < 1 {
		return cliout.Usage(fmt.Errorf("--%s must be a whole number of 1 or more; got %d", flag, n))
	}
	return nil
}
