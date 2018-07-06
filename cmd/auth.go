package cmd

import (
	"github.com/astronomerio/astro-cli/auth"
	"github.com/astronomerio/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	domainOverride string
	oAuthOnly      bool

	authRootCmd = &cobra.Command{
		Use:   "auth",
		Short: "Mangage astronomer identity",
		Long:  "Handles authentication to the Astronomer Platform",
	}

	authLoginCmd = &cobra.Command{
		Use:   "login",
		Short: "Login to Astronomer services",
		Long:  "Authenticate to houston-api using oAuth or basic auth.",
		RunE:  authLogin,
	}

	authLogoutCmd = &cobra.Command{
		Use:   "logout",
		Short: "Logout of Astronomer services",
		Long:  "Logout of Astronomer services",
		Run:   authLogout,
	}
)

func init() {
	// Auth root
	RootCmd.AddCommand(authRootCmd)

	// Auth login
	authRootCmd.AddCommand(authLoginCmd)
	authLoginCmd.Flags().StringVarP(&domainOverride, "domain", "d", "", "pass the cluster domain for authentication")
	authLoginCmd.Flags().BoolVarP(&oAuthOnly, "oauth", "o", false, "do not prompt for local auth")
	// Auth logout
	authRootCmd.AddCommand(authLogoutCmd)
}

func authLogin(cmd *cobra.Command, args []string) error {
	if domainOverride != "" {
		config.CFG.CloudDomain.SetProjectString(domainOverride)
	}

	return auth.Login(oAuthOnly)
}

func authLogout(cmd *cobra.Command, args []string) {
	auth.Logout()
}
