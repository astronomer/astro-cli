package cmd

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	workspaceId   string
	workspaceRole string
	role          string
)

func NewRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Inside rootCmd PersistentPreRun with args: %v\n", args)

			// TODO: API Request here
			if len(args) < 1 {
				color.Yellow("Not so fast!")
			}
		},
	}

	fmt.Printf("adding commands...")

	rootCmd.AddCommand(
		newAuthRootCmd(client, out),
		newWorkspaceCmd(client, out),
		newVersionCmd(out),
		newUpgradeCheckCmd(out),
		newUserCmd(client, out),
		newClusterRootCmd(client, out),
		newDevRootCmd(client, out),
		newCompletionCmd(client, out),
		newConfigRootCmd(client, out),
		newDeploymentRootCmd(client, out),
		newDeployCmd(client, out),
		newSaRootCmd(client, out),
		// TODO: remove newAirflowRootCmd, after 1.0 we have only devRootCmd
		newAirflowRootCmd(client, out),
		newLogsDeprecatedCmd(client, out),
	)
	return rootCmd
}
