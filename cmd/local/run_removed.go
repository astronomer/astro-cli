package local

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// replaceRunDag is what replaces 1.x `astro run <dag-id>`: Airflow's own
// `dags test`, run in the project's Airflow environment. With Airflow stopped
// a standalone project runs it in its venv, so like `astro run` it needs no
// running Airflow.
const replaceRunDag = "astro local run airflow dags test"

// dagIDRe is the shape of a Dag id safe to repeat in a command line.
var dagIDRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// newRunRemovedCmd builds the `astro run` removal stub. The shape follows the
// `astro dev` stub: hidden, any arguments, flag parsing off so an old
// invocation's --dag-file or --execution-date reaches the guidance instead of
// dying on the flag.
//
// The failure is a usage error, the kind cobra's own unknown command is: it
// exits 2, as `astro run` did before this stub existed, and under --output
// json it is the one error object every command publishes.
func newRunRemovedCmd() *cobra.Command {
	var output cliout.Format
	cmd := &cobra.Command{
		Use:                nameRun,
		Short:              "Removed in v2 — use `" + replaceRunDag + "`",
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			return cliout.Usage(errors.New(runRemovedGuidance(args)))
		},
	}
	cliout.AddOutputFlag(cmd, &output)
	markSkipPreRun(cmd)
	return cmd
}

// runRemovedGuidance names the replacement, carrying the Dag id typed when it
// is one. Only the id is repeated: the other flags take paths and dates the
// replacement spells differently.
func runRemovedGuidance(args []string) string {
	dagID := "<dag-id>"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && dagIDRe.MatchString(args[0]) {
		dagID = args[0]
	}
	return fmt.Sprintf("`astro run` was removed in Astro CLI v2. Use `%s %s` instead.\n"+
		"It runs the Dag in this project's Airflow environment and needs no running Airflow in a standalone project. "+
		"Pass a logical date as the argument after the Dag id, in place of --execution-date.", replaceRunDag, dagID)
}
