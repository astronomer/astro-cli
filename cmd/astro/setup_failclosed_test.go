package astro

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
)

// stubLoginSeams replaces the login flow and the auth config lookup for one
// test, and puts both back in its own cleanup.
func stubLoginSeams(t *testing.T, login func() error, authConfig func() (auth.Config, error)) *atomic.Int32 {
	t.Helper()
	prevLogin, prevFetch := authLogin, fetchDomainAuthConfig
	t.Cleanup(func() { authLogin, fetchDomainAuthConfig = prevLogin, prevFetch })
	var logins atomic.Int32
	authLogin = func(string, string, astrov1.APIClient, io.Writer, bool, bool, bool) error {
		logins.Add(1)
		return login()
	}
	fetchDomainAuthConfig = func(string) (auth.Config, error) { return authConfig() }
	return &logins
}

// identityProvider answers every token request with status and body, and is
// the auth config of a login that renews against it.
func identityProvider(t *testing.T, status int, body string) func() (auth.Config, error) {
	t.Helper()
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(idp.Close)
	return func() (auth.Config, error) {
		return auth.Config{ClientID: "client-id", DomainURL: idp.URL + "/"}, nil
	}
}

const rejectedRefresh = `{"error":"invalid_grant","error_description":"Unknown or invalid refresh token."}`

// loginExpiringIn makes the current context hold an access token that expires in
// expiresIn (negative: it expired that long ago), with a refresh token.
func loginExpiringIn(t *testing.T, expiresIn time.Duration) {
	t.Helper()
	resetRetiredRenewal(t)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetContextKey("token", "Bearer saved-access"))
	require.NoError(t, c.SetContextKey("refreshtoken", "saved-refresh"))
	require.NoError(t, c.SetExpiresIn(int64(expiresIn/time.Second)))
}

func TestCheckTokenFailsClosed(t *testing.T) {
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	unreachable := func() (auth.Config, error) {
		return auth.Config{}, &url.Error{Op: "Get", URL: "https://auth.example/", Err: &net.OpError{Op: "dial", Err: io.ErrUnexpectedEOF}}
	}
	neverLogin := func() error {
		t.Error("the login flow should not have started")
		return errors.New("the login flow should not have started")
	}

	t.Run("a login the run may not start is ErrLoginNeeded", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		c, err := context.GetCurrentContext()
		require.NoError(t, err)
		require.NoError(t, c.SetContextKey("token", ""))
		stubLoginSeams(t, func() error { return auth.ErrLoginNeeded }, unreachable)
		assert.ErrorIs(t, checkToken(client, io.Discard), auth.ErrLoginNeeded)
	})

	t.Run("an interrupted login is the interrupt", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		c, err := context.GetCurrentContext()
		require.NoError(t, err)
		require.NoError(t, c.SetContextKey("token", ""))
		stubLoginSeams(t, func() error { return input.ErrInterrupted }, unreachable)
		assert.ErrorIs(t, checkToken(client, io.Discard), input.ErrInterrupted)
	})

	t.Run("a failed device login is its own error", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		c, err := context.GetCurrentContext()
		require.NoError(t, err)
		require.NoError(t, c.SetContextKey("token", ""))
		stubLoginSeams(t, func() error { return errorLogin }, unreachable)
		assert.ErrorIs(t, checkToken(client, io.Discard), errorLogin)
	})

	t.Run("an expired login whose refresh is rejected, in a run that may not ask", func(t *testing.T) {
		loginExpiringIn(t, -time.Hour)
		logins := stubLoginSeams(t, func() error { return auth.ErrLoginNeeded }, identityProvider(t, http.StatusBadRequest, rejectedRefresh))
		err := checkToken(client, io.Discard)
		require.ErrorIs(t, err, ErrLoginRenewal)
		assert.ErrorContains(t, err, "run 'astro login' to log in again")
		assert.ErrorContains(t, err, "Unknown or invalid refresh token")
		assert.EqualValues(t, 1, logins.Load(), "a rejected refresh token is offered a login first")
	})

	t.Run("an expired login whose refresh is rejected, in a run that asks, logs in again", func(t *testing.T) {
		loginExpiringIn(t, -time.Hour)
		logins := stubLoginSeams(t, func() error { return nil }, identityProvider(t, http.StatusBadRequest, rejectedRefresh))
		assert.NoError(t, checkToken(client, io.Discard))
		assert.EqualValues(t, 1, logins.Load())
	})

	t.Run("an expired login whose provider cannot be reached", func(t *testing.T) {
		loginExpiringIn(t, -time.Hour)
		stubLoginSeams(t, neverLogin, unreachable)
		err := checkToken(client, io.Discard)
		require.ErrorIs(t, err, ErrLoginRenewal)
		assert.True(t, errorsIsTransport(err), "the cause stays in the chain: %v", err)
	})

	t.Run("an interrupted re-login after a rejected refresh is the interrupt", func(t *testing.T) {
		loginExpiringIn(t, -time.Hour)
		stubLoginSeams(t, func() error { return input.ErrInterrupted }, identityProvider(t, http.StatusBadRequest, rejectedRefresh))
		assert.ErrorIs(t, checkToken(client, io.Discard), input.ErrInterrupted)
	})

	t.Run("a refresh that fails while the token still works is only logged", func(t *testing.T) {
		// Inside the five-minute margin, not yet expired.
		loginExpiringIn(t, time.Minute)
		logins := stubLoginSeams(t, neverLogin, identityProvider(t, http.StatusBadGateway, `{"error":"server_error","error_description":"bad gateway"}`))
		assert.NoError(t, checkToken(client, io.Discard))
		assert.Zero(t, logins.Load())
		c, err := context.GetCurrentContext()
		require.NoError(t, err)
		assert.Equal(t, "Bearer saved-access", c.Token, "the login is left as it was")
	})

	t.Run("an unreachable provider while the token still works is only logged", func(t *testing.T) {
		loginExpiringIn(t, time.Minute)
		stubLoginSeams(t, neverLogin, unreachable)
		assert.NoError(t, checkToken(client, io.Discard))
	})

	t.Run("a login that is not near expiry is not renewed at all", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		stubLoginSeams(t, neverLogin, func() (auth.Config, error) {
			t.Error("the provider should not have been asked")
			return auth.Config{}, nil
		})
		assert.NoError(t, checkToken(client, io.Discard))
	})

	t.Run("no context at all", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
		stubLoginSeams(t, neverLogin, unreachable)
		assert.ErrorIs(t, checkToken(client, io.Discard), config.ErrGetHomeString)
	})

	t.Run("a config file that did not parse names the file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, []byte("context: [unterminated\n  : :"), 0o600))
		config.InitConfig(fs)
		t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
		stubLoginSeams(t, neverLogin, unreachable)
		err := checkToken(client, io.Discard)
		require.ErrorIs(t, err, ErrConfigUnreadable)
		assert.ErrorContains(t, err, config.HomeConfigFile)
		assert.ErrorContains(t, err, "did not parse")
	})
}

