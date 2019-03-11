package cmd

import (
	"github.com/astronomer/astro-cli/auth"
	"github.com/astronomer/astro-cli/cluster"
	"github.com/spf13/cobra"
)

var (
	oAuthOnly bool
	domain    string

	authRootCmd = &cobra.Command{
		Use:   "auth",
		Short: "Manage astronomer identity",
		Long:  "Handles authentication to the Astronomer Platform",
	}

	authLoginCmd = &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Login to Astronomer services",
		Long:  "Authenticate to houston-api using oAuth or basic auth.",
		RunE:  authLogin,
		Args:  cobra.MaximumNArgs(1),
	}

	authLogoutCmd = &cobra.Command{
		Use:   "logout",
		Short: "Logout of Astronomer services",
		Long:  "Logout of Astronomer services",
		Run:   authLogout,
		Args:  cobra.MaximumNArgs(1),
	}
)

func init() {
	// Auth root
	RootCmd.AddCommand(authRootCmd)

	// Auth login
	authRootCmd.AddCommand(authLoginCmd)
	authLoginCmd.Flags().BoolVarP(&oAuthOnly, "oauth", "o", false, "do not prompt for local auth")
	// Auth logout
	authRootCmd.AddCommand(authLogoutCmd)
}

func authLogin(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		domain = args[0]
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	err := auth.Login(domain, oAuthOnly)
	if err != nil {
		return err
	}

	return nil
}

func authLogout(cmd *cobra.Command, args []string) {
	if len(args) == 1 {
		domain = args[0]
	} else {
		c, _ := cluster.GetCurrentCluster()
		domain = c.Domain
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	auth.Logout(domain)
}
