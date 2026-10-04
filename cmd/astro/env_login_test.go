package astro

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/secrets"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
)

// The saved login every test here starts from, and must end with.
const (
	savedToken   = "Bearer saved-access"
	savedRefresh = "saved-refresh"
	// envKeyToken is the access token the API key exchange returns.
	envKeyToken = "key-access"
)

// savedLogin writes a config with a login for astronomer.io, the current
// context, kept in a vault of the test's own or, with vault false, in the
// config as every build without a vault keeps it. It returns the home dir.
func savedLogin(t *testing.T, vault bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	prevPath, prevFile := config.HomeConfigPath, config.HomeConfigFile
	t.Cleanup(func() { config.HomeConfigPath, config.HomeConfigFile = prevPath, prevFile })
	t.Setenv("ASTRO_DOMAIN", "")
	t.Setenv("ASTRO_API_TOKEN", "")
	t.Setenv("ASTRONOMER_KEY_ID", "")
	t.Setenv("ASTRONOMER_KEY_SECRET", "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	if vault {
		l, err := secrets.NewLogins(filepath.Join(home, "secrets"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(config.UseLoginsForTesting(l))
	} else {
		t.Cleanup(config.UseLoginsForTesting(nil))
	}
	dir := filepath.Join(home, ".astro")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).Format(time.RFC3339)
	// The shape a 1.x CLI writes: no tenant fields.
	yaml := "context: astronomer.io\ncontexts:\n" +
		"  astronomer_io:\n    domain: astronomer.io\n    token: " + savedToken + "\n    refreshtoken: " + savedRefresh +
		"\n    expiresin: " + exp + "\n    organization: saved-org\n    workspace: saved-ws\n    user_email: someone@example.com\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(afero.NewOsFs())
	// The first read moves the login into the vault, when there is one.
	if c, err := config.GetCurrentContext(); err != nil || c.Token != savedToken {
		t.Fatalf("setting up the saved login: token matches = %v, err = %v", c.Token == savedToken, err)
	}
	inConfig := strings.Contains(readFile(t, filepath.Join(dir, "config.yaml")), "saved-access")
	if inConfig == vault {
		t.Fatalf("login in the config = %v with vault = %v", inConfig, vault)
	}
	return home
}

// snapshot is every file under home, by path, so a test can check that
// nothing a later run reads has changed.
func snapshot(t *testing.T, home string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, ".lock") {
			return err
		}
		b, err := os.ReadFile(path)
		files[path] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// changedFiles reports the paths that differ between two snapshots, without
// their contents.
func changedFiles(before, after map[string][]byte) []string {
	var changed []string
	for p, b := range before {
		if a, ok := after[p]; !ok || !bytes.Equal(a, b) {
			changed = append(changed, p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			changed = append(changed, p)
		}
	}
	return changed
}

// assertSavedLoginIntact reads the login as the next process would, with no
// credential in its environment, and checks it is the one savedLogin wrote and
// that the config file holds none of envTokens.
func assertSavedLoginIntact(t *testing.T, home string, envTokens ...string) {
	t.Helper()
	config.InitConfig(afero.NewOsFs())
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != savedToken || c.RefreshToken != savedRefresh {
		t.Errorf("the saved login changed: token kept = %v, refresh token kept = %v", c.Token == savedToken, c.RefreshToken == savedRefresh)
	}
	if exp, _ := c.GetExpiresIn(); time.Until(exp) > 2*time.Hour || time.Until(exp) < 0 {
		t.Error("the saved login's expiry changed")
	}
	if c.Organization != "saved-org" || c.Workspace != "saved-ws" {
		t.Errorf("the saved selection changed: organization = %q, workspace = %q", c.Organization, c.Workspace)
	}
	raw := readFile(t, filepath.Join(home, ".astro", "config.yaml"))
	for _, token := range envTokens {
		if strings.Contains(raw, token) {
			t.Error("the config file holds the environment's token")
		}
	}
}

// apiToken returns an unsigned-for-real API token: checkAPIToken only reads its
// claims.
func apiToken(t *testing.T, name string) string {
	t.Helper()
	return signedAPIToken(t, name, jwt.NewNumericDate(time.Now().Add(24*time.Hour)))
}

func signedAPIToken(t *testing.T, name string, expiresAt *jwt.NumericDate) string {
	t.Helper()
	claims := util.CustomClaims{
		Permissions: []string{"workspaceId:env-ws", "organizationId:env-org"},
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        name,
			ExpiresAt: expiresAt,
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-signing-key"))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func realTokenParser(t *testing.T) {
	t.Helper()
	previous := parseAPIToken
	t.Cleanup(func() { parseAPIToken = previous })
	parseAPIToken = util.ParseAPIToken
}

func orgsClient(err error) *astrov1_mocks.ClientWithResponsesInterface {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	if err != nil {
		mc.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(nil, err).Maybe()
		return mc
	}
	product := astrov1.OrganizationProductHOSTED
	mc.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{{Name: "env-org", Id: "env-org", Product: &product}},
		},
	}, nil).Maybe()
	mc.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{{Id: "d", WorkspaceId: "env-ws"}}},
	}, nil).Maybe()
	return mc
}

