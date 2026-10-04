package astro

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

const otherDomain = "astronomer-dev.io"

// vaultLogins writes a config with a login for astronomer.io (current) and
// one for astronomer-dev.io, each expiring at expiresin, and moves both into
// a vault of the test's own. It returns the config file's path.
func vaultLogins(t *testing.T, expiresin time.Time) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	// InitConfig below repoints the home config at this test's directory, and
	// the next test's fixture writes to whatever path is current before it
	// re-initializes. Put the path back.
	prevPath, prevFile := config.HomeConfigPath, config.HomeConfigFile
	t.Cleanup(func() { config.HomeConfigPath, config.HomeConfigFile = prevPath, prevFile })
	t.Setenv("ASTRO_DOMAIN", "")
	t.Setenv("ASTRO_API_TOKEN", "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	l, err := secrets.NewLogins(filepath.Join(home, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.UseLoginsForTesting(l))
	dir := filepath.Join(home, ".astro")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	exp := expiresin.Format(time.RFC3339)
	yaml := "context: astronomer.io\ncontexts:\n" +
		"  astronomer_io:\n    domain: astronomer.io\n    token: Bearer current-old\n    refreshtoken: current-refresh\n    expiresin: " + exp + "\n" +
		"  astronomer-dev_io:\n    domain: astronomer-dev.io\n    token: Bearer other-old\n    refreshtoken: other-refresh\n    expiresin: " + exp + "\n"
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(afero.NewOsFs())
	for _, d := range []string{"astronomer.io", otherDomain} {
		if _, err := context.GetContext(d); err != nil {
			t.Fatal(err)
		}
	}
	if raw := readFile(t, path); strings.Contains(raw, "-old") || strings.Contains(raw, "-refresh") {
		t.Fatal("the logins did not move into the vault")
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// fakeIDP answers token refreshes with access token "renewed" and counts them.
func fakeIDP(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = r.ParseForm()
		if r.PostForm.Get("refresh_token") == "dead" {
			w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token revoked"}`))
			return
		}
		w.Write([]byte(`{"access_token":"renewed","expires_in":3600}`))
	}))
	t.Cleanup(idp.Close)
	previous := fetchDomainAuthConfig
	t.Cleanup(func() { fetchDomainAuthConfig = previous })
	fetchDomainAuthConfig = func(string) (auth.Config, error) {
		return auth.Config{ClientID: "client-id", DomainURL: idp.URL + "/"}, nil
	}
	return &calls
}

func noLoginFlow(t *testing.T) {
	t.Helper()
	previous := authLogin
	t.Cleanup(func() { authLogin = previous })
	authLogin = func(string, string, astrov1.APIClient, io.Writer, bool, bool, bool) error {
		t.Error("the login flow must not start")
		return nil
	}
}

func TestFreshLoginRenewsAnExpiringVaultLoginAndKeepsItInTheVault(t *testing.T) {
	path := vaultLogins(t, time.Now().Add(time.Minute))
	calls := fakeIDP(t)
	noLoginFlow(t)

	c, err := FreshLogin("", false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer renewed" {
		t.Errorf("token was not renewed (len %d)", len(c.Token))
	}
	if calls.Load() != 1 {
		t.Errorf("refreshes = %d, want 1", calls.Load())
	}
	if raw := readFile(t, path); strings.Contains(raw, "renewed") {
		t.Error("the renewed token was written to config.yaml")
	}
	// A later read, through the vault, sees the renewed login without
	// renewing it again.
	again, err := FreshLogin("", false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Token != "Bearer renewed" || calls.Load() != 1 {
		t.Errorf("second read: renewed = %v, refreshes = %d", again.Token == "Bearer renewed", calls.Load())
	}
}

func TestFreshLoginRenewsTheNamedContext(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Minute))
	calls := fakeIDP(t)
	noLoginFlow(t)

	c, err := FreshLogin(otherDomain, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Domain != otherDomain || c.Token != "Bearer renewed" {
		t.Errorf("domain = %q, renewed = %v", c.Domain, c.Token == "Bearer renewed")
	}
	if calls.Load() != 1 {
		t.Errorf("refreshes = %d, want 1", calls.Load())
	}
}

func TestFreshLoginLeavesACurrentTokenAlone(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)

	c, err := FreshLogin("", false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer current-old" || calls.Load() != 0 {
		t.Errorf("unchanged = %v, refreshes = %d", c.Token == "Bearer current-old", calls.Load())
	}
}

func TestFreshLoginReportsARenewalThatFails(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Minute))
	fakeIDP(t)
	noLoginFlow(t)
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("refreshtoken", "dead"); err != nil {
		t.Fatal(err)
	}

	if _, err := FreshLogin("", false); err == nil {
		t.Error("a revoked refresh token should be an error")
	}
}

