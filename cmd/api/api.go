// Package api provides the 'astro api' command for making authenticated API requests.
package api

import (
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// NewAPICmd creates the parent 'astro api' command.
func NewAPICmd() *cobra.Command {
	return NewAPICmdWithOutput(os.Stdout)
}

// NewAPICmdWithOutput creates the parent 'astro api' command with a custom output writer.
func NewAPICmdWithOutput(out io.Writer) *cobra.Command {
	var noColor bool

	cmd := &cobra.Command{
		Use:           "api",
		Short:         "Make authenticated API requests to Astronomer services",
		SilenceErrors: true, // API commands print error bodies themselves; don't let cobra double-print
		SilenceUsage:  true,
		Long: `Make authenticated HTTP requests to Astronomer APIs and print responses.

The 'astro api' command provides direct access to Astronomer's REST APIs.

Available subcommands:
  airflow  Make requests to the Airflow REST API
  cloud    Make requests to the Astro Cloud API (api.astronomer.io)

Use "astro api [command] --help" for more information about a command.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Cobra does not chain PersistentPreRun hooks: defining one
			// on a child command silently replaces the parent's. Walk up
			// to the root and call its PersistentPreRunE explicitly so
			// that token refresh, logging setup (--verbosity), and
			// version checking all still happen for api subcommands.
			if root := cmd.Root(); root != nil && root.PersistentPreRunE != nil {
				if err := root.PersistentPreRunE(cmd, args); err != nil {
					return err
				}
			}

			// Cobra does not inherit SilenceUsage to subcommands, so propagate
			// it here. The cmd parameter is the actual subcommand being executed,
			// not the parent where PersistentPreRunE is defined.
			//
			// Note: we do NOT propagate SilenceErrors. The parent api command
			// sets SilenceErrors to avoid double-printing HTTP error bodies
			// (SilentError), but subcommands need cobra to print non-silent
			// errors like connection failures.
			cmd.SilenceUsage = true

			if noColor {
				color.NoColor = true
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colorized output")

	cmd.AddCommand(NewAirflowCmd(out))
	cmd.AddCommand(NewCloudCmd(out))

	return cmd
}