// assertUsesEnvironmentToken checks that this process reads bearer, the
// environment's token, as its login, and that `astro auth token` prints it
// without renewing anything.
func assertUsesEnvironmentToken(t *testing.T, bearer string, refreshes *atomic.Int32) {
	t.Helper()
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != bearer || c.RefreshToken != "" {
		t.Errorf("the command does not use the environment's token: token = %v, no refresh token = %v", c.Token == bearer, c.RefreshToken == "")
	}
	if exp, _ := c.GetExpiresIn(); time.Until(exp) < 23*time.Hour {
		t.Error("the command does not read the environment token's own expiry")
	}
	if c.Organization != "env-org" || c.Workspace != "env-ws" {
		t.Errorf("the command does not use the token's selection: organization = %q, workspace = %q", c.Organization, c.Workspace)
	}
	fresh, err := FreshLogin("", false)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Token != bearer || refreshes.Load() != 0 {
		t.Errorf("auth token: environment token = %v, refreshes = %d", fresh.Token == bearer, refreshes.Load())
	}
}

var errRejected = errors.New("401 Unauthorized")

// A token the CLI cannot use, as a command would meet it: a malformed one,
// and a well-formed one the platform rejects. Neither may touch a byte of what
// the next command reads.
func TestABadEnvironmentTokenLeavesTheSavedLoginUntouched(t *testing.T) {
	for _, vault := range []bool{true, false} {
		for name, tc := range map[string]struct {
			token  func(*testing.T) string
			client *astrov1_mocks.ClientWithResponsesInterface
		}{
			"malformed": {func(*testing.T) string { return "env-not-a-token" }, orgsClient(nil)},
			"rejected":  {func(t *testing.T) string { return apiToken(t, "env-rejected") }, orgsClient(errRejected)},
		} {
			t.Run(name+map[bool]string{true: " in the vault", false: " in the config"}[vault], func(t *testing.T) {
				home := savedLogin(t, vault)
				realTokenParser(t)
				noLoginFlow(t)
				t.Setenv("ASTRO_API_TOKEN", tc.token(t))
				before := snapshot(t, home)

				if err := EnsureLogin(tc.client); err == nil {
					t.Fatal("a bad environment token passed the login check")
				}

				if changed := changedFiles(before, snapshot(t, home)); len(changed) != 0 {
					t.Errorf("a failed login check changed %v", changed)
				}
				envToken := os.Getenv("ASTRO_API_TOKEN")
				t.Setenv("ASTRO_API_TOKEN", "")
				assertSavedLoginIntact(t, home, envToken)
			})
		}
	}
}

// A good token is the command's login, ahead of the saved one, and is still
// not saved: the next command without it finds the user's own login.
func TestAGoodEnvironmentTokenIsUsedButNotSaved(t *testing.T) {
	for _, vault := range []bool{true, false} {
		t.Run(map[bool]string{true: "vault", false: "config"}[vault], func(t *testing.T) {
			home := savedLogin(t, vault)
			realTokenParser(t)
			noLoginFlow(t)
			calls := fakeIDP(t)
			token := apiToken(t, "env-good")
			t.Setenv("ASTRO_API_TOKEN", token)
			before := snapshot(t, home)

			if err := EnsureLogin(orgsClient(nil)); err != nil {
				t.Fatal(err)
			}

			assertUsesEnvironmentToken(t, "Bearer "+token, calls)
			if changed := changedFiles(before, snapshot(t, home)); len(changed) != 0 {
				t.Errorf("a command run with an environment token changed %v", changed)
			}

			envToken := os.Getenv("ASTRO_API_TOKEN")
			t.Setenv("ASTRO_API_TOKEN", "")
			assertSavedLoginIntact(t, home, envToken)
		})
	}
}

