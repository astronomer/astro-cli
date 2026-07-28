package envresolve

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// fakeWorkspace is a test workspace (Environment Manager) provider.
type fakeWorkspace struct {
	label string
	vals  map[string]string
	diag  string
}

func (f fakeWorkspace) Lookup(k string) (string, bool) { v, ok := f.vals[k]; return v, ok }
func (f fakeWorkspace) Label() string                  { return f.label }
func (f fakeWorkspace) Diagnose(string) string         { return f.diag }

func workspaceSchema() *envschema.Schema {
	return &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"DATA_WAREHOUSE_URI": {Required: true, Source: envschema.SourceWorkspace},
		},
	}
}

func TestWorkspaceSourceResolvesFromProvider(t *testing.T) {
	wp := fakeWorkspace{label: "workspace", vals: map[string]string{"DATA_WAREHOUSE_URI": "postgres://cloud"}}
	res, err := Resolve(Inputs{
		Schema:            workspaceSchema(),
		Providers:         []Provider{mapProvider{label: "shell", vals: map[string]string{}}},
		WorkspaceProvider: wp,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := res.Values.EnvVars["DATA_WAREHOUSE_URI"]; got != "postgres://cloud" {
		t.Fatalf("value = %q, want the cloud value", got)
	}
	if got := res.Injected["DATA_WAREHOUSE_URI"]; got != "postgres://cloud" {
		t.Fatalf("Injected = %q, want the cloud value layered into Airflow", got)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("Missing = %v, want none", res.Missing)
	}
	if res.Resolved[0].Source != "workspace" || !res.Resolved[0].Found {
		t.Fatalf("resolved source = %q found = %v, want workspace/true", res.Resolved[0].Source, res.Resolved[0].Found)
	}
}

// A local value overrides a workspace-source name: source is a default, not a
// lock, and the cloud value is never fetched into the injection.
func TestLocalBeatsWorkspace(t *testing.T) {
	wp := fakeWorkspace{label: "workspace", vals: map[string]string{"DATA_WAREHOUSE_URI": "postgres://cloud"}}
	res, err := Resolve(Inputs{
		Schema:            workspaceSchema(),
		Providers:         []Provider{mapProvider{label: "project", vals: map[string]string{"DATA_WAREHOUSE_URI": "postgres://local"}}},
		WorkspaceProvider: wp,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := res.Values.EnvVars["DATA_WAREHOUSE_URI"]; got != "postgres://local" {
		t.Fatalf("value = %q, want the local value to win", got)
	}
	if _, ok := res.Injected["DATA_WAREHOUSE_URI"]; ok {
		t.Fatalf("Injected holds the key, but a local file already supplies it")
	}
	if res.Resolved[0].Source != "project" {
		t.Fatalf("source = %q, want project", res.Resolved[0].Source)
	}
}

// An unavailable workspace labels the source with its reason and carries the
// cause into the missing-value report.
func TestUnavailableWorkspaceMissingNote(t *testing.T) {
	wp := fakeWorkspace{
		label: "workspace (unavailable: logged out)",
		vals:  map[string]string{},
		diag:  "you are not logged in — log in with 'astro login'",
	}
	res, err := Resolve(Inputs{
		Schema:            workspaceSchema(),
		Providers:         []Provider{mapProvider{label: "shell", vals: map[string]string{}}},
		WorkspaceProvider: wp,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Resolved[0].Source != "workspace (unavailable: logged out)" || res.Resolved[0].Found {
		t.Fatalf("source = %q found = %v", res.Resolved[0].Source, res.Resolved[0].Found)
	}
	if len(res.Missing) != 1 {
		t.Fatalf("Missing count = %d, want 1", len(res.Missing))
	}
	want := `source "workspace": you are not logged in — log in with 'astro login'`
	if res.Missing[0].SourceNote != want {
		t.Fatalf("SourceNote = %q, want %q", res.Missing[0].SourceNote, want)
	}
}
