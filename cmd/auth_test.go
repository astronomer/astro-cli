package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/secrets"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestAuthRootCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("login", "--help")
	s.NoError(err)
	s.Contains(output, "Authenticate to Astro or APC")
	s.Contains(output, "--signup")
	s.Contains(output, "--signin")
	s.Contains(output, "--force")
}

func (s *CmdSuite) TestAuthTokenHasForce() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("auth", "token", "--help")
	s.NoError(err)
	s.Contains(output, "--force")
}

func (s *CmdSuite) TestLogin() {
	buf := new(bytes.Buffer)
	cloudDomain := "astronomer.io"
	apcDomain := "astronomer_dev.com"

	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signup, force bool) error {
		s.Equal(cloudDomain, domain)
		return nil
	}

	apcLogin = func(domain string, oAuthOnly bool, username, password, houstonVersion string, client houston.ClientInterface, out io.Writer) error {
		s.Equal(apcDomain, domain)
		return nil
	}

	// cloud login success
	login(&cobra.Command{}, []string{cloudDomain}, nil, buf)

	// software login success
	testUtil.InitTestConfig(testUtil.Initial)
	login(&cobra.Command{}, []string{apcDomain}, nil, buf)

	// no domain, cloud login
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	login(&cobra.Command{}, []string{}, nil, buf)

	// no domain, software login
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	login(&cobra.Command{}, []string{}, nil, buf)

	// no domain, no current context set
	config.ResetCurrentContext()
	login(&cobra.Command{}, []string{}, nil, buf)

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	apcDomain = "software.astronomer.io"
	login(&cobra.Command{}, []string{apcDomain}, nil, buf)
	s.Contains(buf.String(), "To login to APC follow the instructions below. If you are attempting to login in to Astro cancel the login and run 'astro login'.\n\n")
}

func (s *CmdSuite) TestLoginSignup() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.T().Cleanup(func() { signup, signin, token = false, false, "" })

	var got bool
	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signupFlag, force bool) error {
		got = signupFlag
		return nil
	}
	buf := new(bytes.Buffer)

	// The config holds a context for astronomer.io and none for astronomer-dev.io.
	const knownDomain, newDomain = "astronomer.io", "astronomer-dev.io"

	signup, signin = true, false
	s.NoError(login(&cobra.Command{}, []string{knownDomain}, nil, buf))
	s.True(got)

	signup, signin = false, true
	s.NoError(login(&cobra.Command{}, []string{newDomain}, nil, buf))
	s.False(got)

	signup, signin = false, false
	s.NoError(login(&cobra.Command{}, []string{knownDomain}, nil, buf))
	s.False(got)

	// Only production takes new accounts, so another environment signs in even
	// with nothing saved for it.
	s.NoError(login(&cobra.Command{}, []string{newDomain}, nil, buf))
	s.False(got)
}

func (s *CmdSuite) TestLoginSignupNewProductionAccount() {
	// The config holds a context for astronomer-dev.io and none for astronomer.io.
	testUtil.InitTestConfig(testUtil.CloudDevPlatform)
	s.T().Cleanup(func() { token = "" })

	var got bool
	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signupFlag, force bool) error {
		got = signupFlag
		return nil
	}
	buf := new(bytes.Buffer)

	s.NoError(login(&cobra.Command{}, []string{"astronomer.io"}, nil, buf))
	s.True(got)

	// A token login opens no browser, so it stays on the sign-in path.
	token = "a-token"
	s.NoError(login(&cobra.Command{}, []string{"astronomer.io"}, nil, buf))
	s.False(got)
}

func (s *CmdSuite) TestLoginShortNames() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	var got string
	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signup, force bool) error {
		got = domain
		return nil
	}
	apcLogin = func(domain string, oAuthOnly bool, username, password, houstonVersion string, client houston.ClientInterface, out io.Writer) error {
		s.Fail("a short name went to APC login", domain)
		return nil
	}
	tests := map[string]string{
		"prod":                      "astronomer.io",
		"stage":                     "astronomer-stage.io",
		"dev":                       "astronomer-dev.io",
		"pr12345":                   "pr12345.astronomer-dev.io",
		"pr12345.astronomer-dev.io": "pr12345.astronomer-dev.io",
	}
	for name, want := range tests {
		s.NoError(login(&cobra.Command{}, []string{name}, nil, new(bytes.Buffer)))
		s.Equal(want, got, name)
	}
}

