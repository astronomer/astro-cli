package astro

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// addListLimit makes cmd a list command with --limit: the most results wanted
// in all, with paging stopping once they are in hand. Left off, a list pages
// through everything. What is kept is the first rows in the order the command
// lists them, so a limited run returns the same ones every time.
//
// It sets cmd.RunE, which reads and validates --limit before anything else (a
// value below 1 is a usage error before the API is asked; one that is not an
// integer is refused by the flag parser) and then calls run with it, 0 when the
// flag is left off. The flag, its help and its validation live here and
// nowhere else, so a list command cannot register one and forget the other.
//
// what is the plural noun the help names. note, when not empty, is a sentence
// of its own the help adds: where the order is the API's own (it takes no
// sort), what the limit does not count, or that a filter runs locally.
func addListLimit(cmd *cobra.Command, what, note string, run func(cmd *cobra.Command, limit int) error) {
	var n int
	usage := "Show at most this many " + what + " (at least 1; default: all)"
	if note != "" {
		usage += ". " + note
	}
	cmd.Flags().IntVar(&n, "limit", 0, usage)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := checkListLimit(cmd, "limit", n); err != nil {
			return err
		}
		return run(cmd, n)
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
