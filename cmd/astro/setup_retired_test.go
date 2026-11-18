package astro

import (
	"bytes"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// resetRetiredRenewal clears the run-wide record that a renewal could not be
// saved, before a test and after it, so one test's failure is not the next's.
func resetRetiredRenewal(t *testing.T) {
	t.Helper()
	reset := func() {
		retiredMu.Lock()
		retiredTokens = map[string]bool{}
		warnedUnsaved = false
		retiredMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// A renewal the provider answered and the config would not keep is warned about
// once, naming the file, and the retired refresh token is not presented again
// for the rest of the run.
func TestUnsavedRenewalWarnsOnceAndStopsRetrying(t *testing.T) {
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	path := vaultLogins(t, time.Now().Add(time.Minute))
	calls := fakeIDP(t)
	noLoginFlow(t)
	cfg, err := fetchDomainAuthConfig("astronomer.io")
	require.NoError(t, err)
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetAuthTenant(cfg.DomainURL, cfg.ClientID))
	before := readFile(t, path)
	readOnlyHome(t)

	// ensureLogin hands checkToken os.Stderr; the warning goes where out says.
	var out bytes.Buffer
	require.NoError(t, checkToken(client, &out))
	require.NoError(t, checkToken(client, &out), "the token still works")

	assert.EqualValues(t, 1, calls.Load(), "the retired refresh token is not presented a second time")
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("Warning:")), "warned once: %q", out.String())
	assert.Contains(t, out.String(), config.HomeConfigFile, "the warning names the config file")
	assert.Equal(t, before, readFile(t, path), "nothing was written")
	assert.ErrorIs(t, renewLogin(&c), errRenewalRetired, "nor does any other renewal in this run")
	assert.ErrorIs(t, RefreshLogin(), errRenewalRetired)
}

// Only the refresh token that was spent is blocked: a login that has since been
// replaced renews as usual.
func TestRetiredRenewalBlocksOnlyTheSpentToken(t *testing.T) {
	resetRetiredRenewal(t)
	retireRenewal("spent")
	retireRenewal("spent-too")
	assert.True(t, renewalRetired("spent"))
	assert.False(t, renewalRetired("new-login"))
}

// A provider that is failing or limiting, with a body that is not a token
// answer, is not a refused refresh token: no browser login is offered.
func TestTransientProviderStatusOffersNoLogin(t *testing.T) {
	for _, status := range []int{500, 502, 503, 429} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			loginExpiringIn(t, -time.Hour)
			logins := stubLoginSeams(t, func() error { return nil }, identityProvider(t, status, "<html>try later</html>"))
			err := checkToken(new(astrov1_mocks.ClientWithResponsesInterface), io.Discard)
			require.ErrorIs(t, err, ErrLoginRenewal)
			assert.Zero(t, logins.Load(), "the refresh token was never refused")
		})
	}
	for _, status := range []int{400, 401} {
		t.Run(strconv.Itoa(status)+" with an empty body is a refusal", func(t *testing.T) {
			loginExpiringIn(t, -time.Hour)
			logins := stubLoginSeams(t, func() error { return nil }, identityProvider(t, status, ""))
			_ = checkToken(new(astrov1_mocks.ClientWithResponsesInterface), io.Discard)
			assert.EqualValues(t, 1, logins.Load(), "the refresh token was refused, so a login is offered")
		})
	}
	t.Run("a 4xx with a body that is not JSON is still a refusal", func(t *testing.T) {
		loginExpiringIn(t, -time.Hour)
		logins := stubLoginSeams(t, func() error { return nil }, identityProvider(t, 400, "nope"))
		_ = checkToken(new(astrov1_mocks.ClientWithResponsesInterface), io.Discard)
		assert.EqualValues(t, 1, logins.Load())
	})
}

// A 401 renewal takes another process's renewal, when that renewal is a token
// that works, instead of spending the refresh token a second time. The login
// the caller was refused with is passed in, so no timing is involved.
func TestRefreshLoginTakesAnotherProcessesRenewal(t *testing.T) {
	vaultLogins(t, time.Now().Add(-time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	refused, err := context.GetCurrentContext()
	require.NoError(t, err)
	// Another process renewed it: a different token, good for an hour.
	other := refused
	require.NoError(t, other.SetContextKey("token", "Bearer renewed-elsewhere"))
	require.NoError(t, other.SetExpiresIn(3600))

	require.NoError(t, refreshLoginFrom(&refused))

	assert.Zero(t, calls.Load(), "the other process's renewal was taken")
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	assert.Equal(t, "Bearer renewed-elsewhere", c.Token)
}

// A different token that is itself expired (or about to be) is not a renewal to
// take: the login is renewed from the refresh token it holds now.
func TestRefreshLoginRenewsWhenTheOtherTokenIsStale(t *testing.T) {
	vaultLogins(t, time.Now().Add(-time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	refused, err := context.GetCurrentContext()
	require.NoError(t, err)
	other := refused
	require.NoError(t, other.SetContextKey("token", "Bearer also-stale"))
	require.NoError(t, other.SetExpiresIn(-3600))

	require.NoError(t, refreshLoginFrom(&refused))

	assert.EqualValues(t, 1, calls.Load())
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	assert.Equal(t, "Bearer renewed", c.Token)
}

// A retirement nothing printed does not use up the one warning: a renewal that
// is retired silently first (the 401 path prints nothing) leaves checkToken
// still to say that the login was not saved.
func TestSilentRetirementKeepsTheWarning(t *testing.T) {
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	vaultLogins(t, time.Now().Add(time.Minute))
	fakeIDP(t)
	noLoginFlow(t)
	cfg, err := fetchDomainAuthConfig("astronomer.io")
	require.NoError(t, err)
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetAuthTenant(cfg.DomainURL, cfg.ClientID))
	retireRenewal("some-other-refresh-token") // as RefreshLogin or renewLogin would, silently
	readOnlyHome(t)

	var out bytes.Buffer
	require.NoError(t, checkToken(client, &out))

	assert.Contains(t, out.String(), "could not be saved")
}

// A login that still works does not wait out another process's renewal.
func TestValidTokenDoesNotWaitLongForTheRenewalLock(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Minute))
	calls := fakeIDP(t)
	noLoginFlow(t)
	unlock := config.LockLoginRenewal()
	defer unlock()

	start := time.Now()
	err := checkToken(new(astrov1_mocks.ClientWithResponsesInterface), io.Discard)
	require.NoError(t, err)
	assert.Zero(t, calls.Load(), "no renewal without the lock")
	// Not parallel: the package's fixtures share config state and t.Setenv.
	assert.Less(t, time.Since(start), 20*time.Second)
}