func (s *CmdSuite) TestLoginForce() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	var got bool
	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signup, force bool) error {
		got = force
		return nil
	}

	s.T().Cleanup(func() { forceLogin = false })
	buf := new(bytes.Buffer)

	s.NoError(login(&cobra.Command{}, []string{"astronomer.io"}, nil, buf))
	s.False(got)

	forceLogin = true
	s.NoError(login(&cobra.Command{}, []string{"astronomer.io"}, nil, buf))
	s.True(got)
	s.NoError(login(&cobra.Command{}, []string{}, nil, buf))
	s.True(got)
}

// --vault lets a login kept in the config for an older CLI back into the
// local secrets vault; without it the login stays in the config.
func (s *CmdSuite) TestLoginVault() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	keyring.MockInit()
	l, err := secrets.NewLogins(filepath.Join(s.T().TempDir(), "secrets"))
	s.Require().NoError(err)
	s.T().Cleanup(config.UseLoginsForTesting(l))
	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signup, force bool) error {
		return nil
	}
	s.T().Cleanup(func() { loginInVault = false })
	buf := new(bytes.Buffer)

	// A config file of the test's own on disk, whose time the older CLI's
	// write below sets.
	prevFile := config.HomeConfigFile
	s.T().Cleanup(func() { config.HomeConfigFile = prevFile })
	config.HomeConfigFile = filepath.Join(s.T().TempDir(), "config.yaml")
	s.Require().NoError(os.WriteFile(config.HomeConfigFile, nil, 0o600))
	cfg, key := config.HomeConfigFile, "astronomer_io"
	older := secrets.Login{Token: "Bearer older-access", RefreshToken: "older-refresh"}
	l.Save(cfg, key, secrets.Login{Token: "Bearer first-access", RefreshToken: "first-refresh"})
	// An older CLI logged in over the vault's login, writing the config after
	// the vault entry.
	later := time.Now().Add(time.Minute)
	s.Require().NoError(os.Chtimes(cfg, later, later))
	l.Resolve(cfg, key, older)
	inVault := secrets.Login{Token: secrets.LoginInVault}

	s.NoError(login(&cobra.Command{}, []string{"astronomer.io"}, nil, buf))
	s.True(l.Save(cfg, key, older) == older, "without --vault the login left the config")

	loginInVault = true
	// Spelled the way the login page's address is, which the login saves
	// under astronomer.io.
	s.NoError(login(&cobra.Command{}, []string{"cloud.astronomer.io"}, nil, buf))
	s.True(l.Save(cfg, key, older) == inVault, "--vault did not let the login back into the vault")
}

func (s *CmdSuite) TestLoginSignupAndSigninConflict() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.T().Cleanup(func() { signup, signin = false, false })

	_, err := executeCommand("login", "--signup", "--signin")
	s.ErrorContains(err, "[signup signin]")
}

// A sign-up that still needs the email address verified worked. Say so and exit
// zero: a caller that reads a non-zero exit as "try again" would sign up twice.
func (s *CmdSuite) TestLoginEmailVerificationPending() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signupFlag, force bool) error {
		return astroAuth.ErrEmailVerificationPending
	}

	buf := new(bytes.Buffer)
	err := login(&cobra.Command{}, []string{"astronomer.io"}, nil, buf)
	s.NoError(err)
	s.Contains(buf.String(), "Check your inbox for a verification email")
	s.Contains(buf.String(), "run 'astro login' to finish signing in")
}