func TestFreshLoginWithoutALoginStartsNoLoginFlow(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Minute))
	calls := fakeIDP(t)
	noLoginFlow(t)
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SignOut(); err != nil {
		t.Fatal(err)
	}

	got, err := FreshLogin("", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "" || calls.Load() != 0 {
		t.Errorf("signed out: token empty = %v, refreshes = %d", got.Token == "", calls.Load())
	}
}

// Setup gives `astro auth token` no login flow: it would wait for a browser
// that a tool running the command has no way to open.
func TestSetupStartsNoLoginFlowForAuthToken(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Minute))
	noLoginFlow(t)
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SignOut(); err != nil {
		t.Fatal(err)
	}

	root := &cobra.Command{Use: "astro"}
	authCmd := &cobra.Command{Use: "auth"}
	tokenCmd := &cobra.Command{Use: "token", Run: func(*cobra.Command, []string) {}}
	authCmd.AddCommand(tokenCmd)
	root.AddCommand(authCmd)
	root.SetArgs([]string{"auth", "token"})
	cmd, err := root.ExecuteC()
	if err != nil {
		t.Fatal(err)
	}
	if err := Setup(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

// A renewal in the background must not touch the context's other fields:
// saveRenewedToken rewrites the workspace from last_used_workspace, which
// would undo a `workspace switch`.
func TestFreshLoginKeepsTheSelectedWorkspace(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Minute))
	fakeIDP(t)
	noLoginFlow(t)
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("last_used_workspace", "ws-a"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("workspace", "ws-b"); err != nil {
		t.Fatal(err)
	}

	got, err := FreshLogin("", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer renewed" || got.Workspace != "ws-b" {
		t.Errorf("renewed = %v, workspace = %q, want ws-b", got.Token == "Bearer renewed", got.Workspace)
	}
}

// --force renews a token the config records as current, for one the platform
// refused although it looked unexpired.
func TestFreshLoginWithForceRenewsACurrentToken(t *testing.T) {
	path := vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)

	c, err := FreshLogin("", true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer renewed" || calls.Load() != 1 {
		t.Errorf("renewed = %v, refreshes = %d, want 1", c.Token == "Bearer renewed", calls.Load())
	}
	if raw := readFile(t, path); strings.Contains(raw, "renewed") {
		t.Error("the renewed token was written to config.yaml")
	}
}

// --force with nothing to renew from is an error, not a prompt.
func TestFreshLoginWithForceAndNoRefreshTokenFails(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("refreshtoken", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := FreshLogin("", true); !errors.Is(err, errNoRefreshToken) {
		t.Errorf("err = %v, want errNoRefreshToken", err)
	}
	if calls.Load() != 0 {
		t.Errorf("refreshes = %d, want 0", calls.Load())
	}
	// Without --force the token is printed as it is, as before.
	if got, err := FreshLogin("", false); err != nil || got.Token != "Bearer current-old" {
		t.Errorf("without --force: unchanged = %v, err = %v", got.Token == "Bearer current-old", err)
	}
}

// A token from the environment cannot be renewed; --force says so rather than
// printing the saved login's token in its place.
func TestFreshLoginWithForceRefusesEnvironmentCredentials(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	t.Setenv("ASTRO_API_TOKEN", "from-the-environment")

	if _, err := FreshLogin("", true); !errors.Is(err, errForceWithEnvCredentials) {
		t.Errorf("err = %v, want errForceWithEnvCredentials", err)
	}
	if calls.Load() != 0 {
		t.Errorf("refreshes = %d, want 0", calls.Load())
	}
}

// A context kept in the config for an older CLI stays there when --force
// renews it, so that CLI keeps the renewed login.
func TestFreshLoginWithForceKeepsALoginForAnOlderCLIInTheConfig(t *testing.T) {
	path := vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	// An older CLI logs in over the moved login.
	exp := time.Now().Add(time.Hour).Format(time.RFC3339)
	older := "context: astronomer.io\ncontexts:\n" +
		"  astronomer_io:\n    domain: astronomer.io\n    token: Bearer older-access\n    refreshtoken: older-refresh\n    expiresin: " + exp + "\n"
	if err := os.WriteFile(path, []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(afero.NewOsFs())

	c, err := FreshLogin("", true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer renewed" || calls.Load() != 1 {
		t.Errorf("renewed = %v, refreshes = %d, want 1", c.Token == "Bearer renewed", calls.Load())
	}
	if raw := readFile(t, path); !strings.Contains(raw, "renewed") || !strings.Contains(raw, "older-refresh") {
		t.Error("the renewed login left the config of a context an older CLI uses")
	}
}

const renewHelperEnv = "ASTRO_TEST_FORCED_RENEWAL_HELPER"

// TestForcedRenewalHelper is one `astro auth token --force` process for
// TestConcurrentForcedRenewalsRefreshOnce. It does nothing in a normal run.
func TestForcedRenewalHelper(t *testing.T) {
	if os.Getenv(renewHelperEnv) == "" {
		t.Skip("run by TestConcurrentForcedRenewalsRefreshOnce")
	}
	idpURL, start := os.Getenv("ASTRO_TEST_IDP_URL"), os.Getenv("ASTRO_TEST_START_FILE")
	fetchDomainAuthConfig = func(string) (auth.Config, error) {
		return auth.Config{ClientID: "client-id", DomainURL: idpURL + "/"}, nil
	}
	config.InitConfig(afero.NewOsFs())
	// Wait for the other process, so both read the same token before either
	// renews it.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(start); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never started")
		}
	}
	c, err := FreshLogin("", true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer renewed" {
		t.Errorf("token is not the renewal (len %d)", len(c.Token))
	}
}

