package local

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/vaultenv"
)

// dotenvKeys returns the keys a dotenv file holds, never the values, so a
// failing assertion names what is in the file without printing a credential.
func dotenvKeys(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		keys = append(keys, strings.TrimSpace(strings.TrimPrefix(key, "export ")))
	}
	sort.Strings(keys)
	return keys
}

func hasKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

// A name the manifest declares sensitive goes to the vault without --secret, and
// the project's .env is not touched.
func TestSetVaultsADeclaredSensitiveName(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n")

	d, _, stderr := envDeps(t, dir, "s3cr3t\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}

	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "API_TOKEN") {
		t.Errorf("a declared-sensitive value was written to the plaintext .env (keys: %v)", keys)
	}
	if n := len(vaultFiles(t)); n != 1 {
		t.Errorf("vault holds %d entries, want exactly one", n)
	}
	if got := getJSON(t, dir, "API_TOKEN"); got.Source != vaultenv.SourceProject || got.Value != "s3cr3t" {
		t.Errorf("get resolved source %q (value matches: %t), want the project vault", got.Source, got.Value == "s3cr3t")
	}
	if !strings.Contains(stderr.String(), "declared sensitive") {
		t.Errorf("set should say why the value went to the vault; stderr: %q", stderr.String())
	}
}

// A declared connection is vaulted: every connection is by default, and a
// declared one is sensitive besides (envschema refuses saying otherwise).
func TestSetVaultsADeclaredConnection(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env.connections]\ndb_main = {}\n")

	d, _, _ := envDeps(t, dir, "postgres://u:p@h:5432/db\n")
	if err := execute(t, d, "local", "env", "connection", "set", "db_main"); err != nil {
		t.Fatal(err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "AIRFLOW_CONN_DB_MAIN") {
		t.Errorf("a declared connection was written to the plaintext .env (keys: %v)", keys)
	}
	if n := len(vaultFiles(t)); n != 1 {
		t.Errorf("vault holds %d entries, want exactly one", n)
	}
}

// Declared but not sensitive, and not declared at all: both keep the plaintext
// file, as before.
func TestSetKeepsNonSensitiveNamesInTheFile(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nLOG_LEVEL = {}\n")

	for _, name := range []string{"LOG_LEVEL", "UNDECLARED"} {
		d, _, _ := envDeps(t, dir, "v\n")
		if err := execute(t, d, "local", "env", "variable", "set", name); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
		if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, name) {
			t.Errorf("%s should be in the plaintext .env (keys: %v)", name, keys)
		}
	}
	if names := vaultFiles(t); len(names) != 0 {
		t.Errorf("non-sensitive sets reached the vault: %d entries", len(names))
	}
}

// A set that cannot read the declarations refuses rather than guessing "not
// sensitive", and writes nothing anywhere.
func TestSetRefusesWhenTheDeclarationsDoNotRead(t *testing.T) {
	for name, body := range map[string]string{
		"schema error": "[tool.astro.env]\nAPI_TOKEN = { sensitive = 'yes' }\n",
		"toml error":   "[tool.astro.env\nAPI_TOKEN = {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := secretEnvProject(t, body)

			d, _, _ := envDeps(t, dir, "s3cr3t\n")
			err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN")
			if err == nil {
				t.Fatal("want a refusal when the manifest does not read")
			}
			if !strings.Contains(err.Error(), "cannot tell whether") {
				t.Errorf("the error should say the declarations could not be read: %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(statErr) {
				t.Error("a refused set wrote the plaintext .env")
			}
			if n := len(vaultFiles(t)); n != 0 {
				t.Errorf("a refused set left %d vault entries", n)
			}

			// --secret does not need the declarations: the value is going to
			// the vault whatever they say.
			d, _, _ = envDeps(t, dir, "s3cr3t\n")
			if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN", "--secret"); err != nil {
				t.Errorf("set --secret should not depend on the manifest reading: %v", err)
			}
		})
	}
}

