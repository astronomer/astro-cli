package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/auth"
	"github.com/astronomer/astro-cli/cluster"
	"github.com/spf13/cobra"
)

var (
	oAuthOnly bool
	domain    string
)

func newAuthRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		Use:   "auth",
		Short: "Authenticate with an Astronomer Cluster",
		Long:  "Handles authentication to an Astronomer Cluster",
	}

	cmd.AddCommand(
		newAuthLoginCmd(out),
		newAuthLogoutCmd(),
	)
	return cmd
}

func newAuthLoginCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Login to Astronomer",
		Long:  "Authenticate to houston-api using oAuth or basic auth.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return authLogin(cmd, args, out)
		},
	}
	cmd.Flags().BoolVarP(&oAuthOnly, "oauth", "o", false, "do not prompt for local auth")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout of Astronomer",
		Long:  "Logout of Astronomer",
		RunE:  authLogout,
		Args:  cobra.MaximumNArgs(1),
	}
	return cmd
}

func authLogin(cmd *cobra.Command, args []string, out io.Writer) error {
	if len(args) == 1 {
		domain = args[0]
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	// by using "" we are delegating username/password to Login by asking input
	err := auth.Login(domain, oAuthOnly, "", "", houstonClient, out)
	if err != nil {
		return err
	}

	return nil
}

func authLogout(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		domain = args[0]
	} else {
		c, _ := cluster.GetCurrentCluster()
		domain = c.Domain
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	auth.Logout(domain)
	return nil
}
