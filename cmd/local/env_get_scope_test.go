package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/vaultenv"
)

// getScopedJSON runs `<noun> get <name> <flags> --output json` and decodes it.
func getScopedJSON(t *testing.T, dir, noun, name string, flags ...string) (envValue, error) {
	t.Helper()
	d, out, _ := envDeps(t, dir, "")
	args := append([]string{"local", "env", noun, "get", name}, flags...)
	if err := execute(t, d, append(args, "--output", "json")...); err != nil {
		return envValue{}, err
	}
	var v envValue
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("decode the get result: %v", err)
	}
	return v, nil
}

// A get with the scope flag a set used finds the value, whichever store the set
// routed it to: a scope flag covers the scope's plain file and its vault.
func TestGetWithTheSetsScopeFlagFindsTheValue(t *testing.T) {
	cases := []struct {
		noun, name, value string
		project, global   string // the source get should report at each scope
	}{
		{"connection", "db_main", "postgres://u:p@h:5432/db", vaultenv.SourceProject, vaultenv.SourceGlobal},
		{"airflow-variable", "region", "us-east-1", vaultenv.SourceProject, vaultenv.SourceGlobal},
		{"variable", "LOG_LEVEL", "info", vaultenv.SourceProject, vaultenv.SourceGlobal},
	}
	for _, tc := range cases {
		for _, flag := range []string{"--project", "--global"} {
			t.Run(tc.noun+" "+flag, func(t *testing.T) {
				dir := secretEnvProject(t, "")
				d, _, _ := envDeps(t, dir, "")
				if err := execute(t, d, "local", "env", tc.noun, "set", tc.name, "--value", tc.value, flag); err != nil {
					t.Fatal(err)
				}
				got, err := getScopedJSON(t, dir, tc.noun, tc.name, flag)
				if err != nil {
					t.Fatalf("get %s: %v", flag, err)
				}
				want := tc.project
				if flag == "--global" {
					want = tc.global
				}
				if got.Source != want {
					t.Errorf("source = %q, want %q", got.Source, want)
				}
				if tc.noun != localenv.Noun(localenv.KindConn) && got.Value != tc.value {
					t.Errorf("get returned a different value than was set (matches: %t)", got.Value == tc.value)
				}
				if tc.noun == localenv.Noun(localenv.KindConn) && !strings.Contains(got.Value, `"conn_type":"postgres"`) {
					t.Error("get did not return the stored connection")
				}
			})
		}
	}
}

// --plain reads only the plain file; with no flag a get reads both stores.
func TestGetSecretFlagPicksOneStore(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "db_main", "--value", "postgres://u:p@h/db"); err != nil {
		t.Fatal(err)
	}
	if _, err := getScopedJSON(t, dir, "connection", "db_main", "--project", "--plain"); err == nil {
		t.Error("get --plain found a value that is only in the vault")
	}
	if got, err := getScopedJSON(t, dir, "connection", "db_main"); err != nil || got.Source != vaultenv.SourceProject {
		t.Errorf("get = %q, %v; want the project vault", got.Source, err)
	}

	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "LOG_LEVEL", "--value", "info", "--plain"); err != nil {
		t.Fatal(err)
	}
	if got, err := getScopedJSON(t, dir, "variable", "LOG_LEVEL", "--plain"); err != nil || got.Source != string(localenv.ScopeProject) {
		t.Errorf("get --plain = %q, %v; want the plain file", got.Source, err)
	}
}

// When both stores in a scope hold a copy, get returns the one the chain uses
// (the .env in the project scope, the vault in the global one) and says there
// is another.
func TestGetWithBothCopiesPicksTheChainsWinner(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--value", "v"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, out, stderr := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "get", "TOKEN", "--project", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got envValue
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != string(localenv.ScopeProject) {
		t.Errorf("project scope: source = %q, want the .env, which outranks the project vault", got.Source)
	}
	if !strings.Contains(stderr.String(), "also set in") {
		t.Errorf("get should say there is another copy; stderr: %q", stderr.String())
	}

	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "SHARED", "--value", "v", "--global"); err != nil {
		t.Fatal(err)
	}
	globalPath, err := localenv.GlobalEnvPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, []byte("SHARED=hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := getScopedJSON(t, dir, "variable", "SHARED", "--global")
	if err != nil {
		t.Fatal(err)
	}
	if g.Source != vaultenv.SourceGlobal {
		t.Errorf("global scope: source = %q, want the global vault, which outranks ~/.astro/env", g.Source)
	}
}
