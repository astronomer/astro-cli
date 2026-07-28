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
			"FROM_SHELL":   {}, // shell beats both files
			"FROM_PROJECT": {}, // project beats global
			"FROM_GLOBAL":  {},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {}, // via AIRFLOW_VAR_*
		},
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {},
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

// A manifest default sits at the bottom of the chain: any file value beats it,
// and it resolves a name nothing else supplies.
func TestResolveDefaultAtBottom(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"LOG_LEVEL":  {Default: "info", HasDefault: true}, // nothing else holds it: default wins
			"OVERRIDDEN": {Default: "fallback", HasDefault: true},
			"EMPTY_DEF":  {Default: "", HasDefault: true}, // an empty-string default still resolves
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Default: "500", HasDefault: true}, // default injected under AIRFLOW_VAR_*
		},
	}
	project := mapProvider{label: "project", vals: map[string]string{
		"OVERRIDDEN": "from-file",
	}}
	res, err := Resolve(Inputs{Schema: schema, Providers: []Provider{project}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("unexpected missing: %+v", res.Missing)
	}
	if got := res.Values.EnvVars["LOG_LEVEL"]; got != "info" {
		t.Fatalf("LOG_LEVEL = %q, want the default", got)
	}
	if got := res.Values.EnvVars["OVERRIDDEN"]; got != "from-file" {
		t.Fatalf("OVERRIDDEN = %q, want the file value to beat the default", got)
	}
	if got, ok := res.Values.EnvVars["EMPTY_DEF"]; !ok || got != "" {
		t.Fatalf("EMPTY_DEF = %q (present=%v), want an empty-string default that resolves", got, ok)
	}
	if got := res.Values.AirflowVariables["batch_size"]; got != "500" {
		t.Fatalf("batch_size = %q, want the default", got)
	}
	// A default is not on disk, so it is injected for start; a file value is not.
	if got := res.Injected["LOG_LEVEL"]; got != "info" {
		t.Fatalf("Injected[LOG_LEVEL] = %q, want the default layered in", got)
	}
	if _, ok := res.Injected["OVERRIDDEN"]; ok {
		t.Fatalf("Injected holds OVERRIDDEN, but a file already supplies it")
	}
	if got := res.Injected["AIRFLOW_VAR_BATCH_SIZE"]; got != "500" {
		t.Fatalf("Injected[AIRFLOW_VAR_BATCH_SIZE] = %q, want the default under its env key", got)
	}
	bySource := map[string]string{}
	for _, rn := range res.Resolved {
		bySource[rn.Name] = rn.Source
	}
	if bySource["LOG_LEVEL"] != SourceDefault || bySource["batch_size"] != SourceDefault {
		t.Fatalf("default source labels = %v, want %q", bySource, SourceDefault)
	}
	if bySource["OVERRIDDEN"] != "project" {
		t.Fatalf("OVERRIDDEN source = %q, want project", bySource["OVERRIDDEN"])
	}
}

// A blank declaration with nothing to resolve it is missing.
func TestResolveMissing(t *testing.T) {
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"NEEDED": {}, // blank: required, no default
		},
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {},
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
	if res.Missing[0].EnvKey != "AIRFLOW_CONN_WAREHOUSE" {
		t.Errorf("connection missing: %+v", res.Missing[0])
	}
	if res.Missing[1].Name != "NEEDED" || res.Missing[1].EnvKey != "NEEDED" {
		t.Errorf("env missing: %+v", res.Missing[1])
	}
}

func TestResolveCorruptConnection(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {},
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
			"TOKEN": {Source: envschema.SourceWorkspace},
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
