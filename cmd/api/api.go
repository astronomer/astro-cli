// Package api provides the 'astro api' command for making authenticated API requests.
package api

import (
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// NewAPICmd creates the parent 'astro api' command.
func NewAPICmd() *cobra.Command {
	return NewAPICmdWithOutput(os.Stdout)
}

// NewAPICmdWithOutput creates the parent 'astro api' command with a custom output writer.
func NewAPICmdWithOutput(out io.Writer) *cobra.Command {
	var noColor bool

	cmd := &cobra.Command{
		Use:          "api",
		Short:        "Make authenticated API requests to Astronomer services",
		SilenceUsage: true,
		Long: `Make authenticated HTTP requests to Astronomer APIs and print responses.

The 'astro api' command provides direct access to Astronomer's REST APIs.

Available subcommands:
  airflow   Make requests to the Airflow REST API
  cloud     Make requests to the Astro Cloud API (api.astronomer.io)
  registry  Query the Airflow Provider Registry

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
			// Errors are not silenced anywhere in the family: a failure is
			// reported once, by cliout.Execute. A request whose error body was
			// already printed returns a SilentError, which Execute takes as
			// already presented and adds nothing to.
			cmd.SilenceUsage = true

			if noColor {
				color.NoColor = true
			}

			return nil
		},
		// No Run: `astro api` is a group, so cobra prints its help, with no
		// pre-run, when it is run bare.
	}

	cmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colorized output")

	cmd.AddCommand(NewAirflowCmd(out))
	cmd.AddCommand(NewCloudCmd(out))
	cmd.AddCommand(NewRegistryCmd(out))

	return cmd
}

// addOutputFlags gives ls or describe the CLI's shared -o text|json, written
// into format, and a tombstone for the --json it replaced. The requests
// themselves have no -o: they print the API's own response, and shape it with
// --jq and --template.
//
// Neither needs checking here. The -o flag refuses a format it does not offer
// while cobra parses flags, so format only ever holds text or json; the
// tombstone refuses --json in Args. Both fail before any pre-run refreshes a
// token or records telemetry, and before a spec is fetched.
func addOutputFlags(cmd *cobra.Command, format *cliout.Format) {
	cliout.AddOutputFlag(cmd, format)
	cliout.AddRemovedFlag(cmd, "json", "", true, cliout.ErrJSONFlagRemoved)
}