func (s *CmdSuite) TestLogout() {
	localDomain := "localhost"
	apcDomain := "astronomer_dev.com"

	cloudLogout = func(domain string, out io.Writer) {
		s.Equal(localDomain, domain)
	}
	apcLogout = func(domain string) {
		s.Equal(apcDomain, domain)
	}

	// cloud logout success
	err := logout(&cobra.Command{}, []string{localDomain}, os.Stdout)
	s.NoError(err)

	// software logout success
	err = logout(&cobra.Command{}, []string{apcDomain}, os.Stdout)
	s.NoError(err)

	// no domain, cloud logout
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	s.NoError(err)

	// no domain, software logout
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	s.NoError(err)

	// no domain, no current context set
	config.ResetCurrentContext()
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	s.EqualError(err, "no context set, have you authenticated to Astro or APC? Run astro login and try again")
}

func (s *CmdSuite) TestLogoutExpandsShortNames() {
	var loggedOut string
	cloudLogout = func(domain string, out io.Writer) { loggedOut = domain }
	apcLogout = func(domain string) { s.Fail("a short name must not reach the APC logout", domain) }

	for short, host := range map[string]string{"prod": "astronomer.io", "dev": "astronomer-dev.io", "pr41523": "pr41523.astronomer-dev.io"} {
		s.NoError(logout(&cobra.Command{}, []string{short}, io.Discard))
		s.Equal(host, loggedOut)
	}
}

func (s *CmdSuite) TestAuthToken() {
	buf := new(bytes.Buffer)

	// Test with valid token (with Bearer prefix)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	c, err := config.GetCurrentContext()
	s.NoError(err)
	expectedToken := "test-token-12345"
	err = c.SetContextKey("token", "Bearer "+expectedToken)
	s.NoError(err)

	err = printAuthToken(&cobra.Command{}, "", false, textTo(buf))
	s.NoError(err)
	s.Equal(expectedToken+"\n", buf.String())

	// Test with token without Bearer prefix
	buf.Reset()
	err = c.SetContextKey("token", expectedToken)
	s.NoError(err)

	err = printAuthToken(&cobra.Command{}, "", false, textTo(buf))
	s.NoError(err)
	s.Equal(expectedToken+"\n", buf.String())

	// Test with no token (not authenticated)
	buf.Reset()
	err = c.SetContextKey("token", "")
	s.NoError(err)

	err = printAuthToken(&cobra.Command{}, "", false, textTo(buf))
	s.EqualError(err, "no token found. Please run 'astro login' to authenticate")

	// Test with no current context set
	buf.Reset()
	config.ResetCurrentContext()
	err = printAuthToken(&cobra.Command{}, "", false, textTo(buf))
	s.Error(err)
}

// runAuthToken runs `astro auth token` with args the way main runs it.
func runAuthToken(args ...string) (stdout, stderr string, err error) {
	return runCommands(func(out io.Writer) []*cobra.Command {
		auth := &cobra.Command{Use: "auth"}
		auth.AddCommand(newAuthTokenCommand(out))
		return []*cobra.Command{auth}
	}, append([]string{"auth", "token"}, args...)...)
}

