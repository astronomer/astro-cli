package cmd

import (
	"io"

	"github.com/sjmiller609/astro-cli/auth"
	"github.com/sjmiller609/astro-cli/cluster"
	"github.com/sjmiller609/astro-cli/houston"
	"github.com/spf13/cobra"
)

var (
	oAuthOnly bool
	domain    string
)

func newAuthRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage astronomer identity",
		Long:  "Handles authentication to the Astronomer Platform",
	}
	cmd.AddCommand(
		newAuthLoginCmd(client, out),
		newAuthLogoutCmd(client, out),
	)
	return cmd
}

func newAuthLoginCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Login to Astronomer services",
		Long:  "Authenticate to houston-api using oAuth or basic auth.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return authLogin(cmd, args, client, out)
		},
	}
	cmd.Flags().BoolVarP(&oAuthOnly, "oauth", "o", false, "do not prompt for local auth")
	return cmd
}

func newAuthLogoutCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout of Astronomer services",
		Long:  "Logout of Astronomer services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return authLogout(cmd, args)
		},
		Args: cobra.MaximumNArgs(1),
	}
	return cmd
}

func authLogin(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	if len(args) == 1 {
		domain = args[0]
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	// by using "" we are delegating username/password to Login by asking input
	err := auth.Login(domain, oAuthOnly, "", "", client, out)
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
