package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Another process saves a login after this one loaded the config: the
// tests below call that process "the other tool".

const (
	otherToken   = "Bearer other-tool-access"
	otherRefresh = "other-tool-refresh"
)

// vaultConfig is plaintextConfig with the login in the vault.
var vaultConfig = strings.ReplaceAll(strings.ReplaceAll(plaintextConfig,
	"token: "+plainToken, "token: 'Bearer '"),
	"refreshtoken: "+plainRefresh, `refreshtoken: ""`)

// otherToolSavesToTheVault does on disk what a vault-aware tool does when it
// saves a login: the vault entry first, then the config's fields.
func otherToolSavesToTheVault(t *testing.T, l *secrets.Logins) {
	t.Helper()
	if fields := l.Save(HomeConfigFile, "astronomer_io", secrets.Login{Token: otherToken, RefreshToken: otherRefresh}); fields.Token != secrets.LoginInVault {
		t.Fatal("setup: the vault did not take the other tool's login")
	}
	if err := os.WriteFile(HomeConfigFile, []byte(vaultConfig), 0o600); err != nil {
		t.Fatal(err)
	}
}

// keptInConfigMarkers lists the markers that keep a login in the config.
func keptInConfigMarkers(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(os.Getenv("ASTRO_HOME"), "secrets", "login-in-config-*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// readsOtherToolsLogin reports whether a new process reads the other tool's
// login, and that it was not taken for an older tool.
func readsOtherToolsLogin(t *testing.T) {
	t.Helper()
	if login, ok := diskLogin("astronomer_io"); !ok || login.Token != secrets.LoginInVault || login.RefreshToken != "" {
		t.Error("the config no longer points at the other tool's login in the vault")
	}
	initHome(afero.NewOsFs())
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != otherToken || c.RefreshToken != otherRefresh {
		t.Error("a new process does not read the other tool's login")
	}
	if n := len(keptInConfigMarkers(t)); n != 0 {
		t.Errorf("markers keeping the login in the config = %d, want 0", n)
	}
}

// A write of anything else does not put back the token fields this process
// loaded before the other tool saved its login.
func TestAnUnrelatedWriteKeepsANewerLogin(t *testing.T) {
	writes := map[string]func() error{
		"a global setting": func() error { return CFG.PageSize.SetHomeString("50") },
		"a context field": func() error {
			c := Context{Domain: "astronomer.io"}
			return c.SetContextKey("workspace", "another-workspace")
		},
		"an expiry": func() error {
			c := Context{Domain: "astronomer.io"}
			return c.SetExpiresIn(3600)
		},
		"a context switch": func() error { return CFG.Context.SetHomeString("astronomer.io") },
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			l := loginHome(t, plaintextConfig, nil)
			otherToolSavesToTheVault(t, l)
			if err := write(); err != nil {
				t.Fatal(err)
			}
			if configHolds(t, "plain-access-token") || configHolds(t, plainRefresh) {
				t.Error("the write put this process's older login back in the config")
			}
			readsOtherToolsLogin(t)
		})
	}
}

// A workspace switch writes back the whole context it read, login included.
// The login it read is the one this process loaded, so it is not saved over
// the other tool's newer one.
func TestAWholeContextWriteBackKeepsANewerLogin(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	otherToolSavesToTheVault(t, l)
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	c.Workspace = "another-workspace"
	if err := c.SetContext(); err != nil {
		t.Fatal(err)
	}
	if !configHolds(t, "another-workspace") {
		t.Fatal("the workspace was not written")
	}
	readsOtherToolsLogin(t)
}

