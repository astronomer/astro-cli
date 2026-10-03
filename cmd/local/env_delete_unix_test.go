//go:build !windows

package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bothCopies leaves TOKEN in the project vault and, by hand, in the project
// .env, the state a plain delete has to clear from both stores.
func bothCopies(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory modes these failures are injected with")
	}
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--value", "v"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// readOnly makes a directory unwritable for the rest of the test, so removing
// or replacing a file in it fails.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	// Best effort, so TempDir can clean up.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

// The vault copy goes first, so a vault removal that fails leaves both copies
// where they were and the error says nothing was deleted.
func TestPlainDeleteThatFailsAtTheVaultDeletesNothing(t *testing.T) {
	dir := bothCopies(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	readOnly(t, filepath.Join(home, ".astro", "secrets"))

	d, _, _ := envDeps(t, dir, "")
	err = execute(t, d, "local", "env", "variable", "delete", "TOKEN")
	if err == nil {
		t.Fatal("want the vault failure reported")
	}
	if !strings.Contains(err.Error(), "nothing was deleted") {
		t.Errorf("the error should say nothing was deleted: %v", err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, "TOKEN") {
		t.Errorf("a failed delete removed the plaintext copy anyway (keys: %v)", keys)
	}
	if n := len(vaultFiles(t)); n != 1 {
		t.Errorf("vault holds %d entries, want the one that could not be removed", n)
	}
}

// A file removal that fails after the vault copy went says the vault copy went.
func TestPlainDeleteThatFailsAtTheFileSaysTheVaultCopyWent(t *testing.T) {
	dir := bothCopies(t)
	readOnly(t, dir)

	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "delete", "TOKEN")
	if err == nil {
		t.Fatal("want the file failure reported")
	}
	if msg := err.Error(); !strings.Contains(msg, "deleted variable TOKEN from vault") || !strings.Contains(msg, "could not be removed") {
		t.Errorf("the error should say the vault copy was deleted and the plaintext one was not: %v", err)
	}
	if n := len(vaultFiles(t)); n != 0 {
		t.Errorf("vault holds %d entries, want the vault copy gone", n)
	}
}
