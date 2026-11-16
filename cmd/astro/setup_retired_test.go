package astro

import (
	"bytes"
	"io"
	"strconv"
	"sync"
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
		retired = false
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
	t.Run("a 4xx with a body that is not JSON is still a refusal", func(t *testing.T) {
		loginExpiringIn(t, -time.Hour)
		logins := stubLoginSeams(t, func() error { return nil }, identityProvider(t, 400, "nope"))
		_ = checkToken(new(astrov1_mocks.ClientWithResponsesInterface), io.Discard)
		assert.EqualValues(t, 1, logins.Load())
	})
}

// A 401 renewal takes the renewal lock, and takes another process's renewal
// instead of spending the refresh token a second time.
func TestRefreshLoginTakesAnotherProcessesRenewal(t *testing.T) {
	vaultLogins(t, time.Now().Add(-time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)

	unlock := config.LockLoginRenewal()
	var wg sync.WaitGroup
	var got error
	wg.Add(1)
	go func() {
		defer wg.Done()
		got = RefreshLogin()
	}()
	time.Sleep(300 * time.Millisecond) // RefreshLogin has read the login and waits on the lock
	c, err := context.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetContextKey("token", "Bearer renewed-elsewhere"))
	unlock()
	wg.Wait()

	require.NoError(t, got)
	assert.Zero(t, calls.Load(), "the other process's renewal was taken")
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
	assert.Less(t, time.Since(start), 8*time.Second)
}