// Otto and its subagents can each run `astro auth token --force` at the same
// moment with the same refresh token. The renewal happens once: the second
// process waits, re-reads the login and returns the first one's renewal.
func TestConcurrentForcedRenewalsRefreshOnce(t *testing.T) {
	if os.Getenv(renewHelperEnv) != "" {
		t.Skip("inside a helper")
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".astro")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).Format(time.RFC3339)
	yaml := "context: astronomer.io\ncontexts:\n" +
		"  astronomer_io:\n    domain: astronomer.io\n    token: Bearer refused\n    refreshtoken: shared-refresh\n    expiresin: " + exp + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(300 * time.Millisecond) // long enough for the other process to be waiting
		w.Write([]byte(`{"access_token":"renewed","expires_in":3600}`))
	}))
	t.Cleanup(idp.Close)
	start := filepath.Join(t.TempDir(), "start")

	// os.Executable, not os.Args[0]: other tests in this package replace os.Args.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmds := make([]*exec.Cmd, 2)
	outs := make([]*strings.Builder, 2)
	for i := range cmds {
		cmd := exec.Command(self, "-test.run=^TestForcedRenewalHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(), renewHelperEnv+"=1", "ASTRO_HOME="+home, "ASTRO_DOMAIN=", "ASTRO_API_TOKEN=",
			"ASTRONOMER_KEY_ID=", "ASTRONOMER_KEY_SECRET=", "ASTRO_TEST_IDP_URL="+idp.URL, "ASTRO_TEST_START_FILE="+start)
		outs[i] = &strings.Builder{}
		cmd.Stdout, cmd.Stderr = outs[i], outs[i]
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds[i] = cmd
	}
	time.Sleep(200 * time.Millisecond) // both processes loaded the config
	if err := os.WriteFile(start, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Errorf("process %d failed: %v\n%s", i, err, outs[i])
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("refreshes = %d, want 1", n)
	}
}

// A process that starts just after another renewed reads the renewal as its
// own starting token. With --force it takes a token issued within the last
// minute as that renewal and does not refresh again.
func TestFreshLoginWithForceKeepsAJustIssuedToken(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	fresh := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(time.Now().Add(-10 * time.Second))})
	signed, err := fresh.SignedString([]byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("token", "Bearer "+signed); err != nil {
		t.Fatal(err)
	}

	got, err := FreshLogin("", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer "+signed || calls.Load() != 0 {
		t.Errorf("unchanged = %v, refreshes = %d, want 0", got.Token == "Bearer "+signed, calls.Load())
	}

	// A token issued longer ago is renewed.
	old := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(time.Now().Add(-renewedRecently - time.Minute))})
	signed, err = old.SignedString([]byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("token", "Bearer "+signed); err != nil {
		t.Fatal(err)
	}
	if got, err := FreshLogin("", true); err != nil || got.Token != "Bearer renewed" || calls.Load() != 1 {
		t.Errorf("old token: renewed = %v, refreshes = %d, err = %v", got.Token == "Bearer renewed", calls.Load(), err)
	}
}

// A machine whose clock is behind reads every token as issued in its future.
// --force still renews one issued well ahead of the local clock, rather than
// taking it as just renewed until the clock catches up.
func TestFreshLoginWithForceRenewsATokenFromTheFuture(t *testing.T) {
	vaultLogins(t, time.Now().Add(time.Hour))
	calls := fakeIDP(t)
	noLoginFlow(t)
	ahead := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour))})
	signed, err := ahead.SignedString([]byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("token", "Bearer "+signed); err != nil {
		t.Fatal(err)
	}
	if got, err := FreshLogin("", true); err != nil || got.Token != "Bearer renewed" || calls.Load() != 1 {
		t.Errorf("renewed = %v, refreshes = %d, err = %v", got.Token == "Bearer renewed", calls.Load(), err)
	}
}
