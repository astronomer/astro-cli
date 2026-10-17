package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestAuthRootCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("login", "--help")
	s.NoError(err)
	s.Contains(output, "Authenticate to Astro or APC")
}

func (s *CmdSuite) TestLogin() {
	buf := new(bytes.Buffer)
	cloudDomain := "astronomer.io"
	apcDomain := "astronomer_dev.com"

	cloudLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
		s.Equal(cloudDomain, domain)
		return nil
	}

	apcLogin = func(domain string, oAuthOnly bool, username, password, token, houstonVersion string, client houston.ClientInterface, out io.Writer) error {
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

// --token-login reaches the APC login, with the domain given or the current
// context's, as it reaches the Astro one: before, the APC login was handed no
// token and asked for one interactively, which a CI job cannot answer.
func (s *CmdSuite) TestLoginPassesTheTokenToAPC() {
	origAPCLogin, origToken := apcLogin, token
	defer func() { apcLogin, token = origAPCLogin, origToken }()

	var got []string
	apcLogin = func(domain string, oAuthOnly bool, username, password, token, houstonVersion string, client houston.ClientInterface, out io.Writer) error {
		got = append(got, token)
		return nil
	}
	token = "ci-token"

	testUtil.InitTestConfig(testUtil.Initial)
	s.NoError(login(&cobra.Command{}, []string{"astronomer_dev.com"}, nil, new(bytes.Buffer)))
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	s.NoError(login(&cobra.Command{}, []string{}, nil, new(bytes.Buffer)))
	s.Equal([]string{"ci-token", "ci-token"}, got)
}

// A token login asks for nothing, so --oauth with it is refused rather than
// silently ignored.
func (s *CmdSuite) TestLoginRefusesTokenWithOAuth() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	_, err := executeCommand("login", "astronomer_dev.com", "--token-login", "ci-token", "--oauth")
	s.ErrorContains(err, "[token-login oauth]")
}

func (s *CmdSuite) TestLogout() {
	localDomain := "localhost"
	apcDomain := "astronomer_dev.com"

	origCloudLogout, origAPCLogout := cloudLogout, apcLogout
	defer func() { cloudLogout, apcLogout = origCloudLogout, origAPCLogout }()

	var cloudLogoutErr error
	cloudLogout = func(domain string, out io.Writer) error {
		s.Equal(localDomain, domain)
		return cloudLogoutErr
	}
	apcLogout = func(domain string) {
		s.Equal(apcDomain, domain)
	}

	// cloud logout success
	err := logout(&cobra.Command{}, []string{localDomain}, os.Stdout)
	s.NoError(err)

	// a cloud logout that fails fails the command, so it exits non-zero
	cloudLogoutErr = errors.New("failed to clear the credentials")
	err = logout(&cobra.Command{}, []string{localDomain}, os.Stdout)
	s.ErrorIs(err, cloudLogoutErr)
	cloudLogoutErr = nil

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

func (s *CmdSuite) TestAuthToken() {
	buf := new(bytes.Buffer)

	// Test with valid token (with Bearer prefix)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	c, err := config.GetCurrentContext()
	s.NoError(err)
	expectedToken := "test-token-12345"
	err = c.SetContextKey("token", "Bearer "+expectedToken)
	s.NoError(err)

	err = printAuthToken(&cobra.Command{}, "", buf)
	s.NoError(err)
	s.Equal(expectedToken+"\n", buf.String())

	// Test with token without Bearer prefix
	buf.Reset()
	err = c.SetContextKey("token", expectedToken)
	s.NoError(err)

	err = printAuthToken(&cobra.Command{}, "", buf)
	s.NoError(err)
	s.Equal(expectedToken+"\n", buf.String())

	// Test with no token (not authenticated)
	buf.Reset()
	err = c.SetContextKey("token", "")
	s.NoError(err)

	err = printAuthToken(&cobra.Command{}, "", buf)
	s.EqualError(err, "no token found. Please run 'astro login' to authenticate")

	// Test with no current context set
	buf.Reset()
	config.ResetCurrentContext()
	err = printAuthToken(&cobra.Command{}, "", buf)
	s.Error(err)
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
	err = printAuthToken(&cobra.Command{}, c.Domain, buf)
	s.NoError(err)
	s.Equal(expectedToken+"\n", buf.String())

	// Test with non-existent context
	buf.Reset()
	err = printAuthToken(&cobra.Command{}, "nonexistent.domain.com", buf)
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
