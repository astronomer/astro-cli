package local

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/localenv"
)

// Connection ids and Airflow variable keys are upper-cased into their env-var
// key, so "region" and "REGION" are one AIRFLOW_VAR_REGION to Airflow and to
// the plain file, but two names to the vault. Every vault operation matches on
// the env-var key, so the two spellings are one value in both stores.
var caseFoldCases = []struct {
	noun, upper, lower, envKey, value string
}{
	{"airflow-variable", "REGION", "region", "AIRFLOW_VAR_REGION", "us-east-1"},
	{"connection", "DB", "db", "AIRFLOW_CONN_DB", "postgres://u:p@h/db"},
}

// A plaintext set removes a vaulted copy stored under the other spelling.
func TestPlainSetRemovesTheVaultCopyUnderAnotherSpelling(t *testing.T) {
	for _, tc := range caseFoldCases {
		t.Run(tc.noun, func(t *testing.T) {
			dir := secretEnvProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.upper, "--value", tc.value); err != nil {
				t.Fatal(err)
			}
			d, _, _ = envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.lower, "--value", tc.value, "--plain", "--replace-secret"); err != nil {
				t.Fatal(err)
			}
			if n := len(vaultFiles(t)); n != 0 {
				t.Errorf("the vaulted %s survived a plaintext set of %s: %d entries", tc.upper, tc.lower, n)
			}
			if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, tc.envKey) {
				t.Errorf("the plaintext set is missing from .env (keys: %v)", keys)
			}
		})
	}
}

// Two spellings set into the vault leave one entry, holding the later value,
// and get under either spelling returns it.
func TestVaultSetReplacesTheOtherSpelling(t *testing.T) {
	for _, tc := range caseFoldCases {
		t.Run(tc.noun, func(t *testing.T) {
			dir := secretEnvProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.lower, "--value", tc.value); err != nil {
				t.Fatal(err)
			}
			later := tc.value + "-later"
			if tc.noun == localenv.Noun(localenv.KindConn) {
				later = "postgres://u:p@later/db"
			}
			d, _, _ = envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.upper, "--value", later); err != nil {
				t.Fatal(err)
			}
			if n := len(vaultFiles(t)); n != 1 {
				t.Errorf("vault holds %d entries for one env key, want 1", n)
			}
			for _, spelling := range []string{tc.lower, tc.upper} {
				d, out, _ := envDeps(t, dir, "")
				if err := execute(t, d, "local", "env", tc.noun, "get", spelling); err != nil {
					t.Fatalf("get %s: %v", spelling, err)
				}
				if !strings.Contains(out.String(), "later") {
					t.Errorf("get %s did not return the later value", spelling)
				}
			}
		})
	}
}

// delete under one spelling removes the value stored under the other.
func TestDeleteMatchesTheOtherSpelling(t *testing.T) {
	for _, tc := range caseFoldCases {
		t.Run(tc.noun, func(t *testing.T) {
			dir := secretEnvProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.upper, "--value", tc.value); err != nil {
				t.Fatal(err)
			}
			d, _, _ = envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "delete", tc.lower); err != nil {
				t.Fatalf("delete %s: %v", tc.lower, err)
			}
			if n := len(vaultFiles(t)); n != 0 {
				t.Errorf("delete left %d vault entries", n)
			}
		})
	}
}
