package localenv

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// fakeWorkspace is a test Environment Manager provider.
type fakeWorkspace struct {
	label string
	vals  map[string]string
}

func (f fakeWorkspace) Lookup(k string) (string, bool) { v, ok := f.vals[k]; return v, ok }
func (f fakeWorkspace) Label() string                  { return f.label }

func workspaceListSchema() *envschema.Schema {
	return &envschema.Schema{EnvVars: map[string]envschema.ValueSpec{
		"DATA_WAREHOUSE_URI": {Required: true, Source: envschema.SourceWorkspace},
	}}
}

// list labels a workspace-source name with the workspace source when it answers.
func TestListLabelsAvailableWorkspaceSource(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir()) // empty global file
	projDir := t.TempDir()

	wp := fakeWorkspace{label: "workspace", vals: map[string]string{"DATA_WAREHOUSE_URI": "postgres://cloud"}}
	items, err := List(nil, projDir, workspaceListSchema(), ListOptions{WorkspaceProvider: wp})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != "workspace" {
		t.Fatalf("items = %+v, want one row sourced workspace", items)
	}
}

// When the workspace is unavailable, list shows the reason and never a value.
func TestListLabelsUnavailableWorkspaceSource(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir())
	projDir := t.TempDir()

	wp := fakeWorkspace{label: "workspace (unavailable: logged out)", vals: map[string]string{}}
	items, err := List(nil, projDir, workspaceListSchema(), ListOptions{WorkspaceProvider: wp})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != "workspace (unavailable: logged out)" {
		t.Fatalf("items = %+v, want the unavailable label", items)
	}
}

// A local file value overrides the workspace source, so list reports the file
// source, not the workspace.
func TestListLocalOverridesWorkspaceSource(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir())
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("DATA_WAREHOUSE_URI=postgres://local\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wp := fakeWorkspace{label: "workspace", vals: map[string]string{"DATA_WAREHOUSE_URI": "postgres://cloud"}}
	items, err := List(nil, projDir, workspaceListSchema(), ListOptions{WorkspaceProvider: wp})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != SourceProject {
		t.Fatalf("items = %+v, want the project source to win", items)
	}
}

var _ envresolve.Provider = fakeWorkspace{}
