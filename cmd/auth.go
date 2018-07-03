package cmd

import (
	"github.com/astronomerio/astro-cli/auth"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	domainOverride string

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
	// Auth logout
	authRootCmd.AddCommand(authLogoutCmd)
}

func authLogin(cmd *cobra.Command, args []string) error {
	if domainOverride != "" {
		config.CFG.CloudDomain.SetProjectString(domainOverride)
	}

	projectCloudDomain := config.CFG.CloudDomain.GetProjectString()
	globalCloudDomain := config.CFG.CloudDomain.GetHomeString()

	if len(projectCloudDomain) == 0 && len(globalCloudDomain) == 0 {
		return errors.New(messages.CONFIG_DOMAIN_NOT_SET_ERROR)
	}

	auth.Login()
	return nil
}

func authLogout(cmd *cobra.Command, args []string) {
	auth.Logout()
}
