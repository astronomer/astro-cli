package local

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/localdocker"
)

// newSessionWatchCmd builds the hidden `astro __local-session-watch`
// subcommand: the detached watcher a session-tied docker start spawns
// (internal/localdocker). It waits for the starting process to exit, then
// stops the project's compose stack. Never typed by users — the engine spawns
// it with a machine-built command line.
func newSessionWatchCmd(_ Deps) *cobra.Command {
	var (
		parentPID int
		project   string
	)
	cmd := &cobra.Command{
		Use:           localdocker.SessionWatchSubcommand,
		Short:         "Watch a session and stop its docker Airflow when it ends (internal)",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			eng := localdocker.New(routesDir())
			return eng.WatchAndStop(cmd.Context(), project, parentPID)
		},
	}
	cmd.Flags().IntVar(&parentPID, "parent-pid", 0, "PID to watch; the project is stopped when it exits")
	cmd.Flags().StringVar(&project, "project", "", "project path whose containers to stop")
	markSkipPreRun(cmd)
	return cmd
}
