package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	cloudAuth "github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/keychain"
	softwareAuth "github.com/astronomer/astro-cli/software/auth"
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

// newLoginCommand is a top-level alias for "astro auth login" kept for backward compatibility.
func newLoginCommand(store keychain.SecureStore, tokenHolder *httputil.TokenHolder, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) *cobra.Command {
	cmd := newAuthLoginCommand(store, tokenHolder, coreClient, platformCoreClient, out)
	cmd.Long = "Authenticate to Astro or Astro Private Cloud. This is an alias for 'astro auth login'."
	return cmd
}

// newLogoutCommand is a top-level alias for "astro auth logout" kept for backward compatibility.
func newLogoutCommand(store keychain.SecureStore, out io.Writer) *cobra.Command {
	cmd := newAuthLogoutCommand(store, out)
	cmd.Long = "Log out of Astronomer. This is an alias for 'astro auth logout'."
	return cmd
}

func login(cmd *cobra.Command, args []string, store keychain.SecureStore, tokenHolder *httputil.TokenHolder, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if len(args) == 1 {
		// check if user provided a valid cloud domain
		if !context.IsCloudDomain(args[0]) {
			// get the domain from context as an extra check
			ctx, _ := context.GetCurrentContext()
			if context.IsCloudDomain(ctx.Domain) {
				fmt.Fprintf(out, "To login to Astro Private Cloud follow the instructions below. If you are attempting to login in to Astro cancel the login and run 'astro login'.\n\n")
			}
			return softwareLogin(args[0], oAuth, "", "", houstonVersion, store, houstonClient, out)
		}
		return cloudLogin(args[0], token, store, tokenHolder, coreClient, platformCoreClient, out, shouldDisplayLoginLink)
	}
	// Log back into the current context in case no domain is passed
	ctx, err := context.GetCurrentContext()
	if err != nil || ctx.Domain == "" {
		// Default case when no domain is passed, and error getting current context
		return cloudLogin(domainutil.DefaultDomain, token, store, tokenHolder, coreClient, platformCoreClient, out, shouldDisplayLoginLink)
	} else if context.IsCloudDomain(ctx.Domain) {
		return cloudLogin(ctx.Domain, token, store, tokenHolder, coreClient, platformCoreClient, out, shouldDisplayLoginLink)
	}
	return softwareLogin(ctx.Domain, oAuth, "", "", houstonVersion, store, houstonClient, out)
}

func logout(cmd *cobra.Command, args []string, store keychain.SecureStore, out io.Writer) error {
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
		cloudLogout(domain, store, out)
	} else {
		softwareLogout(domain, store)
	}
	return nil
}

func newAuthRootCmd(store keychain.SecureStore, tokenHolder *httputil.TokenHolder, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication to Astronomer",
		Long:  "Commands for authenticating to Astro or Astro Private Cloud",
	}
	cmd.AddCommand(
		newAuthLoginCommand(store, tokenHolder, coreClient, platformCoreClient, out),
		newAuthLogoutCommand(store, out),
		newAuthTokenCommand(store, out),
	)
	return cmd
}

func newAuthLoginCommand(store keychain.SecureStore, tokenHolder *httputil.TokenHolder, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Log in to Astronomer",
		Long:  "Authenticate to Astro or Astro Private Cloud",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd, args, store, tokenHolder, coreClient, platformCoreClient, out)
		},
	}

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for cloud CLI login")
	cmd.Flags().StringVarP(&token, "token-login", "t", "", "Login with a token for browserless cloud CLI login")
	cmd.Flags().BoolVarP(&oAuth, "oauth", "o", false, "Do not prompt for local auth for software login")
	return cmd
}

func newAuthLogoutCommand(store keychain.SecureStore, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of Astronomer",
		Long:  "Log out of Astronomer",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logout(cmd, args, store, out)
		},
		Args: cobra.MaximumNArgs(1),
	}
	return cmd
}

func newAuthTokenCommand(store keychain.SecureStore, out io.Writer) *cobra.Command {
	var tokenDomain string
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token",
		Long:  "Print the current authentication token to standard output. This is useful for using the token in scripts or CI/CD pipelines.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printAuthToken(cmd, store, tokenDomain, out)
		},
	}
	cmd.Flags().StringVarP(&tokenDomain, "domain", "d", "", "Print the token for a specific context domain instead of the current context")
	return cmd
}

func printAuthToken(cmd *cobra.Command, store keychain.SecureStore, contextDomain string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var domain string
	if contextDomain != "" {
		domain = contextDomain
	} else {
		c, err := context.GetCurrentContext()
		if err != nil {
			return err
		}
		domain = c.Domain
	}

	if store == nil {
		return fmt.Errorf("no token found. Please run 'astro login' to authenticate")
	}
	creds, err := store.GetCredentials(domain)
	if err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return fmt.Errorf("no token found. Please run 'astro login' to authenticate")
		}
		return fmt.Errorf("reading credentials: %w", err)
	}
	if creds.Token == "" {
		return fmt.Errorf("no token found. Please run 'astro login' to authenticate")
	}

	rawToken := strings.TrimPrefix(creds.Token, "Bearer ")
	fmt.Fprintln(out, rawToken)
	return nil
}
