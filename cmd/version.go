package cmd

import (
	"github.com/astronomerio/astro-cli/version"
	"github.com/spf13/cobra"
)

var (
	currVersion string
	currCommit  string
	versionCmd  = &cobra.Command{
		Use:   "version",
		Short: "Astronomer CLI version",
		Long:  "The astro-cli semantic version and git commit tied to that release.",
		RunE:  printVersion,
	}

	upgradeCmd = &cobra.Command{
		Use:   "upgrade",
		Short: "Check for newer version of Astronomer CLI",
		Long:  "Check for newer version of Astronomer CLI",
		RunE:  upgradeCheck,
	}
)

func init() {
	RootCmd.AddCommand(versionCmd)
	RootCmd.AddCommand(upgradeCmd)
}

func printVersion(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	err := version.PrintVersion(currVersion, currCommit)
	if err != nil {
		return err
	}
	return nil
}

func upgradeCheck(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	
	err := version.CheckForUpdate(currVersion, currCommit)
	if err != nil {
		return err
	}
	return nil
}
