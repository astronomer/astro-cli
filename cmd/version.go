package cmd

import (
	"io"

	"github.com/sjmiller609/astro-cli/version"
	"github.com/spf13/cobra"
)

func newVersionCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Astronomer CLI version",
		Long:  "The astro-cli semantic version and git commit tied to that release.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printVersion(cmd, out, args)
		},
	}
	return cmd
}

func newUpgradeCheckCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for newer version of Astronomer CLI",
		Long:  "Check for newer version of Astronomer CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return upgradeCheck(cmd, out, args)
		},
	}
	return cmd
}

func printVersion(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	err := version.PrintVersion(out)
	if err != nil {
		return err
	}
	return nil
}

func upgradeCheck(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	err := version.CheckForUpdate(out)
	if err != nil {
		return err
	}
	return nil
}