func (s *CmdSuite) TestAuthTokenOutput() {
	login := func(token string) config.Context {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		s.Require().NoError(err)
		s.Require().NoError(c.SetContextKey("token", token))
		return c
	}

	s.Run("text is the bare token, for $(astro auth token)", func() {
		login("Bearer the-token")
		stdout, stderr, err := runAuthToken()
		s.NoError(err)
		s.Empty(stderr)
		s.Equal("the-token\n", stdout)
	})

	// signed is a token whose own claims say when it expires, or say nothing.
	signed := func(exp *time.Time) string {
		claims := jwt.RegisteredClaims{Subject: "someone"}
		if exp != nil {
			claims.ExpiresAt = jwt.NewNumericDate(*exp)
		}
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("k"))
		s.Require().NoError(err)
		return tok
	}

	s.Run("json publishes the token, its domain and the expiry it states", func() {
		exp := time.Now().Add(time.Hour).Truncate(time.Second).UTC()
		token := signed(&exp)
		c := login("Bearer " + token)
		s.Require().NoError(c.SetExpiresIn(7200), "what the config records is not the token's word")
		stdout, stderr, err := runAuthToken("-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		got := decodeOne[authToken](s, stdout)
		s.Equal(token, got.Token)
		s.Equal("astronomer.io", got.Domain)
		s.Require().NotNil(got.ExpiresAt)
		s.True(exp.Equal(*got.ExpiresAt), "%v, want %v", got.ExpiresAt, exp)
	})

	s.Run("json falls back to the expiry a saved login recorded", func() {
		c := login("Bearer the-token")
		s.Require().NoError(c.SetExpiresIn(3600))
		stdout, _, err := runAuthToken("-o", "json")
		s.NoError(err)
		got := decodeOne[authToken](s, stdout)
		s.Equal("the-token", got.Token)
		s.Require().NotNil(got.ExpiresAt, "a browser login's token need not be a JWT this reads")
		s.WithinDuration(time.Now().Add(time.Hour), *got.ExpiresAt, time.Minute)
		var raw struct {
			ExpiresAt string `json:"expires_at"`
		}
		s.Require().NoError(json.Unmarshal([]byte(stdout), &raw))
		s.True(strings.HasSuffix(raw.ExpiresAt, "Z"), "published in UTC: %s", raw.ExpiresAt)
	})

	s.Run("json leaves expires_at out for an ASTRO_API_TOKEN that states none, however it is written", func() {
		token := signed(nil)
		for _, env := range []string{token, "bearer " + token, token + " ", "Bearer\t Bearer " + token + "\n"} {
			c := login("")
			s.T().Setenv("ASTRO_API_TOKEN", env)
			// What the login check does with such a token: the variable as
			// given, and a year, made up.
			s.Require().NoError(c.UseEnvironmentLogin("Bearer "+env, time.Now().AddDate(1, 0, 0)))
			stdout, _, err := runAuthToken("-o", "json")
			s.NoError(err)
			s.NotContains(stdout, "expires_at", "ASTRO_API_TOKEN=%q", env)
		}
	})

	s.Run("json leaves expires_at out when nothing says", func() {
		login("the-token")
		stdout, _, err := runAuthToken("-o", "json")
		s.NoError(err)
		s.Equal(authToken{Token: "the-token", Domain: "astronomer.io"}, decodeOne[authToken](s, stdout))
		s.NotContains(stdout, "expires_at")
	})

	s.Run("json with no login fails as unauthenticated", func() {
		login("")
		stdout, stderr, err := runAuthToken("-o", "json")
		s.ErrorIs(err, errNoAuthToken)
		s.Empty(stderr)
		failure := decodeOne[cliout.ErrorObject](s, stdout)
		s.Equal(KindUnauthenticated, failure.Kind)
		s.Equal(1, failure.Code)
	})
}

func (s *CmdSuite) TestAuthTokenWithContext() {
	buf := new(bytes.Buffer)

	// Set up a specific context with a token
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	c, err := config.GetCurrentContext()
	s.NoError(err)
	expectedToken := "context-specific-token"
	err = c.SetContextKey("token", "Bearer "+expectedToken)
	s.NoError(err)

	// Retrieve token using explicit context domain
	err = printAuthToken(&cobra.Command{}, c.Domain, false, textTo(buf))
	s.NoError(err)
	s.Equal(expectedToken+"\n", buf.String())

	// Test with non-existent context
	buf.Reset()
	err = printAuthToken(&cobra.Command{}, "nonexistent.domain.com", false, textTo(buf))
	s.Error(err)
}

func (s *CmdSuite) TestAuthRootCmd() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("auth", "--help")
	s.NoError(err)
	s.Contains(output, "Commands for authenticating to Astro or APC")

	// Test auth login subcommand exists
	output, err = executeCommand("auth", "login", "--help")
	s.NoError(err)
	s.Contains(output, "Authenticate to Astro or APC")

	// Test auth logout subcommand exists
	output, err = executeCommand("auth", "logout", "--help")
	s.NoError(err)
	s.Contains(output, "Log out of Astronomer")

	// Test auth token subcommand exists
	output, err = executeCommand("auth", "token", "--help")
	s.NoError(err)
	s.Contains(output, "Print the current authentication token")
	s.Contains(output, "--domain")
}
