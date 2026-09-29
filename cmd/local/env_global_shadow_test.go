package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A --global set of a name the project scope already holds warns that the
// project copy wins for this project, names the command that removes it, and
// leaves it in place: a --global command does not delete project values.
func TestGlobalSetWarnsAboutAProjectPlaintextCopy(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_TOKEN=plain\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	d, _, stderr := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN", "--value", "v", "--global"); err != nil {
		t.Fatal(err)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "warning: API_TOKEN is also set in project") {
		t.Errorf("a global set shadowed by the project .env should warn; stderr: %q", msg)
	}
	if !strings.Contains(msg, "astro local env variable delete API_TOKEN --project --secret=false") {
		t.Errorf("the warning should name the command that removes the plaintext copy; stderr: %q", msg)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, "API_TOKEN") {
		t.Errorf("a --global set removed the project's copy (keys: %v)", keys)
	}
}

// A project vault copy outranks the global tiers too, so it gets the same
// warning, with the vault's delete command.
func TestGlobalSetWarnsAboutAProjectVaultCopy(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db", "--value", "postgres://u:p@h/db"); err != nil {
		t.Fatal(err)
	}
	d, _, stderr := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db", "--value", "postgres://u:p@g/db", "--global", "--everywhere"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "astro local env connection delete db --project --secret\n") {
		t.Errorf("the warning should name the project vault's delete command; stderr: %q", stderr.String())
	}
	if n := len(vaultFiles(t)); n != 2 {
		t.Errorf("vault holds %d entries, want the project copy kept beside the global one", n)
	}
}

// Nothing in the project scope, nothing to warn about.
func TestGlobalSetWithNoProjectCopyDoesNotWarn(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, stderr := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db", "--value", "postgres://u:p@g/db", "--global", "--everywhere"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), "warning:") {
		t.Errorf("no project copy, but set warned: %q", stderr.String())
	}
}
