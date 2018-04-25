package cmd

import (
	"fmt"

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

	}

	projectRegistry := config.CFG.RegistryAuthority.GetProjectString()
	projectCloudDomain := config.CFG.CloudDomain.GetProjectString()
	globalCloudDomain := config.CFG.CloudDomain.GetHomeString()
	globalRegistry := config.CFG.RegistryAuthority.GetHomeString()

	// checks for registry in all the expected places
	// prompts user for any implicit behavior
	switch {
	case registryOverride != "":
		config.CFG.RegistryAuthority.SetProjectString(registryOverride)
	case projectRegistry != "":
		// Don't prompt user, using project config is default expected behavior
		break
	case projectCloudDomain != "":
		config.CFG.RegistryAuthority.SetProjectString(fmt.Sprintf("registry.%s", projectCloudDomain))
		fmt.Printf("No registry set, using default: registry.%s\n", projectCloudDomain)
	case globalCloudDomain != "":
		config.CFG.RegistryAuthority.SetProjectString(fmt.Sprintf("registry.%s", globalCloudDomain))
		fmt.Printf("No registry set, using default: registry.%s\n", globalCloudDomain)
	case globalRegistry != "":
		// Don't prompt user, falling back to global config is default expected behavior
		break
	default:
		return errors.New("No registry set. Use -r to pass a custom registry\n\nEx.\nastro auth login -r registry.EXAMPLE_DOMAIN.com\n ")
	}

	auth.Login()
	return nil
}

func authLogout(cmd *cobra.Command, args []string) {
	auth.Logout()
}
