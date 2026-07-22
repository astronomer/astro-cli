package local

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/localstandalone/supervise"
)

// newSuperviseCmd builds the hidden `astro __supervise` subcommand: the
// supervisor process the standalone engine wraps every Airflow launch in
// (internal/localstandalone/supervise). Never typed by users — the engine
// spawns it with a machine-built command line, so flag parsing is left to
// the supervise package itself.
func newSuperviseCmd(_ Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:                supervise.Subcommand,
		Short:              "Run the local Airflow supervisor (internal)",
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(_ *cobra.Command, args []string) error {
			return supervise.Run(args)
		},
	}
	markSkipPreRun(cmd)
	return cmd
}
