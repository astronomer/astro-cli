package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

const envToken = "Bearer env-api-token"

// A login renewed from the saved refresh token while a command runs on an
// environment token is the user's own, and is saved like any renewal; the
// command keeps reading the environment's.
func TestARenewalDuringAnEnvironmentLoginIsSaved(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UseEnvironmentLogin(envToken, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSharedContextKey(tokenField, "Bearer renewed-access"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSharedContextKey(refreshTokenField, "renewed-refresh"); err != nil {
		t.Fatal(err)
	}
	if c, err := GetCurrentContext(); err != nil || c.Token != envToken {
		t.Errorf("the command stopped reading the environment's token (err %v)", err)
	}

	forgetEnvironmentLogin()
	initHome(afero.NewOsFs())
	if got := vaultLogin(t, l); got.Token != "Bearer renewed-access" || got.RefreshToken != "renewed-refresh" {
		t.Errorf("the renewal was not saved: token = %v, refresh token = %v", got.Token == "Bearer renewed-access", got.RefreshToken == "renewed-refresh")
	}
}

// The environment's token is never saved, under the context it was given to
// or any other.
func TestTheEnvironmentTokenIsNotSavedUnderAnotherContext(t *testing.T) {
	loginHome(t, plaintextConfig+`  other_io:
    domain: other.io
    token: Bearer other-access
    refreshtoken: other-refresh
`, nil)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UseEnvironmentLogin(envToken, time.Time{}); err != nil {
		t.Fatal(err)
	}
	other := Context{Domain: "other.io"}
	if err := other.SetContextKey(tokenField, envToken); err != nil {
		t.Fatal(err)
	}

	forgetEnvironmentLogin()
	initHome(afero.NewOsFs())
	got, err := other.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer other-access" || got.RefreshToken != "other-refresh" {
		t.Error("the other context's login changed")
	}
	if configHolds(t, "env-api-token") {
		t.Error("the config file holds the environment's token")
	}
}

// The organization and workspace an environment credential is for hold for
// the process; a write of the same value is a write-back and is not stored,
// and a different one is the command's own choice: stored, and read back.
func TestTheEnvironmentSelectionIsNotSavedButAChoiceIs(t *testing.T) {
	loginHome(t, plaintextConfig, nil)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UseEnvironmentLogin(envToken, time.Time{}); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{{"organization", "env-org"}, {"workspace", "env-ws"}} {
		if err := c.SetEnvironmentContextKey(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.SetEnvironmentContextKey("token", "x"); err == nil {
		t.Error("SetEnvironmentContextKey accepted a login field")
	}
	if err := (&Context{Domain: "other.io"}).SetEnvironmentContextKey("workspace", "x"); !errors.Is(err, errNoEnvironmentLogin) {
		t.Errorf("err = %v, want errNoEnvironmentLogin for a context without one", err)
	}
	if err := c.SetContextKey("organization", "env-org"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("workspace", "chosen-ws"); err != nil {
		t.Fatal(err)
	}
	got, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if got.Organization != "env-org" || got.Workspace != "chosen-ws" {
		t.Errorf("in process: organization = %q, workspace = %q, want env-org, chosen-ws", got.Organization, got.Workspace)
	}
	if configHolds(t, "env-org") || !configHolds(t, "chosen-ws") {
		t.Error("the config stores the environment's organization, or not the chosen workspace")
	}
}

// The environment's expiry belongs to its token: a context read some other
// way holds the saved login, and its saved expiry.
func TestTheEnvironmentExpiryIsOnlyForItsToken(t *testing.T) {
	saved := time.Now().Add(time.Hour).Truncate(time.Second)
	loginHome(t, plaintextConfig, nil)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetExpiresIn(int64(time.Until(saved).Seconds())); err != nil {
		t.Fatal(err)
	}
	envExpiry := time.Now().Add(24 * time.Hour)
	if err := c.UseEnvironmentLogin(envToken, envExpiry); err != nil {
		t.Fatal(err)
	}
	env, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if exp, _ := env.GetExpiresIn(); !exp.Equal(envExpiry) {
		t.Error("the environment's login does not read its own expiry")
	}
	if exp, _ := c.GetExpiresIn(); exp.Sub(saved).Abs() > 2*time.Second {
		t.Errorf("the saved login reads expiry %v, want %v", exp, saved)
	}
}

// Reading the context with an environment login never asks the keyring: the
// command does not use the saved login, and a locked keyring must not stall
// or warn about it.
func TestAnEnvironmentLoginDoesNotReadTheVault(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	if _, err := GetCurrentContext(); err != nil {
		t.Fatal(err)
	}
	if got := vaultLogin(t, l); got.Token != plainToken {
		t.Fatal("setting up: the login did not move into the vault")
	}
	// A new process, whose keyring does not answer.
	keyring.MockInitWithError(errors.New("keyring locked"))
	fresh, err := secrets.NewLogins(filepath.Join(os.Getenv("ASTRO_HOME"), "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(UseLoginsForTesting(fresh))
	initHome(afero.NewOsFs())
	if err := (&Context{Domain: "astronomer.io"}).UseEnvironmentLogin(envToken, time.Time{}); err != nil {
		t.Fatal(err)
	}

	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != envToken {
		t.Error("the context does not read the environment's token")
	}
	if stamps, _ := filepath.Glob(filepath.Join(os.Getenv("ASTRO_HOME"), "secrets", "login-keyring-*")); len(stamps) != 0 {
		t.Errorf("reading the context asked the keyring: %v", stamps)
	}
}
