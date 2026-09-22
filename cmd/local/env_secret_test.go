package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/vaultenv"
)

// secretEnvProject is envProject plus the isolation the vault needs, which is
// NOT the same isolation the files need.
//
// envProject points ASTRO_HOME at a scratch dir. pkg/secrets deliberately
// ignores ASTRO_HOME — one of the two tools sharing the vault is a GUI app that
// inherits no shell environment, so a vault whose location depends on how the
// process started could move out from under its own index — and resolves the
// real home directory instead. Without the HOME override below, these tests
// would read and write the developer's own ~/.astro/secrets.
//
// USERPROFILE as well as HOME: os.UserHomeDir reads that one on Windows.
//
// The keyring mock is separate again: the master key is keyed by the keyring
// service name and not by any path, so no amount of directory isolation keeps a
// test out of the login keychain.
func secretEnvProject(t *testing.T, envBody string) (dir string) {
	t.Helper()
	dir = envProject(t, envBody)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	keyring.MockInit()
	t.Cleanup(keyring.MockInit) // leave no error-injecting mock behind
	return dir
}

// vaultFiles is what the shared vault holds on disk.
func vaultFiles(t *testing.T) []string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".astro", "secrets"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// --secret is the store choice: the value goes to the vault, the project's .env
// is not touched, and the chain resolves it back labeled with the tier it came
// from.
func TestEnvSetSecretWritesTheVaultNotTheFile(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		content, _ := os.ReadFile(filepath.Join(dir, ".env"))
		t.Errorf("--secret wrote a plain .env:\n%s", content)
	}
	if len(vaultFiles(t)) != 1 {
		t.Errorf("vault holds %v, want exactly one entry", vaultFiles(t))
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "s3cr3t") {
		t.Errorf("get did not return the vaulted value: %q", got)
	}
	// Text mode prints the value alone so it can be piped; the source rides the
	// JSON form.
	d, jsonOut, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "API_TOKEN", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if got := jsonOut.String(); !strings.Contains(got, "vault") {
		t.Errorf("get should name the vault as the source: %q", got)
	}
}

// The value is encrypted at rest. A vault that stored plaintext would pass every
// other test here.
func TestEnvSetSecretStoresCiphertext(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "plaintext-canary\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	secretsDir := filepath.Join(home, ".astro", "secrets")
	for _, name := range vaultFiles(t) {
		raw, err := os.ReadFile(filepath.Join(secretsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "plaintext-canary") {
			t.Errorf("value stored in the clear in %s:\n%s", name, raw)
		}
	}
}

// getJSON runs `get --output json` and decodes it, so a test asserts on the
// SOURCE field rather than on the whole blob. Substring-matching the blob is how
// an assertion accidentally passes on the value instead: a value of
// "global-value" contains "global" whatever tier answered.
func getJSON(t *testing.T, dir string, args ...string) envValue {
	t.Helper()
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, append([]string{"local", "env", "variable", "get"}, append(args, "--output", "json")...)...); err != nil {
		t.Fatalf("get %v: %v", args, err)
	}
	var v envValue
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", out.String(), err)
	}
	return v
}

// --secret composes with the scope flags rather than replacing them: --global
// writes the machine-wide tier, which reports itself distinctly so a user can
// tell which one answered.
func TestEnvSetSecretGlobalIsADistinctTier(t *testing.T) {
	dir := secretEnvProject(t, "")

	// A value with no tier name in it, so only the source field can satisfy the
	// assertion below.
	d, _, _ := envDeps(t, dir, "shared-secret\n")
	if err := execute(t, d, "local", "env", "variable", "set", "SHARED", "--secret", "--global"); err != nil {
		t.Fatal(err)
	}
	got := getJSON(t, dir, "SHARED")
	if got.Value != "shared-secret" {
		t.Errorf("value = %q", got.Value)
	}
	if got.Source != vaultenv.SourceGlobal {
		t.Errorf("source = %q, want the global tier: a --global write must not be keyed to the project", got.Source)
	}

	// And the project tier wins over it for the same name.
	d, _, _ = envDeps(t, dir, "project-secret\n")
	if err := execute(t, d, "local", "env", "variable", "set", "SHARED", "--secret"); err != nil {
		t.Fatal(err)
	}
	got = getJSON(t, dir, "SHARED")
	if got.Value != "project-secret" || got.Source != vaultenv.SourceProject {
		t.Errorf("get = %+v, want the project tier to win", got)
	}
}

