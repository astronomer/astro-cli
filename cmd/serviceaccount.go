package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/houston"
	sa "github.com/astronomer/astro-cli/serviceaccount"
	"github.com/spf13/cobra"
)

func newSaDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [SA-ID]",
		Aliases: []string{"de"},
		Short:   "Delete a service-account in the astronomer platform",
		Long:    "Delete a service-account in the astronomer platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return saDelete(cmd, args, client, out)
		},
		Args: cobra.ExactArgs(1),
	}
	return cmd
}

func saDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Delete(args[0], client, out)
}
