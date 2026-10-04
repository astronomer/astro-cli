package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/afero"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

const (
	plainToken   = "Bearer plain-access-token"
	plainRefresh = "plain-refresh-token"
)

const plaintextConfig = `context: astronomer.io
contexts:
  astronomer_io:
    domain: astronomer.io
    token: ` + plainToken + `
    refreshtoken: ` + plainRefresh + `
    user_email: someone@example.com
`

// loginHome points config at a home of its own on disk holding yaml, with
// logins kept in a vault of its own under a mock keyring.
func loginHome(t *testing.T, yaml string, keyringErr error) *secrets.Logins {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	t.Setenv("ASTRO_DOMAIN", "")
	// initHome below repoints the home config at this test's directory, and
	// the next test's fixture writes to whatever path is current before it
	// re-initializes. Put the path back.
	prevPath, prevFile := HomeConfigPath, HomeConfigFile
	t.Cleanup(func() { HomeConfigPath, HomeConfigFile = prevPath, prevFile })
	if keyringErr != nil {
		keyring.MockInitWithError(keyringErr)
	} else {
		keyring.MockInit()
	}
	t.Cleanup(keyring.MockInit)
	forgetEnvironmentLogin()
	t.Cleanup(forgetEnvironmentLogin)
	initHome(afero.NewOsFs())
	if yaml != "" {
		if err := os.MkdirAll(HomeConfigPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(HomeConfigFile, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		initHome(afero.NewOsFs())
	}
	l, err := secrets.NewLogins(filepath.Join(home, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(UseLoginsForTesting(l))
	return l
}

// configHolds reports whether the config file on disk contains s, without
// printing either.
func configHolds(t *testing.T, s string) bool {
	t.Helper()
	raw, err := os.ReadFile(HomeConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(raw), s)
}

func vaultLogin(t *testing.T, l *secrets.Logins) secrets.Login {
	t.Helper()
	login, _, err := l.Resolve(HomeConfigFile, "astronomer_io", secrets.Login{Token: secrets.LoginInVault})
	if err != nil {
		t.Fatal(err)
	}
	return login
}

func TestAPlaintextLoginMovesToTheVaultOnRead(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)

	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != plainToken || c.RefreshToken != plainRefresh {
		t.Errorf("context tokens match = %v/%v, want the saved login", c.Token == plainToken, c.RefreshToken == plainRefresh)
	}
	if configHolds(t, "plain-access-token") || configHolds(t, plainRefresh) {
		t.Error("a token is still in the config file after the move")
	}
	if !configHolds(t, "someone@example.com") {
		t.Error("the move lost the context's other fields")
	}
	if got := vaultLogin(t, l); got.Token != plainToken || got.RefreshToken != plainRefresh {
		t.Error("the vault does not hold the login")
	}

	// A fresh process reads it back from the vault.
	initHome(afero.NewOsFs())
	c, err = GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != plainToken || c.RefreshToken != plainRefresh {
		t.Error("the login did not survive a restart")
	}
}

func TestWithoutAKeyringTheLoginStaysInTheConfig(t *testing.T) {
	loginHome(t, plaintextConfig, errors.New("no Secret Service available"))

	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != plainToken || c.RefreshToken != plainRefresh {
		t.Error("the plaintext login is not used without a keyring")
	}
	if !configHolds(t, "plain-access-token") {
		t.Error("the config lost the login although the vault could not take it")
	}

	// Logging in and refreshing keep working, in the config.
	if err := c.SetSharedContextKey(tokenField, "Bearer renewed-access"); err != nil {
		t.Fatal(err)
	}
	if !configHolds(t, "renewed-access") {
		t.Error("a refresh without a keyring did not reach the config")
	}
	c, err = GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "Bearer renewed-access" || c.RefreshToken != plainRefresh {
		t.Error("the refreshed login does not read back")
	}
}

func TestSavingALoginKeepsItOutOfTheConfig(t *testing.T) {
	l := loginHome(t, "", nil)
	c := Context{Domain: "astronomer.io"}
	if err := c.SetContext(); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey(tokenField, "Bearer fresh-access"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey(refreshTokenField, "fresh-refresh"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey("user_email", "someone@example.com"); err != nil {
		t.Fatal(err)
	}
	if configHolds(t, "fresh-access") || configHolds(t, "fresh-refresh") {
		t.Error("a saved token reached the config file")
	}
	got, err := c.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer fresh-access" || got.RefreshToken != "fresh-refresh" || got.UserEmail != "someone@example.com" {
		t.Error("the saved login does not read back")
	}
	if v := vaultLogin(t, l); v.Token != "Bearer fresh-access" {
		t.Error("the vault does not hold the saved token")
	}

	// SetContext writes the whole context, tokens included.
	got.Token = "Bearer whole-context"
	if err := got.SetContext(); err != nil {
		t.Fatal(err)
	}
	if configHolds(t, "whole-context") {
		t.Error("SetContext wrote the token to the config file")
	}
	if again, _ := c.GetContext(); again.Token != "Bearer whole-context" || again.RefreshToken != "fresh-refresh" {
		t.Error("SetContext's login does not read back")
	}
}

// Logout empties both token fields of every context on the tenant: the vault
// entries go, and nothing is left in the config.
func TestLogoutClearsTheVaultAndTheConfig(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{tokenField, refreshTokenField} {
		if err := c.SetSharedContextKey(field, ""); err != nil {
			t.Fatal(err)
		}
	}
	if v := vaultLogin(t, l); v != (secrets.Login{}) {
		t.Error("the vault still holds the login after logout")
	}
	if configHolds(t, "plain-access-token") || configHolds(t, plainRefresh) || configHolds(t, secrets.LoginInVault) {
		t.Error("the config still holds the login after logout")
	}
	if got, _ := GetCurrentContext(); got.Token != "" || got.RefreshToken != "" {
		t.Error("the context still has a login after logout")
	}
}

func TestDeletingAContextRemovesItsLogin(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteContext(); err != nil {
		t.Fatal(err)
	}
	if v := vaultLogin(t, l); v != (secrets.Login{}) {
		t.Error("the vault still holds the deleted context's login")
	}
}

// An older binary logs in again after the move: it writes a plaintext login.
// That login is used and stays in the config, through any number of reads and
// refreshes, so the older binary keeps working. --vault (ResumeLoginVault)
// lets it back into the vault.
func TestAnOlderBinarysLoginStaysInTheConfig(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	if _, err := GetCurrentContext(); err != nil {
		t.Fatal(err)
	}
	if configHolds(t, "plain-access-token") {
		t.Fatal("setup: the first login was not moved")
	}
	newer := strings.ReplaceAll(strings.ReplaceAll(plaintextConfig, "plain-access-token", "older-binary-access"), plainRefresh, "older-binary-refresh")
	if err := os.WriteFile(HomeConfigFile, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		initHome(afero.NewOsFs()) // a new command
		c, err := GetCurrentContext()
		if err != nil {
			t.Fatal(err)
		}
		if c.RefreshToken != "older-binary-refresh" {
			t.Fatalf("command %d: the older binary's login was not used", i)
		}
		if err := c.SetSharedContextKey(tokenField, fmt.Sprintf("Bearer renewed-%d", i)); err != nil {
			t.Fatal(err)
		}
		if !configHolds(t, fmt.Sprintf("renewed-%d", i)) || !configHolds(t, "older-binary-refresh") {
			t.Fatalf("command %d: the login left the config", i)
		}
	}
	if files := vaultFiles(t); len(files) != 0 {
		t.Errorf("vault entries for a login kept in the config: %d, want 0", len(files))
	}

	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ResumeLoginVault(); err != nil {
		t.Fatalf("ResumeLoginVault: %v", err)
	}
	initHome(afero.NewOsFs())
	if _, err := GetCurrentContext(); err != nil {
		t.Fatal(err)
	}
	if configHolds(t, "older-binary-refresh") {
		t.Error("the login stayed in the config after ResumeLoginVault")
	}
	if v := vaultLogin(t, l); v.RefreshToken != "older-binary-refresh" {
		t.Error("the vault does not hold the login after ResumeLoginVault")
	}
}

// An older binary that signs out empties the token fields. The new binary
// reads that as signed out, whatever the vault holds, and removes the vault
// entry the sign-out left behind.
func TestAnOlderBinarysLogoutIsHonored(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	if _, err := GetCurrentContext(); err != nil {
		t.Fatal(err)
	}
	if vaultLogin(t, l).Token == "" {
		t.Fatal("the login did not move into the vault")
	}
	// Saved long enough ago not to be a login another process is recording.
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(HomeConfigPath), "secrets", "*.json"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("vault entries = %d, err = %v, want 1", len(entries), err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(entries[0], old, old); err != nil {
		t.Fatal(err)
	}
	signedOut := strings.ReplaceAll(strings.ReplaceAll(plaintextConfig, plainToken, `""`), plainRefresh, `""`)
	if err := os.WriteFile(HomeConfigFile, []byte(signedOut), 0o600); err != nil {
		t.Fatal(err)
	}
	initHome(afero.NewOsFs())
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "" || c.RefreshToken != "" {
		t.Error("a login survived an older binary's logout")
	}
	if got := vaultLogin(t, l); got.Token != "" || got.RefreshToken != "" {
		t.Error("the vault kept the login after an older binary's logout")
	}
}

// Without a vault, as in every other test in this binary, the config works as
// it always has.
func TestWithoutAVaultTheConfigWorksAsBefore(t *testing.T) {
	loginHome(t, plaintextConfig, nil)
	loginsOverride = nil
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != plainToken || c.RefreshToken != plainRefresh {
		t.Error("the plaintext login is not used without a vault")
	}
	if err := c.SetContextKey(tokenField, "Bearer no-vault"); err != nil {
		t.Fatal(err)
	}
	if !configHolds(t, "no-vault") {
		t.Error("without a vault the token did not reach the config")
	}
}

// vaultFiles lists the login entries in the test's vault directory.
func vaultFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(os.Getenv("ASTRO_HOME"), "secrets", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// Writing back the empty fields a failed read produced must not delete the
// login the read could not open; signing out still does.
func TestAnUnreadableLoginSurvivesAWriteBackButNotASignOut(t *testing.T) {
	loginHome(t, plaintextConfig, nil)
	if _, err := GetCurrentContext(); err != nil {
		t.Fatal(err)
	}
	files := vaultFiles(t)
	if len(files) != 1 {
		t.Fatalf("vault entries = %d, want 1", len(files))
	}
	if err := os.WriteFile(files[0], []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "" || c.RefreshToken != "" {
		t.Fatal("an unreadable login produced tokens")
	}
	// What an organization switch does with the context it read.
	if err := c.SetContextKey(tokenField, c.Token); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey(refreshTokenField, c.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if len(vaultFiles(t)) != 1 || !configHolds(t, "token: 'Bearer '") {
		t.Error("writing back empty fields deleted the unreadable login")
	}
	// What a workspace switch does with it.
	c.Workspace = "another-workspace"
	if err := c.SetContext(); err != nil {
		t.Fatal(err)
	}
	if len(vaultFiles(t)) != 1 || !configHolds(t, "token: 'Bearer '") {
		t.Error("saving the whole context deleted the unreadable login")
	}

	if err := c.SignOut(); err != nil {
		t.Fatal(err)
	}
	if len(vaultFiles(t)) != 0 {
		t.Error("SignOut left the vault entry")
	}
	if configHolds(t, secrets.LoginInVault) {
		t.Error("SignOut left the config pointing at the vault")
	}
}

// This process loaded the config before another tool saved a newer login.
// The older login it holds is used, but not moved over the newer one.
func TestAStaleSnapshotIsNotMoved(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	newer := strings.ReplaceAll(plaintextConfig, "plain-access-token", "newer-access")
	if err := os.WriteFile(HomeConfigFile, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != plainToken {
		t.Error("the loaded login was not used")
	}
	if !configHolds(t, "newer-access") {
		t.Error("the newer login on disk was overwritten")
	}
	if v := vaultLogin(t, l); v != (secrets.Login{}) {
		t.Error("the stale login was moved into the vault")
	}
}

func TestListContextsReadsNoLogin(t *testing.T) {
	loginHome(t, plaintextConfig, nil)
	contexts, err := ListContexts()
	if err != nil {
		t.Fatal(err)
	}
	c := contexts.Contexts["astronomer_io"]
	if c.Token != "" || c.RefreshToken != "" || c.UserEmail != "someone@example.com" {
		t.Error("ListContexts returned a login, or lost the other fields")
	}
	if len(vaultFiles(t)) != 0 {
		t.Error("ListContexts moved a login")
	}
}

// ReloadHome reads under the config's lock, so it waits for a write another
// process holds the lock for rather than reading the file halfway through.
func TestReloadHomeWaitsForAWriteInProgress(t *testing.T) {
	loginHome(t, plaintextConfig, nil)
	// ReloadHome sets HomeConfigFile again while this test waits on it.
	path := HomeConfigFile
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	// Halfway through a write: not a config yet.
	if err := os.WriteFile(path, []byte("contexts:\n  astronomer_io:\n    domain: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		ReloadHome()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("ReloadHome read while another process held the config lock")
	case <-time.After(300 * time.Millisecond):
	}
	if err := os.WriteFile(path, []byte(plaintextConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	<-done
	if unreadableConfigs[path] {
		t.Error("ReloadHome read the config halfway through a write")
	}
}
