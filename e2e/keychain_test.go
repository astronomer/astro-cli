//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// keyringStampName is pkg/secrets' loginStamp, and "timeout" is its content: a
// keyring that did not answer in time. While it is under an hour old,
// pkg/secrets takes the keychain for unavailable and does not ask it, which is
// what a machine with no keyring does, and what CI's runners are.
const (
	keyringStampName    = "login-keyring-unavailable"
	keyringStampContent = "timeout"
)

// stampKeyringUnavailable writes that stamp into the vault of the home dir,
// so no command run with that home reaches the OS keychain for a login. It is
// the one lever for the keychain: HOME relocates the vault's files and nothing
// relocates the keychain, and a login the config holds is otherwise moved into
// the vault, whose master key is stored there. newNamedProject calls it for
// every project, so no case has to remember.
//
// It covers logins, which is everything a case without a deliberate vault write
// does. An encrypted value written to the vault (`astro local env <noun> set`
// without --plain) is not covered and still reaches the keychain, which is why
// the cases that set one pass --plain (presetArgs).
func stampKeyringUnavailable(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".astro", "secrets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("making the vault directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, keyringStampName), []byte(keyringStampContent), 0o600); err != nil {
		t.Fatalf("stamping the keyring unavailable: %v", err)
	}
}

// keychainOptIn is the variable a person sets to let a case use the real OS
// keychain.
const keychainOptIn = "ASTRO_E2E_KEYCHAIN"

// useRealKeychain is how a case opts in to the real keychain, deliberately: it
// skips unless the person running the suite set ASTRO_E2E_KEYCHAIN=1, and
// otherwise takes the stamp off p, so the commands it runs reach the keychain.
// Writing a credential into somebody's own keychain is not something a suite
// may decide for them, which is why the default is the stamp and the opt-in is
// a variable that CI does not set. why is what the case needs it for.
func useRealKeychain(t *testing.T, p *project, why string) {
	t.Helper()
	if os.Getenv(keychainOptIn) != "1" {
		t.Skipf("needs the real OS keychain (%s); set %s=1 to allow it", why, keychainOptIn)
	}
	if err := os.Remove(filepath.Join(p.home, ".astro", "secrets", keyringStampName)); err != nil {
		t.Fatalf("removing the keyring stamp: %v", err)
	}
}

func keyringStampPath(p *project) string {
	return filepath.Join(p.home, ".astro", "secrets", keyringStampName)
}

// Every project starts stamped, so a case that forgets about the keychain
// cannot reach it.
func TestEveryProjectStartsWithTheKeyringStamped(t *testing.T) {
	for name, p := range map[string]*project{
		"newProject":      newProject(t),
		"newNamedProject": newNamedProject(t, "named"),
		"sibling":         newProject(t).sibling("other"),
	} {
		got, err := os.ReadFile(keyringStampPath(p))
		if err != nil || string(got) != keyringStampContent {
			t.Errorf("%s: the keyring stamp = %q, %v", name, got, err)
		}
	}
}

// The opt-in skips without the variable, and with it removes the stamp.
func TestUseRealKeychainNeedsTheOptIn(t *testing.T) {
	t.Run("without the variable the case is skipped and the stamp stays", func(t *testing.T) {
		t.Setenv(keychainOptIn, "")
		p := newProject(t)
		t.Run("case", func(t *testing.T) { useRealKeychain(t, p, "a test of the opt-in") })
		if _, err := os.Stat(keyringStampPath(p)); err != nil {
			t.Errorf("the stamp is gone though the case never opted in: %v", err)
		}
	})
	t.Run("with the variable the stamp comes off", func(t *testing.T) {
		t.Setenv(keychainOptIn, "1")
		p := newProject(t)
		useRealKeychain(t, p, "a test of the opt-in")
		if _, err := os.Stat(keyringStampPath(p)); !os.IsNotExist(err) {
			t.Errorf("the stamp is still there after opting in: %v", err)
		}
	})
}
