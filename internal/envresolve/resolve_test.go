package envresolve

import (
	"reflect"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// mapProvider is a test provider: a labeled bag of env-var keys.
type mapProvider struct {
	label string
	vals  map[string]string
}

func (p mapProvider) Lookup(key string) (string, bool) { v, ok := p.vals[key]; return v, ok }
func (p mapProvider) Label() string                    { return p.label }

func connValue(t *testing.T, c connmodel.Connection) string {
	t.Helper()
	_, val, ok := airflowenv.EncodeConnEnv(c)
	if !ok {
		t.Fatalf("encode %+v", c)
	}
	return val
}

func TestResolveLayering(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"FROM_SHELL":   {Required: true}, // shell beats both files
			"FROM_PROJECT": {Required: true}, // project beats global
			"FROM_GLOBAL":  {Required: true},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Type: envschema.TypeInt, Required: true}, // via AIRFLOW_VAR_*
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true},
		},
	}
	shell := mapProvider{label: "shell", vals: map[string]string{
		"FROM_SHELL": "shell-wins",
	}}
	project := mapProvider{label: "project", vals: map[string]string{
		"FROM_SHELL":             "project-loses",
		"FROM_PROJECT":           "project-wins",
		"AIRFLOW_VAR_BATCH_SIZE": "100",
		"AIRFLOW_CONN_WAREHOUSE": connValue(t, connmodel.Connection{ConnID: "warehouse", ConnType: "postgres", ConnHost: "db"}),
	}}
	global := mapProvider{label: "global", vals: map[string]string{
		"FROM_PROJECT": "global-loses",
		"FROM_GLOBAL":  "global-wins",
	}}

	res, err := Resolve(Inputs{Schema: schema, Providers: []Provider{shell, project, global}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("unexpected missing: %+v", res.Missing)
	}
	wantVals := map[string]string{"FROM_SHELL": "shell-wins", "FROM_PROJECT": "project-wins", "FROM_GLOBAL": "global-wins"}
	if !reflect.DeepEqual(res.Values.EnvVars, wantVals) {
		t.Fatalf("env vars = %+v, want %+v", res.Values.EnvVars, wantVals)
	}
	if got := res.Values.AirflowVariables["batch_size"]; got != "100" {
		t.Fatalf("batch_size = %q, want 100", got)
	}
	if got := res.Values.Connections["warehouse"]; got != "postgres" {
		t.Fatalf("warehouse conn_type = %q, want postgres", got)
	}

	// Resolved reports the winning source for every declared name.
	wantSource := map[string]string{
		"FROM_SHELL": "shell", "FROM_PROJECT": "project", "FROM_GLOBAL": "global",
		"batch_size": "project", "warehouse": "project",
	}
	for _, rn := range res.Resolved {
		if !rn.Found {
			t.Errorf("%s %s: not found", rn.Section, rn.Name)
		}
		if rn.Source != wantSource[rn.Name] {
			t.Errorf("%s: source = %q, want %q", rn.Name, rn.Source, wantSource[rn.Name])
		}
	}
}

func TestResolveMissing(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"NEEDED":   {Required: true, Sensitive: true, Description: "a token"},
			"OPTIONAL": {},
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true},
		},
	}
	res, err := Resolve(Inputs{Schema: schema, Providers: nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 2 {
		t.Fatalf("missing = %+v, want 2", res.Missing)
	}
	// Sorted by section then name: connection warehouse, then env_var NEEDED.
	if res.Missing[0].Section != envschema.SectionConnection || res.Missing[0].Name != "warehouse" {
		t.Errorf("missing[0] = %+v", res.Missing[0])
	}
	if res.Missing[0].EnvKey != "AIRFLOW_CONN_WAREHOUSE" || !res.Missing[0].Sensitive {
		t.Errorf("connection missing: %+v", res.Missing[0])
	}
	if res.Missing[1].Name != "NEEDED" || res.Missing[1].EnvKey != "NEEDED" || !res.Missing[1].Sensitive {
		t.Errorf("env missing: %+v", res.Missing[1])
	}
}

func TestResolveCorruptConnection(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true},
		},
	}
	project := mapProvider{label: "project", vals: map[string]string{
		"AIRFLOW_CONN_WAREHOUSE": "not-json",
	}}
	res, err := Resolve(Inputs{Schema: schema, Providers: []Provider{project}})
	if err != nil {
		t.Fatal(err)
	}
	// Present but corrupt: a type violation, not a missing value.
	if len(res.Missing) != 0 {
		t.Fatalf("unexpected missing: %+v", res.Missing)
	}
	var found bool
	for _, v := range res.Violations {
		if v.Section == envschema.SectionConnection && v.Kind == envschema.ViolationWrongType {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a wrong-type violation, got %+v", res.Violations)
	}
}

// A workspace source with no WorkspaceProvider wired gates the required name
// as missing, with a note — never silently resolved from another place.
func TestResolveWorkspaceSourceNoProvider(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"TOKEN": {Required: true, Source: envschema.SourceWorkspace},
		},
	}
	res, err := Resolve(Inputs{Schema: schema})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0].Name != "TOKEN" {
		t.Fatalf("Missing = %+v, want TOKEN", res.Missing)
	}
	if res.Missing[0].SourceNote == "" {
		t.Fatalf("want a SourceNote naming why the workspace source was unavailable")
	}
}
