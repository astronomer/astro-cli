package local

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/vaultenv"
)

// vaultDefaultCases are the two kinds stored in the vault by default, each with
// a value and the env key a plaintext copy would carry.
var vaultDefaultCases = []struct {
	noun, name, value, envKey string
}{
	{"connection", "db_main", "postgres://u:p@h:5432/db", "AIRFLOW_CONN_DB_MAIN"},
	{"airflow-variable", "region", "us-east-1", "AIRFLOW_VAR_REGION"},
}

// A connection or Airflow variable set without --secret goes to the vault even
// when the manifest does not declare it, as in Astro Desktop.
func TestSetVaultsConnectionsAndAirflowVariablesByDefault(t *testing.T) {
	for _, tc := range vaultDefaultCases {
		t.Run(tc.noun, func(t *testing.T) {
			dir := secretEnvProject(t, "")

			d, out, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.name, "--value", tc.value, "--output", "json"); err != nil {
				t.Fatal(err)
			}
			if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, tc.envKey) {
				t.Errorf("an undeclared %s was written to the plaintext .env (keys: %v)", tc.noun, keys)
			}
			if n := len(vaultFiles(t)); n != 1 {
				t.Errorf("vault holds %d entries, want exactly one", n)
			}
			var res envResult
			if err := json.Unmarshal(out.Bytes(), &res); err != nil {
				t.Fatalf("decode the set result: %v", err)
			}
			if string(res.Scope) != vaultenv.SourceProject {
				t.Errorf("set reported scope %q, want %q", res.Scope, vaultenv.SourceProject)
			}
		})
	}
}

// Plain env vars are not part of the default: undeclared, they stay in .env.
func TestSetKeepsAnUndeclaredEnvVarInTheFile(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "v\n")
	if err := execute(t, d, "local", "env", "variable", "set", "LOG_LEVEL"); err != nil {
		t.Fatal(err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, "LOG_LEVEL") {
		t.Errorf("LOG_LEVEL should be in the plaintext .env (keys: %v)", keys)
	}
	if n := len(vaultFiles(t)); n != 0 {
		t.Errorf("an undeclared env var reached the vault: %d entries", n)
	}
}

// --secret=false is the escape hatch: it keeps an undeclared connection or
// Airflow variable, or a declared one that is not sensitive, in the plain file.
func TestSecretFalseKeepsANonSensitiveConnOrVarInTheFile(t *testing.T) {
	for _, tc := range vaultDefaultCases {
		t.Run(tc.noun, func(t *testing.T) {
			dir := secretEnvProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.name, "--value", tc.value, "--secret=false"); err != nil {
				t.Fatal(err)
			}
			if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, tc.envKey) {
				t.Errorf("--secret=false should keep it in .env (keys: %v)", keys)
			}
			if n := len(vaultFiles(t)); n != 0 {
				t.Errorf("--secret=false still reached the vault: %d entries", n)
			}
		})
	}
	t.Run("declared, not sensitive", func(t *testing.T) {
		dir := secretEnvProject(t, "[tool.astro.env.airflow_variables]\nregion = {}\n")
		d, _, _ := envDeps(t, dir, "")
		if err := execute(t, d, "local", "env", "airflow-variable", "set", "region", "--value", "us-east-1", "--secret=false"); err != nil {
			t.Fatal(err)
		}
		if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, "AIRFLOW_VAR_REGION") {
			t.Errorf("a declared, non-sensitive variable should be allowed in .env (keys: %v)", keys)
		}
	})
}

// The default needs no manifest, so a connection set without a flag works in a
// project whose manifest does not parse. --secret=false does depend on the
// declarations, so it is refused there.
func TestConnDefaultDoesNotReadTheManifest(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env]\nAPI_TOKEN = { sensitive = 'yes' }\n")

	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db_main", "--value", "postgres://u:p@h/db"); err != nil {
		t.Errorf("a vault-by-default set should not depend on the manifest: %v", err)
	}
	d, _, _ = envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "connection", "set", "other", "--value", "postgres://u:p@h/db", "--secret=false")
	if err == nil || !strings.Contains(err.Error(), "cannot tell whether") {
		t.Errorf("--secret=false needs the declarations, so it should be refused here: %v", err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "AIRFLOW_CONN_OTHER") {
		t.Errorf("a refused set wrote .env (keys: %v)", keys)
	}
}

// With no keyring the default refuses, points at --secret=false, and writes no
// plaintext. --secret=false then works.
func TestConnDefaultWithoutAKeyringNamesTheEscapeHatch(t *testing.T) {
	dir := secretEnvProject(t, "")
	keyring.MockInitWithError(errors.New("no Secret Service available"))

	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "connection", "set", "db_main", "--value", "postgres://u:p@h/db")
	if err == nil {
		t.Fatal("want a refusal when the keyring is unreachable")
	}
	if !strings.Contains(err.Error(), "--secret=false") {
		t.Errorf("the refusal should name --secret=false as the way to a plain file: %v", err)
	}
	if !strings.Contains(err.Error(), "by default") {
		t.Errorf("the refusal should say the vault was the default: %v", err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "AIRFLOW_CONN_DB_MAIN") {
		t.Errorf("a refused set fell back to .env (keys: %v)", keys)
	}

	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db_main", "--value", "postgres://u:p@h/db", "--secret=false"); err != nil {
		t.Errorf("--secret=false should work without a keyring: %v", err)
	}
}

// delete with no --secret flag clears the name from both stores in the scope,
// whatever the kind, as Astro Desktop's delete does.
func TestDeleteWithoutAFlagClearsBothStores(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "v\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--secret"); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "TOKEN"); err != nil {
		t.Fatalf("a plain delete should find the vaulted copy: %v", err)
	}
	if n := len(vaultFiles(t)); n != 0 {
		t.Errorf("delete left %d vault entries", n)
	}

	d, _, _ = envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "delete", "TOKEN")
	if err == nil || !strings.Contains(err.Error(), "or the vault") {
		t.Errorf("deleting what neither store holds should say both were checked: %v", err)
	}
}
