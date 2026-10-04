package astrosession

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// vaultLogin stores a login for astronomer.io in a vault of the test's own,
// with nothing but the in-vault marker left in the config.
func vaultLogin(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	// InitConfig below repoints the home config at this test's directory, and
	// the next test's fixture writes to whatever path is current before it
	// re-initializes. Put the path back.
	prevPath, prevFile := config.HomeConfigPath, config.HomeConfigFile
	t.Cleanup(func() { config.HomeConfigPath, config.HomeConfigFile = prevPath, prevFile })
	t.Setenv("ASTRO_DOMAIN", "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	l, err := secrets.NewLogins(filepath.Join(home, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.UseLoginsForTesting(l))
	if err := os.MkdirAll(filepath.Join(home, ".astro"), 0o700); err != nil {
		t.Fatal(err)
	}
	yaml := "context: astronomer.io\ncontexts:\n  astronomer_io:\n    domain: astronomer.io\n    token: Bearer vault-login\n    refreshtoken: vault-refresh\n"
	if err := os.WriteFile(filepath.Join(home, ".astro", "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(afero.NewOsFs())
	// The first read moves the login into the vault.
	if c, err := config.GetCurrentContext(); err != nil || c.Token != "Bearer vault-login" {
		t.Fatalf("setting up the vault login: token matches = %v, err = %v", c.Token == "Bearer vault-login", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".astro", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == yaml {
		t.Fatal("the login did not move into the vault")
	}
}

func TestBearerReadsALoginFromTheVault(t *testing.T) {
	vaultLogin(t)
	t.Setenv(EnvAPIToken, "")
	token, err := Bearer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "Bearer vault-login" {
		t.Errorf("token is not the vault's login (len %d)", len(token))
	}
}

// ASTRO_API_TOKEN outranks a saved login exactly as it did when the login was
// in the config.
func TestTheAPITokenVariableStillWinsOverAVaultLogin(t *testing.T) {
	vaultLogin(t)
	t.Setenv(EnvAPIToken, "env-api-token")
	for name, get := range map[string]func() (string, error){
		"Bearer":    func() (string, error) { return Bearer(context.Background()) },
		"BearerFor": func() (string, error) { return BearerFor(context.Background(), "astronomer.io") },
	} {
		token, err := get()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if token != "env-api-token" {
			t.Errorf("%s: token is not the variable's (len %d)", name, len(token))
		}
	}
}