// Code that reads the context and writes it back, as an organization or
// workspace switch does, must not store the environment's token or selection
// in place of the saved ones.
func TestWritingBackTheContextDoesNotSaveTheEnvironmentToken(t *testing.T) {
	for _, vault := range []bool{true, false} {
		t.Run(map[bool]string{true: "vault", false: "config"}[vault], func(t *testing.T) {
			home := savedLogin(t, vault)
			realTokenParser(t)
			noLoginFlow(t)
			t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-writeback"))
			if err := EnsureLogin(orgsClient(nil)); err != nil {
				t.Fatal(err)
			}

			c, err := config.GetCurrentContext()
			if err != nil {
				t.Fatal(err)
			}
			for _, kv := range [][2]string{{"token", c.Token}, {"refreshtoken", c.RefreshToken}, {"user_email", c.UserEmail}} {
				if err := c.SetContextKey(kv[0], kv[1]); err != nil {
					t.Fatal(err)
				}
			}
			if err := c.SetSharedContextKey("token", c.Token); err != nil {
				t.Fatal(err)
			}
			if err := c.SetContextKey("workspace", c.Workspace); err != nil {
				t.Fatal(err)
			}
			if err := c.SetOrganizationContext(c.Organization, c.OrganizationProduct); err != nil {
				t.Fatal(err)
			}
			if err := c.SetContext(); err != nil {
				t.Fatal(err)
			}

			envToken := os.Getenv("ASTRO_API_TOKEN")
			t.Setenv("ASTRO_API_TOKEN", "")
			assertSavedLoginIntact(t, home, envToken)
		})
	}
}

// API keys are exchanged for an access token, which is the environment's
// login just as ASTRO_API_TOKEN is.
func TestEnvironmentAPIKeysLeaveTheSavedLoginUntouched(t *testing.T) {
	for _, vault := range []bool{true, false} {
		for name, orgsErr := range map[string]error{"accepted": nil, "rejected": errRejected} {
			t.Run(name+map[bool]string{true: " vault", false: " config"}[vault], func(t *testing.T) {
				home := savedLogin(t, vault)
				noLoginFlow(t)
				previousFetch, previousClient := fetchDomainAuthConfig, client
				t.Cleanup(func() { fetchDomainAuthConfig, client = previousFetch, previousClient })
				fetchDomainAuthConfig = func(domain string) (auth.Config, error) {
					return auth.Config{DomainURL: "https://auth." + domain + "/"}, nil
				}
				client = testUtil.NewTestClient(func(*http.Request) *http.Response {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(`{"access_token":"` + envKeyToken + `","expires_in":3600}`)),
						Header:     make(http.Header),
					}
				})
				t.Setenv("ASTRONOMER_KEY_ID", "env-key-id")
				t.Setenv("ASTRONOMER_KEY_SECRET", "env-key-secret")
				before := snapshot(t, home)

				err := EnsureLogin(orgsClient(orgsErr))
				if (err == nil) != (orgsErr == nil) {
					t.Fatalf("login check err = %v, want an error = %v", err, orgsErr != nil)
				}
				if c, _ := config.GetCurrentContext(); orgsErr == nil && (c.Token != "Bearer "+envKeyToken || c.Organization != "env-org" || c.Workspace != "env-ws") {
					t.Errorf("the command does not use the API key's login: token = %v, organization = %q, workspace = %q", c.Token == "Bearer "+envKeyToken, c.Organization, c.Workspace)
				}
				if changed := changedFiles(before, snapshot(t, home)); len(changed) != 0 {
					t.Errorf("the login check changed %v", changed)
				}

				t.Setenv("ASTRONOMER_KEY_ID", "")
				t.Setenv("ASTRONOMER_KEY_SECRET", "")
				assertSavedLoginIntact(t, home, envKeyToken)
			})
		}
	}
}

// A token that never expires reads as live, as it did when the CLI saved it,
// so nothing takes it for a stale login to renew.
func TestAnEnvironmentTokenWithNoExpiryReadsAsLive(t *testing.T) {
	savedLogin(t, true)
	realTokenParser(t)
	noLoginFlow(t)
	t.Setenv("ASTRO_API_TOKEN", signedAPIToken(t, "env-no-expiry", nil))
	if err := EnsureLogin(orgsClient(nil)); err != nil {
		t.Fatal(err)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if exp, _ := c.GetExpiresIn(); time.Until(exp) < 300*24*time.Hour {
		t.Errorf("a token with no expiry reads as expiring in %v", time.Until(exp))
	}
}
