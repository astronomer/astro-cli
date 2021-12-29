package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/github"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/version"

	"github.com/spf13/cobra"
)

func newVersionCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "astro CLI version",
		Long:  "The astro-cli semantic version and git commit tied to that release.",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return printVersion(client, cmd, out, args)
		},
	}
	return cmd
}

func newUpgradeCheckCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for newer version of Astronomer CLI",
		Long:  "Check for newer version of Astronomer CLI",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return upgradeCheck(client, cmd, out, args)
		},
	}
	return cmd
}

func printVersion(client *houston.Client, cmd *cobra.Command, out io.Writer, _ []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	err := version.PrintVersion(client, out)
	if err != nil {
		return err
	}
	return nil
}

func upgradeCheck(client *houston.Client, cmd *cobra.Command, out io.Writer, _ []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	ghc := github.NewGithubClient(httputil.NewHTTPClient())

	err := version.CheckForUpdate(client, ghc, out)
	if err != nil {
		return err
	}
	return nil
}
