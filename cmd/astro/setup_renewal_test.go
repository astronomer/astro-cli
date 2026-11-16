package astro

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// readOnlyHome makes the directory holding the home config unwritable, so a
// renewal can be had from the provider and not saved.
func readOnlyHome(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a directory cannot be made unwritable here")
	}
	require.NoError(t, os.Chmod(config.HomeConfigPath, 0o500))
	t.Cleanup(func() { _ = os.Chmod(config.HomeConfigPath, 0o700) })
}

// A renewal that cannot be saved is a failure of the renewal, soft while the
// login's access token still works and hard once it does not. The provider
// may already have retired the old refresh token in the second case.
func TestRenewalThatCannotBeSaved(t *testing.T) {
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	for _, tc := range []struct {
		name string
		// tenantKnown: the context already records the provider's tenant, so
		// the failure comes at saving the renewed login, not before.
		tenantKnown bool
	}{
		{"the tenant cannot be recorded", false},
		{"the renewed login cannot be saved", true},
	} {
		for _, valid := range []bool{true, false} {
			name := tc.name + ", access token expired"
			expiresIn := -time.Hour
			if valid {
				name = tc.name + ", access token still valid"
				expiresIn = time.Minute
			}
			t.Run(name, func(t *testing.T) {
				path := vaultLogins(t, time.Now().Add(expiresIn))
				calls := fakeIDP(t)
				noLoginFlow(t)
				if tc.tenantKnown {
					cfg, err := fetchDomainAuthConfig("astronomer.io")
					require.NoError(t, err)
					c, err := context.GetCurrentContext()
					require.NoError(t, err)
					require.NoError(t, c.SetAuthTenant(cfg.DomainURL, cfg.ClientID))
				}
				before := readFile(t, path)
				readOnlyHome(t)

				err := checkToken(client, os.Stderr)

				if valid {
					assert.NoError(t, err, "the token still works, so the command runs on it")
				} else {
					require.ErrorIs(t, err, ErrLoginRenewal)
					assert.ErrorContains(t, err, "run 'astro login' to log in again")
				}
				if tc.tenantKnown {
					assert.EqualValues(t, 1, calls.Load(), "the provider was asked, and answered")
				}
				assert.Equal(t, before, readFile(t, path), "nothing was written")
			})
		}
	}
}

const checkTokenHelperEnv = "ASTRO_TEST_CHECK_TOKEN_HELPER"

// TestCheckTokenHelper is one command's login check for
// TestConcurrentLoginChecksRenewOnce. It does nothing in a normal run.
func TestCheckTokenHelper(t *testing.T) {
	if os.Getenv(checkTokenHelperEnv) == "" {
		t.Skip("run by TestConcurrentLoginChecksRenewOnce")
	}
	idpURL, start := os.Getenv("ASTRO_TEST_IDP_URL"), os.Getenv("ASTRO_TEST_START_FILE")
	fetchDomainAuthConfig = func(string) (auth.Config, error) {
		return auth.Config{ClientID: "client-id", DomainURL: idpURL + "/"}, nil
	}
	authLogin = func(string, string, astrov1.APIClient, io.Writer, bool, bool, bool) error {
		t.Error("the login flow must not start")
		return nil
	}
	config.InitConfig(afero.NewOsFs())
	// Never touch a config that is not the parent's: a helper that lost its
	// ASTRO_HOME would renew, and save over, the developer's own login.
	if home := os.Getenv("ASTRO_HOME"); home == "" || !strings.HasPrefix(config.HomeConfigFile, home) {
		t.Fatalf("the helper's config is %s, not under ASTRO_HOME=%q", config.HomeConfigFile, home)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(start); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never started")
		}
	}
	if err := checkToken(nil, os.Stderr); err != nil {
		t.Fatal(err)
	}
	c, err := context.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer "+os.Getenv("ASTRO_TEST_RENEWED_TOKEN") {
		t.Errorf("token is not the renewal (len %d)", len(c.Token))
	}
}

// Two commands that find the same access token about to expire renew it once:
// the second waits for the first, re-reads the login and takes its renewal. A
// refresh token the provider rotates would fail the loser otherwise.
func TestConcurrentLoginChecksRenewOnce(t *testing.T) {
	if os.Getenv(checkTokenHelperEnv) != "" {
		t.Skip("inside a helper")
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".astro")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	exp := time.Now().Add(time.Minute).Format(time.RFC3339)
	yaml := "context: astronomer.io\ncontexts:\n" +
		"  astronomer_io:\n    domain: astronomer.io\n    token: Bearer about-to-expire\n    refreshtoken: shared-refresh\n    expiresin: " + exp + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o600))
	const renewed = "renewed-access-token"
	var calls atomic.Int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(300 * time.Millisecond) // long enough for the other process to be waiting
		_, _ = w.Write([]byte(`{"access_token":"` + renewed + `","expires_in":3600}`))
	}))
	t.Cleanup(idp.Close)
	start := filepath.Join(t.TempDir(), "start")

	self, err := os.Executable()
	require.NoError(t, err)
	cmds := make([]*exec.Cmd, 2)
	outs := make([]*strings.Builder, 2)
	for i := range cmds {
		cmd := exec.Command(self, "-test.run=^TestCheckTokenHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(), checkTokenHelperEnv+"=1", "ASTRO_HOME="+home, "ASTRO_DOMAIN=", "ASTRO_API_TOKEN=",
			"ASTRONOMER_KEY_ID=", "ASTRONOMER_KEY_SECRET=", "ASTRO_TEST_IDP_URL="+idp.URL, "ASTRO_TEST_START_FILE="+start,
			"ASTRO_TEST_RENEWED_TOKEN="+renewed)
		outs[i] = &strings.Builder{}
		cmd.Stdout, cmd.Stderr = outs[i], outs[i]
		require.NoError(t, cmd.Start())
		cmds[i] = cmd
	}
	time.Sleep(200 * time.Millisecond) // both processes loaded the config
	require.NoError(t, os.WriteFile(start, nil, 0o600))
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Errorf("process %d failed: %v\n%s", i, err, outs[i])
		}
	}
	assert.EqualValues(t, 1, calls.Load(), "refreshes")
}
