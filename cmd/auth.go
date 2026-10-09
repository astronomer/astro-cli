package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	apcAuth "github.com/astronomer/astro-cli/internal/platform/apc/auth"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	shouldDisplayLoginLink bool
	token                  string
	oAuth                  bool
	signup                 bool
	signin                 bool
	forceLogin             bool
	loginInVault           bool

	cloudLogin  = astroAuth.Login
	cloudLogout = astroAuth.Logout
	apcLogin    = apcAuth.Login
	apcLogout   = apcAuth.Logout
)

// newLoginCommand is a top-level alias for "astro auth login" kept for backward compatibility.
func newLoginCommand(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := newAuthLoginCommand(astroV1Client, out)
	cmd.Long += " This is an alias for 'astro auth login'."
	cmd.Example = `  # Log in to Astro
  astro login`
	return cmd
}

// newLogoutCommand is a top-level alias for "astro auth logout" kept for backward compatibility.
func newLogoutCommand(out io.Writer) *cobra.Command {
	cmd := newAuthLogoutCommand(out)
	cmd.Long = "Log out of Astronomer. This is an alias for 'astro auth logout'."
	cmd.Example = `  # Log out of the current context
  astro logout`
	return cmd
}

// signupVerificationMsg tells the user what an unverified address means. The
// sign-up worked, so this is not a failure and the command exits zero: a caller
// that reads a non-zero exit as "try again" would sign up twice.
const signupVerificationMsg = `Thanks for signing up. Check your inbox for a verification email.
After you verify your email address, run 'astro login' to finish signing in.`

func login(cmd *cobra.Command, args []string, astroV1Client astrov1.APIClient, out io.Writer) error {
	err := runLogin(cmd, args, astroV1Client, out)
	if errors.Is(err, astroAuth.ErrEmailVerificationPending) {
		fmt.Fprintf(out, "\n%s\n", signupVerificationMsg)
		return nil
	}
	return err
}

// signupForDomain picks the browser screen for a login to domain. A flag the
// user passed wins over the detection.
func signupForDomain(domain string) bool {
	switch {
	case signup:
		return true
	case signin, token != "": // a token login opens no browser, so it keeps its old behavior
		return false
	default:
		return astroAuth.ShouldSignup(domain)
	}
}

// resumeLoginVault lets the login being logged in to back into the local
// secrets vault when --vault asks for it: the domain named, or the current
// context's.
func resumeLoginVault(args []string) error {
	if !loginInVault {
		return nil
	}
	domain := domainutil.DefaultDomain
	if len(args) == 1 {
		domain = domainutil.ExpandShortName(args[0])
		if context.IsCloudDomain(domain) {
			// The context the login is saved under, as cloudLogin names it.
			domain = domainutil.FormatDomain(domain)
		}
	} else if ctx, err := context.GetCurrentContext(); err == nil && ctx.Domain != "" {
		domain = ctx.Domain
	}
	c := config.Context{Domain: domain}
	return c.ResumeLoginVault()
}

// apcPlatformVersion is the version of the APC platform at domain, as far as
// the CLI can ask. A login to APC reads it to decide whether to page the
// workspace list, and it is asked for here rather than when the root is built
// (cmd/root.go), so no other command waits on Houston for it.
//
// The Houston client talks to the current context, so only a login to the
// current context's own domain has a platform to ask. A login to another
// domain gets "", which reads as the newest version, rather than the version
// of a host it is not logging in to, and does not wait on that host to get it.
func apcPlatformVersion(domain string) string {
	if houstonVersion != "" {
		return houstonVersion
	}
	ctx, err := context.GetCurrentContext()
	if err != nil || ctx.Domain != domain || context.IsCloudDomain(domain) {
		return ""
	}
	houstonVersion, err = houstonClient.GetPlatformVersion(nil)
	if err != nil {
		logger.Debugf("Unable to get Houston version: %s", err)
	}
	return houstonVersion
}

func runLogin(cmd *cobra.Command, args []string, astroV1Client astrov1.APIClient, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if err := resumeLoginVault(args); err != nil {
		return err
	}
	if len(args) == 1 {
		domain := domainutil.ExpandShortName(args[0])
		// check if user provided a valid cloud domain
		if !context.IsCloudDomain(domain) {
			// get the domain from context as an extra check
			ctx, _ := context.GetCurrentContext() //nolint:errcheck // falls back to the zero context in this shell code
			if context.IsCloudDomain(ctx.Domain) {
				fmt.Fprintf(out, "To login to APC follow the instructions below. If you are attempting to login in to Astro cancel the login and run 'astro login'.\n\n")
			}
			return apcLogin(domain, oAuth, "", "", apcPlatformVersion(domain), houstonClient, out)
		}
		return cloudLogin(domain, token, astroV1Client, out, shouldDisplayLoginLink, signupForDomain(domain), forceLogin)
	}
	// Log back into the current context in case no domain is passed
	ctx, err := context.GetCurrentContext()
	if err != nil || ctx.Domain == "" {
		// Default case when no domain is passed, and error getting current context
		return cloudLogin(domainutil.DefaultDomain, token, astroV1Client, out, shouldDisplayLoginLink, signupForDomain(domainutil.DefaultDomain), forceLogin)
	} else if context.IsCloudDomain(ctx.Domain) {
		return cloudLogin(ctx.Domain, token, astroV1Client, out, shouldDisplayLoginLink, signupForDomain(ctx.Domain), forceLogin)
	}
	return apcLogin(ctx.Domain, oAuth, "", "", apcPlatformVersion(ctx.Domain), houstonClient, out)
}

