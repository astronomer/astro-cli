package cmd

import (
	"github.com/astronomerio/astro-cli/auth"
	"github.com/astronomerio/astro-cli/cluster"
	"github.com/spf13/cobra"
)

var (
	oAuthOnly bool

	authRootCmd = &cobra.Command{
		Use:   "auth",
		Short: "Mangage astronomer identity",
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
	var domain string

	if len(args) == 1 {
		domain = args[0]
	}

	err := auth.Login(domain, oAuthOnly)
	if err != nil {
		return err
	}

	return nil
}

func authLogout(cmd *cobra.Command, args []string) {
	var domain string

	if len(args) == 1 {
		domain = args[0]
	} else {
		c, _ := cluster.GetCurrentCluster()
		domain = c.Domain
	}

	auth.Logout(domain)
}
