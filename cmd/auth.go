package cmd

import (
	"github.com/astronomerio/astro-cli/auth"
	"github.com/spf13/cobra"
)

var (
	authRootCmd = &cobra.Command{
		Use:   "auth",
		Short: "Mangage astronomer identity",
		Long:  "Manage astronomer identity",
	}

	authLoginCmd = &cobra.Command{
		Use:   "login",
		Short: "Login to Astronomer services",
		Long:  "Login to Astronomer services",
		Run:   authLogin,
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

	// Auth logout
	authRootCmd.AddCommand(authLogoutCmd)
}

func authLogin(cmd *cobra.Command, args []string) {
	auth.Login()
}

func authLogout(cmd *cobra.Command, args []string) {
	auth.Logout()
}