// --secret=false is an explicit request for the plaintext file, which a
// sensitive declaration rules out. Refused, with nothing written.
func TestSetRefusesPlaintextForADeclaredSensitiveName(t *testing.T) {
	cases := []struct {
		name, body, noun, arg, value, envKey string
	}{
		{"variable", "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n", "variable", "API_TOKEN", "s3cr3t", "API_TOKEN"},
		{"airflow variable", "[tool.astro.env.airflow_variables]\napi_key = { sensitive = true }\n", "airflow-variable", "api_key", "s3cr3t", "AIRFLOW_VAR_API_KEY"},
		{"connection", "[tool.astro.env.connections]\ndb_main = {}\n", "connection", "db_main", "postgres://u:p@h/db", "AIRFLOW_CONN_DB_MAIN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := secretEnvProject(t, tc.body)

			d, _, _ := envDeps(t, dir, tc.value+"\n")
			err := execute(t, d, "local", "env", tc.noun, "set", tc.arg, "--secret=false")
			if err == nil {
				t.Fatal("want --secret=false refused for a declared-sensitive name")
			}
			if !strings.Contains(err.Error(), "--secret=false") {
				t.Errorf("the refusal should name the flag that asked for plaintext: %v", err)
			}
			if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, tc.envKey) {
				t.Errorf("a refused plaintext write still reached .env (keys: %v)", keys)
			}
			if n := len(vaultFiles(t)); n != 0 {
				t.Errorf("a refused write left %d vault entries", n)
			}
		})
	}
}

// A plaintext copy written before the name was declared sensitive, or by hand,
// is removed by the next set, which vaults the value.
func TestSetRemovesThePlaintextCopyOfAPromotedName(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("KEEP=1\nAPI_TOKEN=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	d, _, stderr := envDeps(t, dir, "new\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	keys := dotenvKeys(t, filepath.Join(dir, ".env"))
	if hasKey(keys, "API_TOKEN") {
		t.Errorf("the plaintext copy survived a vaulted set (keys: %v)", keys)
	}
	if !hasKey(keys, "KEEP") {
		t.Errorf("removing the copy took an unrelated entry with it (keys: %v)", keys)
	}
	if got := getJSON(t, dir, "API_TOKEN"); got.Source != vaultenv.SourceProject {
		t.Errorf("get resolved from %q, want the project vault", got.Source)
	}
	if !strings.Contains(stderr.String(), "removed the other copy") {
		t.Errorf("set should say it removed the plaintext copy; stderr: %q", stderr.String())
	}
}

// One home per scope, in both directions, for names with no declaration too: a
// plain set removes the vault copy, and a --secret set removes the file copy.
// Otherwise a delete against one store leaves the other standing.
func TestSetKeepsOneHomePerScope(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "v1\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "v2\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if n := len(vaultFiles(t)); n != 0 {
		t.Errorf("a plain set left the vault copy behind: %d entries", n)
	}

	d, _, _ = envDeps(t, dir, "v3\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "TOKEN") {
		t.Errorf("a --secret set left the plaintext copy behind (keys: %v)", keys)
	}

	// With one copy, a delete leaves nothing.
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "TOKEN"); err == nil {
		t.Error("TOKEN still resolves after its only copy was deleted")
	}
}

// A plain set on a machine with no keyring still works: removing a vault copy
// deletes a file and needs no master key.
func TestPlainSetNeedsNoKeyring(t *testing.T) {
	dir := secretEnvProject(t, "")
	keyring.MockInitWithError(errors.New("no Secret Service available"))

	d, _, _ := envDeps(t, dir, "v\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN"); err != nil {
		t.Fatalf("a plain set must not depend on the keyring: %v", err)
	}
}

// A promoted write on a machine with no keyring refuses, says the declaration
// is why, and does not fall back to the plaintext file.
func TestPromotedSetRefusesWithoutAKeyring(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n")
	keyring.MockInitWithError(errors.New("no Secret Service available"))

	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN")
	if err == nil {
		t.Fatal("want a refusal when the keyring is unreachable")
	}
	if !strings.Contains(err.Error(), "declared sensitive") {
		t.Errorf("the error should say the declaration sent it to the vault: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(statErr) {
		t.Error("a refused promoted set fell back to the plaintext file")
	}
}

// delete without --secret removes a declared-sensitive name from the vault,
// along with any plaintext copy in the same scope.
func TestDeleteOfADeclaredSensitiveNameClearsBothStores(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n")

	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	if err := execute(t, d, "local", "env", "variable", "set", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	// A copy that got into the file by hand.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_TOKEN=hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if n := len(vaultFiles(t)); n != 0 {
		t.Errorf("delete left %d vault entries", n)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "API_TOKEN") {
		t.Errorf("delete left the plaintext copy (keys: %v)", keys)
	}
}

// delete --secret=false is how a stale plaintext copy of a sensitive name is
// removed on its own, so it is not refused the way a plaintext set is.
func TestDeletePlaintextCopyOfASensitiveNameIsAllowed(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = true }\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_TOKEN=hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "API_TOKEN", "--secret=false"); err != nil {
		t.Fatal(err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "API_TOKEN") {
		t.Errorf("delete --secret=false left the plaintext copy (keys: %v)", keys)
	}
}
