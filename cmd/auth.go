package cmd

import (
	"fmt"

	"github.com/astronomerio/astro-cli/auth"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/input"
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
	// Set Global Domain
	if !(len(projectRoot) > 0) && domainOverride != "" {
		prompt := fmt.Sprintf(messages.CONFIG_SET_GLOBAL_DOMAIN_PROMPT, domainOverride)
		setGlobal, err := input.InputConfirm(prompt)
		if err != nil {
			return err
		}
		if setGlobal {
			config.CFG.CloudDomain.SetHomeString(domainOverride)
		}
	}
	// Set Project Domain
	if len(projectRoot) > 0 && domainOverride != "" {
		config.CFG.CloudDomain.SetProjectString(domainOverride)
	}

	return auth.Login(oAuthOnly)
}

func authLogout(cmd *cobra.Command, args []string) {
	auth.Logout()
}
