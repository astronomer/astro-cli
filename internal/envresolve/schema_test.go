package envresolve

import (
	"errors"
	"reflect"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// decodeEnv parses a [tool.astro.env] TOML body the way pkg/manifest hands
// it over: decoded, untyped plain data.
func decodeEnv(t *testing.T, body string) map[string]any {
	t.Helper()
	var f struct {
		Tool struct {
			Astro struct {
				Env map[string]any `toml:"env"`
			} `toml:"astro"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal([]byte(body), &f); err != nil {
		t.Fatal(err)
	}
	return f.Tool.Astro.Env
}

func TestParseSchemaFull(t *testing.T) {
	env := decodeEnv(t, `
[tool.astro.env]
LOG_LEVEL = 'info'
EMPTY_DEFAULT = ''
API_TOKEN = { source = 'workspace' }
WAREHOUSE_URI = {}

[tool.astro.env.connections]
warehouse = {}
reporting = { source = 'workspace' }

[tool.astro.env.airflow_variables]
batch_size = '500'
`)
	got, err := ParseSchema(env)
	if err != nil {
		t.Fatal(err)
	}
	want := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"LOG_LEVEL":     {Default: "info", HasDefault: true},
			"EMPTY_DEFAULT": {Default: "", HasDefault: true}, // '' is a real default, not a marker
			"API_TOKEN":     {Source: envschema.SourceWorkspace},
			"WAREHOUSE_URI": {}, // {} is required, no default
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Default: "500", HasDefault: true},
		},
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {},
			"reporting": {Source: envschema.SourceWorkspace},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseSchema mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseSchemaEmpty(t *testing.T) {
	for _, env := range []map[string]any{nil, {}} {
		got, err := ParseSchema(env)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, &envschema.Schema{}) {
			t.Errorf("ParseSchema(%v) = %+v, want empty schema", env, got)
		}
	}
}

func TestParseSchemaProblems(t *testing.T) {
	env := decodeEnv(t, `
[tool.astro.env]
"BAD-NAME" = {}                       # not an env-var name
"1LEADING" = {}                       # env vars can't start with a digit
BADSRC = { source = 'cloud' }         # unknown source
EMPTYSRC = { source = '' }            # empty source rejected
TYPO = { typo = true }                # unknown table field
NOTVALUE = 5                          # not a string or table

[tool.astro.env.airflow_variables]
"1ok" = {}                            # AIRFLOW_VAR_1OK is legal

[tool.astro.env.connections]
"my.conn" = {}                        # not a conn id
`)
	_, err := ParseSchema(env)
	var se *SchemaError
	if !errors.As(err, &se) {
		t.Fatalf("want *SchemaError, got %v", err)
	}
	wantKeys := []string{
		"tool.astro.env.1LEADING",
		"tool.astro.env.BAD-NAME",
		"tool.astro.env.BADSRC.source",
		"tool.astro.env.EMPTYSRC.source",
		"tool.astro.env.NOTVALUE",
		"tool.astro.env.TYPO.typo",
		"tool.astro.env.connections.my.conn",
	}
	var gotKeys []string
	for _, p := range se.Problems {
		gotKeys = append(gotKeys, p.Key)
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("problem keys mismatch\n got: %v\nwant: %v", gotKeys, wantKeys)
	}
}

func TestParseSchemaNonTableSection(t *testing.T) {
	_, err := ParseSchema(map[string]any{"connections": "nope"})
	var se *SchemaError
	if !errors.As(err, &se) || len(se.Problems) != 1 || se.Problems[0].Reason != "expected a table" {
		t.Fatalf("want one 'expected a table' problem, got %v", err)
	}
}
