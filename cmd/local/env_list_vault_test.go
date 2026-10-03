package local

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/vaultenv"
)

// An undeclared value held only in the vault is listed, as an orphan of the
// vault tier with a delete command, the way an undeclared file entry is. A
// declared one is a declared row, not an orphan too.
func TestListShowsUndeclaredVaultEntries(t *testing.T) {
	dir := secretEnvProject(t, "[tool.astro.env.connections]\ndeclared = {}\n")
	for _, name := range []string{"db_main", "declared"} {
		d, _, _ := envDeps(t, dir, "")
		if err := execute(t, d, "local", "env", "connection", "set", name, "--value", "postgres://u:p@h/db"); err != nil {
			t.Fatal(err)
		}
	}

	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "api_token", "--value", "s3cret"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	rows := map[string][]localenv.ListItem{}
	dec := json.NewDecoder(strings.NewReader(out.String()))
	for dec.More() {
		var it localenv.ListItem
		if err := dec.Decode(&it); err != nil {
			t.Fatalf("decode: %v", err)
		}
		rows[it.Name] = append(rows[it.Name], it)
	}
	got := rows["db_main"]
	if len(got) != 1 {
		t.Fatalf("db_main rows = %+v, want one", got)
	}
	if !got[0].Orphan || got[0].Source != vaultenv.SourceProject || got[0].Kind != localenv.KindConn {
		t.Errorf("db_main row = %+v, want a conn orphan sourced from the project vault", got[0])
	}
	if got[0].RemoveHint != "astro local env connection delete db_main --project" {
		t.Errorf("remove hint = %q", got[0].RemoveHint)
	}
	if got[0].DeclareHint != "astro local env connection declare db_main" {
		t.Errorf("declare hint = %q", got[0].DeclareHint)
	}
	if v := rows["api_token"]; len(v) != 1 || v[0].DeclareHint != "astro local env variable declare api_token --secret" {
		t.Errorf("api_token rows = %+v, want one with a --secret declare hint", v)
	}
	if d := rows["declared"]; len(d) != 1 || d[0].Orphan {
		t.Errorf("declared rows = %+v, want one declared row", d)
	}

	// --project covers the project's vault as well as its .env, since a set
	// stores there by default; --global covers neither.
	d, out, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--project", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"db_main"`) || !strings.Contains(out.String(), `"source":"vault"`) {
		t.Errorf("list --project left out the project vault:\n%s", out.String())
	}
	d, out, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--global", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `"db_main"`) {
		t.Errorf("list --global showed a project vault entry:\n%s", out.String())
	}
}

// list --global covers the global vault, where every global is stored now.
func TestListGlobalShowsTheGlobalVault(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "SHARED", "--value", "v", "--global", "--auto-link"); err != nil {
		t.Fatal(err)
	}
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--global", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"SHARED"`) || !strings.Contains(out.String(), `"source":"`+vaultenv.SourceGlobal+`"`) {
		t.Errorf("list --global left out the global vault:\n%s", out.String())
	}
}

// An undeclared value in the global vault that reaches the project is passed to
// it at start, so list marks it applied.
func TestListMarksUndeclaredGlobalVaultEntryApplied(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "connection", "set", "shared_db", "--value", "postgres://u:p@h/db", "--global", "--auto-link"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var row localenv.ListItem
	dec := json.NewDecoder(strings.NewReader(out.String()))
	for dec.More() {
		var it localenv.ListItem
		if err := dec.Decode(&it); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if it.Name == "shared_db" {
			row = it
		}
	}
	if row.Source != vaultenv.SourceGlobal || row.Applied == nil || !*row.Applied {
		t.Errorf("shared_db row = %+v, want a global vault row marked applied", row)
	}
	if row.DeclareHint != "astro local env connection declare shared_db" {
		t.Errorf("declare hint = %q", row.DeclareHint)
	}
}