// A plaintext entry still beats a vaulted one of the same name, which is the
// chain position this tier was given: below the project's .env.
func TestPlainProjectFileStillBeatsTheVault(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "from-vault\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "from-file\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "from-file") {
		t.Errorf("a hand-written .env entry must still win: %q", got)
	}
}

// delete --secret removes the vaulted value and leaves nothing behind.
func TestEnvDeleteSecret(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "v\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	if names := vaultFiles(t); len(names) != 0 {
		t.Errorf("vault still holds %v after a delete", names)
	}

	// Deleting what is not there says so rather than reporting success.
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "TOKEN", "--secret"); err == nil {
		t.Error("deleting a value that is not set should fail")
	}
}

// The refusal the whole design turns on. With no reachable keyring there is no
// vault, and the command must say so — never fall back to writing a credential
// into a plain file, which is the silent downgrade that would make --secret a
// lie.
func TestEnvSetSecretRefusesWithoutAKeyring(t *testing.T) {
	dir := secretEnvProject(t, "")
	keyring.MockInitWithError(errors.New("no Secret Service available"))

	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN", "--secret")
	if err == nil {
		t.Fatal("want a refusal when the keyring is unreachable")
	}
	if msg := err.Error(); !strings.Contains(msg, "keyring") {
		t.Errorf("the error should name the keyring: %q", msg)
	}
	// And it must point somewhere useful, since this is a property of the
	// machine rather than of what the user typed.
	if msg := err.Error(); !strings.Contains(msg, "environment") {
		t.Errorf("the error should say what to do instead: %q", msg)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(statErr) {
		t.Error("a refused --secret must not fall back to the plain file")
	}
	if names := vaultFiles(t); len(names) != 0 {
		t.Errorf("a refused --secret left %v behind", names)
	}
}

// A connection has to be normalized the same way whichever store holds it, or
// one conn_id means two different things depending on where it was written.
func TestEnvSetSecretNormalizesAConnectionLikeTheFileDoes(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "postgres://u:p@h:5432/db\n")
	if err := execute(t, d, "local", "env", "connection", "set", "my_db", "--secret"); err != nil {
		t.Fatal(err)
	}
	d, vaultOut, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "get", "my_db", "--secret"); err != nil {
		t.Fatal(err)
	}

	// The same input through the plain file, in a fresh project.
	other := secretEnvProject(t, "")
	d, _, _ = envDeps(t, other, "postgres://u:p@h:5432/db\n")
	if err := execute(t, d, "local", "env", "connection", "set", "my_db"); err != nil {
		t.Fatal(err)
	}
	d, fileOut, _ := envDeps(t, other, "")
	if err := execute(t, d, "local", "env", "connection", "get", "my_db", "--project"); err != nil {
		t.Fatal(err)
	}

	if vaultOut.String() == "" || fileOut.String() == "" {
		t.Fatal("both stores should return a value")
	}
	if vaultOut.String() != fileOut.String() {
		t.Errorf("the two stores normalized one connection differently:\nvault: %s\nfile:  %s",
			vaultOut.String(), fileOut.String())
	}
}

// A declared name held only in the vault must not report as absent. The listing
// was blind to the vault: the providers field existed and was never populated,
// so list said "absent" while start injected the value and get returned it.
func TestListSeesAVaultOnlyValue(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = {}\n")

	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "absent") {
		t.Errorf("list reports a vaulted value as absent:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "vault") {
		t.Errorf("list should name the vault as the source:\n%s", out.String())
	}
}

// set/get --secret have to say they used the vault. Reporting the scope name made
// the output byte-identical to a plain set, so nothing reading it could tell
// whether a credential landed in the encrypted store or a committable file.
func TestSecretOperationsNameTheVault(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, out, _ := envDeps(t, dir, "v\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--secret", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "vault") {
		t.Errorf("set --secret should report the vault, got %s", out.String())
	}
	if got := getJSON(t, dir, "TOKEN", "--secret"); got.Source != vaultenv.SourceProject {
		t.Errorf("get --secret source = %q, want %q", got.Source, vaultenv.SourceProject)
	}
}