// Once a write has put this process's login on disk, a later write takes a
// login the other tool saved in between from disk too.
func TestALaterWriteKeepsANewerLogin(t *testing.T) {
	loginHome(t, vaultConfig, nil)
	c := Context{Domain: "astronomer.io"}
	if err := c.SetContextKey(tokenField, "Bearer this-process"); err != nil {
		t.Fatal(err)
	}
	// The other tool, without vault support, logs in.
	newer := strings.ReplaceAll(strings.ReplaceAll(plaintextConfig, "plain-access-token", "older-tool-access"), plainRefresh, "older-tool-refresh")
	if err := os.WriteFile(HomeConfigFile, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CFG.PageSize.SetHomeString("50"); err != nil {
		t.Fatal(err)
	}
	if !configHolds(t, "older-tool-access") || !configHolds(t, "older-tool-refresh") {
		t.Error("a later write replaced the other tool's login with this process's earlier one")
	}
}

// A login this process saves still wins over one the other tool wrote to the
// config since this process loaded it.
func TestThisProcesssOwnLoginStillWins(t *testing.T) {
	saves := map[string]func(c *Context) error{
		"SetContextKey": func(c *Context) error {
			if err := c.SetContextKey(tokenField, "Bearer this-process"); err != nil {
				return err
			}
			return c.SetContextKey(refreshTokenField, "this-process-refresh")
		},
		"SetContext": func(c *Context) error {
			c.Token, c.RefreshToken = "Bearer this-process", "this-process-refresh"
			return c.SetContext()
		},
	}
	for name, save := range saves {
		t.Run(name, func(t *testing.T) {
			l := loginHome(t, vaultConfig, nil)
			// The other tool, without vault support, logs in.
			newer := strings.ReplaceAll(strings.ReplaceAll(plaintextConfig, "plain-access-token", "older-tool-access"), plainRefresh, "older-tool-refresh")
			if err := os.WriteFile(HomeConfigFile, []byte(newer), 0o600); err != nil {
				t.Fatal(err)
			}
			c := Context{Domain: "astronomer.io"}
			if err := save(&c); err != nil {
				t.Fatal(err)
			}
			if configHolds(t, "older-tool") {
				t.Error("this process's login did not replace the other tool's in the config")
			}
			if v := vaultLogin(t, l); v.Token != "Bearer this-process" || v.RefreshToken != "this-process-refresh" {
				t.Error("the vault does not hold this process's login")
			}
			initHome(afero.NewOsFs())
			if got, _ := GetCurrentContext(); got.Token != "Bearer this-process" {
				t.Error("this process's login does not read back")
			}
			if n := len(keptInConfigMarkers(t)); n != 0 {
				t.Errorf("markers keeping the login in the config = %d, want 0", n)
			}
		})
	}
}

// Signing out empties the fields on disk even when the other tool saved a
// login after this process loaded the config.
func TestSignOutClearsANewerLogin(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	otherToolSavesToTheVault(t, l)
	c := Context{Domain: "astronomer.io"}
	if err := c.SignOut(); err != nil {
		t.Fatal(err)
	}
	if login, ok := diskLogin("astronomer_io"); !ok || login != (secrets.Login{}) {
		t.Error("the config does not read as signed out")
	}
	if len(vaultFiles(t)) != 0 {
		t.Error("the vault kept a login after sign-out")
	}
}

// Deleting a context still deletes it, whatever the file on disk holds.
func TestDeleteContextIsNotUndoneByANewerLogin(t *testing.T) {
	l := loginHome(t, plaintextConfig, nil)
	otherToolSavesToTheVault(t, l)
	c := Context{Domain: "astronomer.io"}
	if err := c.DeleteContext(); err != nil {
		t.Fatal(err)
	}
	if _, ok := diskLogin("astronomer_io"); ok {
		t.Error("the deleted context is back in the config")
	}
}

func TestReadingTheConfigAgainForgetsLoginWrites(t *testing.T) {
	loginHome(t, plaintextConfig, nil)
	markLoginWrite("astronomer_io")
	initHome(afero.NewOsFs())
	if wroteLogin("astronomer_io") {
		t.Error("a login write survived reading the config again")
	}
}

const writeBackHelperEnv = "ASTRO_TEST_WRITE_BACK_HELPER"

// TestWriteBackHelper is the process for TestAnotherProcessesWriteKeepsANewerLogin
// that loaded the config first. It does nothing in a normal run.
func TestWriteBackHelper(t *testing.T) {
	if os.Getenv(writeBackHelperEnv) == "" {
		t.Skip("run by TestAnotherProcessesWriteKeepsANewerLogin")
	}
	initHome(afero.NewOsFs())
	if err := os.WriteFile(os.Getenv("ASTRO_TEST_LOADED_FILE"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(os.Getenv("ASTRO_TEST_WRITE_FILE")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never told to write")
		}
	}
	if err := CFG.PageSize.SetHomeString("50"); err != nil {
		t.Fatal(err)
	}
}

// Two real processes: one loads the config, this test moves and renews the
// login in the meantime, then the first writes a setting.
func TestAnotherProcessesWriteKeepsANewerLogin(t *testing.T) {
	if os.Getenv(writeBackHelperEnv) != "" {
		t.Skip("inside a helper")
	}
	l := loginHome(t, plaintextConfig, nil)
	signals := t.TempDir()
	loaded, write := filepath.Join(signals, "loaded"), filepath.Join(signals, "write")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestWriteBackHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), writeBackHelperEnv+"=1", "ASTRO_HOME="+os.Getenv("ASTRO_HOME"), "ASTRO_DOMAIN=",
		"ASTRO_TEST_LOADED_FILE="+loaded, "ASTRO_TEST_WRITE_FILE="+write)
	out := &strings.Builder{}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(loaded); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("the other process never loaded the config\n%s", out)
		}
	}

	c, err := GetCurrentContext() // moves the login into the vault
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey(tokenField, otherToken); err != nil {
		t.Fatal(err)
	}
	if err := c.SetContextKey(refreshTokenField, otherRefresh); err != nil {
		t.Fatal(err)
	}
	if configHolds(t, "plain-access-token") {
		t.Fatal("setup: the login was not moved")
	}

	if err := os.WriteFile(write, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the other process failed: %v\n%s", err, out)
	}
	if !configHolds(t, "page_size") {
		t.Fatal("the other process did not write its setting")
	}
	if v := vaultLogin(t, l); v.Token != otherToken {
		t.Error("the vault lost the renewed login")
	}
	readsOtherToolsLogin(t)
}
