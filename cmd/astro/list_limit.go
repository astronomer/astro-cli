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
func addListLimitFlag(cmd *cobra.Command, target *int, what string) {
	cmd.Flags().IntVar(target, "limit", 0, "Show at most this many "+what+" (default: all of them)")
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
