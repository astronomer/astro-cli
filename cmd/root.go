package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/version"
)

var (
	workspaceID    string
	workspaceRole  string
	deploymentRole string
	role           string
	skipVerCheck   bool
)

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return version.ValidateCompatibility(client, out, version.CurrVersion, skipVerCheck)
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&skipVerCheck, "skip-version-check", "", false, "skip version compatibility check")
	rootCmd.AddCommand(
		newAuthRootCmd(client, out),
		newWorkspaceCmd(client, out),
		newVersionCmd(client, out),
		newUpgradeCheckCmd(client, out),
		newUserCmd(client, out),
		newClusterRootCmd(client, out),
		newDevRootCmd(client, out),
		newCompletionCmd(client, out),
		newConfigRootCmd(client, out),
		newDeploymentRootCmd(client, out),
		newDeployCmd(),
		newSaRootCmd(client, out),
		// TODO: remove newAirflowRootCmd, after 1.0 we have only devRootCmd
		newAirflowRootCmd(client, out),
		newLogsDeprecatedCmd(),
	)
	return rootCmd
}
