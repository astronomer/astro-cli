package cmd

import (
	"fmt"
	"io"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	cloudAuth "github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
	softwareAuth "github.com/astronomer/astro-cli/software/auth"

	"github.com/spf13/cobra"
)

var (
	shouldDisplayLoginLink bool
	token                  string
	oAuth                  bool

	cloudLogin     = cloudAuth.Login
	cloudLogout    = cloudAuth.Logout
	softwareLogin  = softwareAuth.Login
	softwareLogout = softwareAuth.Logout
)

func newLoginCommand(astroClient astro.Client, coreClient astrocore.CoreClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Log in to Astronomer",
		Long:  "Authenticate to Astro or Astronomer Software",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd, args, astroClient, coreClient, out)
		},
	}

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for cloud CLI login")
	cmd.Flags().StringVarP(&token, "token-login", "t", "", "Login with a token for browserless cloud CLI login")
	cmd.Flags().BoolVarP(&oAuth, "oauth", "o", false, "Do not prompt for local auth for software login")
	return cmd
}

func newLogoutCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of Astronomer",
		Long:  "Log out of Astronomer",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logout(cmd, args, out)
		},
		Args: cobra.MaximumNArgs(1),
	}
	return cmd
}

func login(cmd *cobra.Command, args []string, astroClient astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if len(args) == 1 {
		// check if user provided a valid cloud domain
		if !context.IsCloudDomain(args[0]) {
			// get the domain from context as an extra check
			ctx, _ := context.GetCurrentContext()
			if context.IsCloudDomain(ctx.Domain) {
				// print an error if context domain is a valid cloud domain
				fmt.Fprintf(out, "Error: %s is an invalid domain to login into Astro.\n", args[0])
				// give the user an option to login to software
				y, _ := input.Confirm("Are you trying to authenticate to Astronomer Software or Nebula?")
				if !y {
					fmt.Println("Canceling login...")
					return nil
				}
			}
			return softwareLogin(args[0], oAuth, "", "", houstonVersion, houstonClient, out)
		}
		return cloudLogin(args[0], "", token, astroClient, coreClient, out, shouldDisplayLoginLink)
	}
	// Log back into the current context in case no domain is passed
	ctx, err := context.GetCurrentContext()
	if err != nil || ctx.Domain == "" {
		// Default case when no domain is passed, and error getting current context
		return cloudLogin(domainutil.DefaultDomain, "", token, astroClient, coreClient, out, shouldDisplayLoginLink)
	} else if context.IsCloudDomain(ctx.Domain) {
		return cloudLogin(ctx.Domain, "", token, astroClient, coreClient, out, shouldDisplayLoginLink)
	}
	return softwareLogin(ctx.Domain, oAuth, "", "", houstonVersion, houstonClient, out)
}

func logout(cmd *cobra.Command, args []string, out io.Writer) error {
	var domain string
	if len(args) == 1 {
		domain = args[0]
	} else {
		c, err := context.GetCurrentContext()
		if err != nil {
			return err
		}
		domain = c.Domain
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if context.IsCloudDomain(domain) {
		cloudLogout(domain, out)
	} else {
		softwareLogout(domain)
	}
	return nil
}

// This is to ensure we throw a meaningful error in case someone is using deprecated `astro auth login` or `astro auth logout` cmd
func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "auth",
		Deprecated: "use 'astro login' or 'astro logout' instead.\n\nWelcome to the Astro CLI v1.0.0, go to https://github.com/astronomer/astro-cli/blob/main/CHANGELOG.md#100---2022-05-23 to see a full list of breaking changes.\n",
		Hidden:     true,
	}
	return cmd
}
