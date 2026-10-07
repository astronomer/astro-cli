package cmd

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/version"
)

// versionOutput is what `astro version -o json` prints. It is a contract, so
// it starts minimal: a field is easy to add and impossible to take back.
type versionOutput struct {
	Version string `json:"version"`
}

func newVersionCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "version",
		Short: "List running version of the Astro CLI",
		Long:  `The astro semantic version.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			format, err := cliout.ParseFormat(output)
			if err != nil {
				return err
			}
			return cliout.Renderer{Format: format, Out: cmd.OutOrStdout()}.Emit(
				versionOutput{Version: version.Current()},
				version.PrintVersion,
			)
		},
		Example: `  # Print the CLI's version
  astro version`,
	}
	cliout.AddOutputFlag(cmd, &output)
	return cmd
}