func logout(cmd *cobra.Command, args []string, out io.Writer) error {
	var domain string
	if len(args) == 1 {
		domain = domainutil.ExpandShortName(args[0])
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
		apcLogout(domain)
	}
	return nil
}

func newAuthRootCmd(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication to Astronomer",
		Long:  "Commands for authenticating to Astro or APC",
	}
	cmd.AddCommand(
		newAuthLoginCommand(astroV1Client, out),
		newAuthLogoutCommand(out),
		newAuthTokenCommand(out),
	)
	return cmd
}

func newAuthLoginCommand(astroV1Client astrov1.APIClient, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [BASEDOMAIN]",
		Short: "Log in to Astronomer",
		Long:  "Authenticate to Astro or APC. A saved login for the domain that still works is reused without a browser; use --force to log in again. Without a saved account for astronomer.io, the browser opens the sign-up screen; use --signin for an existing account.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd, args, astroV1Client, out)
		},
		Example: `  # Log in to Astro in the browser
  astro auth login

  # Log in to an APC installation
  astro auth login <BASEDOMAIN>

  # Log in without a browser, with an API token
  astro auth login --token-login <TOKEN>`,
	}

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for cloud CLI login")
	cmd.Flags().StringVarP(&token, "token-login", "t", "", "Login with a token for browserless cloud CLI login")
	cmd.Flags().BoolVarP(&oAuth, "oauth", "o", false, "Do not prompt for local auth for APC login")
	cmd.Flags().BoolVar(&signup, "signup", false, "Create a new Astro account instead of signing in to an existing one")
	cmd.Flags().BoolVar(&signin, "signin", false, "Sign in to an existing Astro account instead of creating one")
	cmd.Flags().BoolVar(&forceLogin, "force", false, "Log in through the browser even when a saved login still works, and ask for the password again")
	cmd.Flags().BoolVar(&loginInVault, "vault", false, "Keep this login in the local secrets vault even after an older Astro CLI or Astro Desktop was seen using it. That older tool will ask you to log in again")
	cmd.MarkFlagsMutuallyExclusive("signup", "signin")
	return cmd
}

func newAuthLogoutCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of Astronomer",
		Long:  "Log out of Astronomer",
		RunE: func(cmd *cobra.Command, args []string) error {
			return logout(cmd, args, out)
		},
		Args: cobra.MaximumNArgs(1),
		Example: `  # Log out of the current context
  astro auth logout

  # Log out of another context
  astro auth logout <DOMAIN>`,
	}
	return cmd
}

func newAuthTokenCommand(out io.Writer) *cobra.Command {
	var (
		tokenDomain string
		forceRenew  bool
		output      cliout.Format
	)
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token",
		Long:  "Print the current authentication token to standard output. This is useful for using the token in scripts or CI/CD pipelines.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printAuthToken(cmd, tokenDomain, forceRenew, cliout.Renderer{Format: output, Out: out})
		},
		Example: `  # Print the current context's token
  astro auth token

  # Print the token for another context
  astro auth token --domain <DOMAIN>

  # The token with its domain and expiry, for a script
  astro auth token -o json`,
	}
	cmd.Flags().StringVarP(&tokenDomain, "domain", "d", "", "Print the token for a specific context domain instead of the current context")
	cmd.Flags().BoolVar(&forceRenew, "force", false, "Renew the token from the saved login even if it has not expired yet")
	cliout.AddOutputFlag(cmd, &output)
	return cmd
}

// errNoAuthToken is `astro auth token` finding a context with no login in it.
var errNoAuthToken = errors.New("no token found. Please run 'astro login' to authenticate")

// authToken is what `astro auth token -o json` publishes: the token, without
// its "Bearer " prefix, the domain it is for, and when it expires, which is
// absent when that is not known (tokenExpiry).
type authToken struct {
	Token     string     `json:"token"`
	Domain    string     `json:"domain"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// printAuthToken publishes the login's token. In text it is the token and
// nothing else, so `$(astro auth token)` is the token.
func printAuthToken(cmd *cobra.Command, contextDomain string, force bool, r cliout.Renderer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	c, err := astroCmd.FreshLogin(contextDomain, force)
	if err != nil {
		return err
	}

	if c.Token == "" {
		return errNoAuthToken
	}

	tok := authToken{Token: strings.TrimPrefix(c.Token, "Bearer "), Domain: c.Domain}
	tok.ExpiresAt = tokenExpiry(&c, tok.Token)
	return r.Emit(&tok, cliout.Text(func(b *bufio.Writer) { fmt.Fprintln(b, tok.Token) }))
}

// tokenExpiry is when token expires: its own exp claim when it states one,
// and otherwise the expiry its login recorded, which is the identity
// provider's word for a browser login whose token is no JWT this reads. An
// ASTRO_API_TOKEN with no exp claim has none: the expiry recorded for it is a
// year from now, made up so the login reads as live, and is not published.
func tokenExpiry(c *config.Context, token string) *time.Time {
	if claims, err := util.ParseAPIToken(token); err == nil && claims.ExpiresAt != nil {
		expiry := claims.ExpiresAt.UTC()
		return &expiry
	}
	// The environment's token, read by the one rule every reader of the
	// variable uses (any-case scheme, repeated, any whitespace).
	if env, ok := astrosession.APIToken(); ok && astrosession.Credential(env) == astrosession.Credential(token) {
		return nil
	}
	if expiry, err := c.GetExpiresIn(); err == nil && !expiry.IsZero() {
		expiry = expiry.UTC()
		return &expiry
	}
	return nil
}
