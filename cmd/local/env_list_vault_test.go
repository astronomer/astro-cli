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
		if err := execute(t, d, "local", "env", "connection", "set", name, "--value", "postgres://u:p@h/db", "--secret"); err != nil {
			t.Fatal(err)
		}
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
	if got[0].RemoveHint != "astro local env connection delete db_main --project --secret" {
		t.Errorf("remove hint = %q", got[0].RemoveHint)
	}
	if d := rows["declared"]; len(d) != 1 || d[0].Orphan {
		t.Errorf("declared rows = %+v, want one declared row", d)
	}

	// --project names the dotenv file, so the vault orphan is not in it.
	d, out, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--project", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `"db_main"`) {
		t.Errorf("list --project showed a vault entry:\n%s", out.String())
	}
}
