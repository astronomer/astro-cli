package cmd

import (
	"io"

	astro "github.com/astronomer/astro-cli/astro-client"
	cloudAuth "github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/context"
	softwareAuth "github.com/astronomer/astro-cli/software/auth"

	"github.com/spf13/cobra"
)

var (
	shouldDisplayLoginLink bool
	shouldLoginWithToken   bool
	oAuth                  bool
	interactive            bool
	pageSize               int

	cloudLogin     = cloudAuth.Login
	cloudLogout    = cloudAuth.Logout
	softwareLogin  = softwareAuth.Login
	softwareLogout = softwareAuth.Logout
)

const defaultPageSize = 100

func newLoginCommand(astroClient astro.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Log in to Astronomer",
		Long:  "Authenticate to Astro or Astronomer Software",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd, args, astroClient, out)
		},
	}
	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for cloud CLI login")
	cmd.Flags().BoolVarP(&shouldLoginWithToken, "token-login", "t", false, "Login with a token for browserless cloud CLI login")
	cmd.Flags().BoolVarP(&oAuth, "oauth", "o", false, "Do not prompt for local auth for software login")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Prompt for interactive cli for software login")
	cmd.Flags().IntVarP(&pageSize, "page-size", "s", defaultPageSize, "Pagination page size for software workspace selection")
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

func login(cmd *cobra.Command, args []string, astroClient astro.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if !(pageSize > 0 && pageSize < defaultPageSize) {
		pageSize = defaultPageSize
	}

	if len(args) == 1 {
		if !context.IsCloudDomain(args[0]) {
			return softwareLogin(args[0], oAuth, "", "", interactive, pageSize, houstonClient, out)
		}
		return cloudLogin(args[0], astroClient, out, shouldDisplayLoginLink, shouldLoginWithToken)
	}
	// Log back into the current context in case no domain is passed
	ctx, err := context.GetCurrentContext()
	if err != nil || ctx.Domain == "" {
		// Default case when no domain is passed, and error getting current context
		return cloudLogin(cloudAuth.Domain, astroClient, out, shouldDisplayLoginLink, shouldLoginWithToken)
	} else if context.IsCloudDomain(ctx.Domain) {
		return cloudLogin(ctx.Domain, astroClient, out, shouldDisplayLoginLink, shouldLoginWithToken)
	}
	return softwareLogin(ctx.Domain, oAuth, "", "", interactive, pageSize, houstonClient, out)
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
