package cmd

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/version"
)

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "List running version of the Astro CLI",
		Long:  `The astro semantic version.`,
		Run:   printVersion,
	}

	return cmd
}

func printVersion(cmd *cobra.Command, args []string) {
	version.PrintVersion()
}