func errorsIsTransport(err error) bool {
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

func mintToken(t *testing.T, claims *util.CustomClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	return token
}

func TestSetupFailsClosedOnTheAPIToken(t *testing.T) {
	prevParse := parseAPIToken
	t.Cleanup(func() { parseAPIToken = prevParse })
	parseAPIToken = util.ParseAPIToken
	setupCmd := func() *cobra.Command {
		root := &cobra.Command{Use: "astro"}
		leaf := &cobra.Command{Use: "list"}
		root.AddCommand(leaf)
		return leaf
	}
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	t.Run("a token with no permissions is not a token, and the saved login is not used instead", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		logins := stubLoginSeams(t, func() error { return nil }, identityProvider(t, http.StatusOK, `{}`))
		t.Setenv("ASTRO_API_TOKEN", mintToken(t, &util.CustomClaims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}))
		err := Setup(setupCmd(), client)
		require.ErrorIs(t, err, ErrNotAPIToken)
		assert.EqualError(t, err, "ASTRO_API_TOKEN is not an Astro API token (it has no permissions): set a valid token, or unset it to use your login")
		assert.Zero(t, logins.Load())
		assert.False(t, config.HoldsEnvironmentLogin(), "the saved login must not be the run's")
		client.AssertNotCalled(t, "ListOrganizationsWithResponse")
	})

	t.Run("an expired token says so, and prints nothing", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		stubLoginSeams(t, func() error { return nil }, identityProvider(t, http.StatusOK, `{}`))
		t.Setenv("ASTRO_API_TOKEN", mintToken(t, &util.CustomClaims{
			Permissions:      []string{"organizationId:org"},
			RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))},
		}))
		stderr := captureStderr(t, func() {
			err := Setup(setupCmd(), client)
			assert.EqualError(t, err, "API Token is expired")
			assert.ErrorIs(t, err, ErrAPITokenExpired)
		})
		assert.Empty(t, stderr, "the failure is the error, not a line beside it")
	})

	t.Run("a malformed token says so", func(t *testing.T) {
		loginExpiringIn(t, time.Hour)
		stubLoginSeams(t, func() error { return nil }, identityProvider(t, http.StatusOK, `{}`))
		t.Setenv("ASTRO_API_TOKEN", "not-a-jwt")
		err := Setup(setupCmd(), client)
		assert.EqualError(t, err, "API Token is invalid or malformed")
		assert.ErrorIs(t, err, ErrAPITokenMalformed)
	})
}

func TestEnvironmentOverlayFailuresAreReturned(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
	// No environment login was applied to this context, so recording a
	// selection for it has nothing to attach to.
	err := useEnvironmentSelection(&config.Context{Domain: "astronomer.io"}, "org", "HOSTED", "ws")
	require.Error(t, err)
	assert.ErrorContains(t, err, "cannot apply ASTRO_API_TOKEN's workspace to this run")
	assert.False(t, config.HoldsEnvironmentLogin())
}

func TestSetupSkipsCompletion(t *testing.T) {
	// Logged out, with a login flow that fails the test if it is reached.
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetContextKey("token", ""))
	logins := stubLoginSeams(t, func() error { return auth.ErrLoginNeeded }, func() (auth.Config, error) {
		t.Error("completion reached the auth config")
		return auth.Config{}, io.ErrUnexpectedEOF
	})
	t.Setenv("ASTRO_API_TOKEN", "garbage")
	for _, name := range []string{cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd} {
		root := &cobra.Command{Use: "astro"}
		req := &cobra.Command{Use: name}
		root.AddCommand(req)
		assert.NoError(t, Setup(req, new(astrov1_mocks.ClientWithResponsesInterface)), name)
		assert.True(t, IsCompletion(req), name)
	}
	assert.Zero(t, logins.Load())
}
