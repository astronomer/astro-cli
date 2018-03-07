package cmd

import (
	"github.com/astronomerio/astro-cli/auth"
	"github.com/astronomerio/astro-cli/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	registryOverride string

	authRootCmd = &cobra.Command{
		Use:   "auth",
		Short: "Mangage astronomer identity",
		Long:  "Manage astronomer identity",
	}

	authLoginCmd = &cobra.Command{
		Use:   "login",
		Short: "Login to Astronomer services",
		Long:  "Login to Astronomer services",
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
	authLoginCmd.Flags().StringVarP(&registryOverride, "registry", "r", "", "pass a custom project registry for authentication")
	// Auth logout
	authRootCmd.AddCommand(authLogoutCmd)
}

func authLogin(cmd *cobra.Command, args []string) error {
	if len(registryOverride) > 0 {
		config.CFG.RegistryAuthority.SetProjectString(registryOverride)
	}

	if config.CFG.RegistryAuthority.GetHomeString() == "" &&
		registryOverride == "" {
		return errors.New("No registry set. Use -r to pass a custom registry\n\nEx.\nastro auth login -r registry.EXAMPLE_DOMAIN.com\n ")
	}

	auth.Login()
	return nil
}

func authLogout(cmd *cobra.Command, args []string) {
	auth.Logout()
}
