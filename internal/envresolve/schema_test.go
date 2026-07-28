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
[tool.astro.env.vars.API_URL]
type = "url"
required = true
description = "the upstream API"

[tool.astro.env.vars.MODE]
enum = ["dev", "prod"]
sensitive = false

[tool.astro.env.vars.API_TOKEN]
required = true
sensitive = true
source = "workspace"

[tool.astro.env.airflow_variables.batch_size]
type = "int"
required = true

[tool.astro.env.connections.warehouse]
conn_type = "postgres"
required = true
description = "the analytics DB"
source = "workspace"
`)
	got, err := ParseSchema(env)
	if err != nil {
		t.Fatal(err)
	}
	want := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"API_URL":   {Type: envschema.TypeURL, Required: true, Description: "the upstream API"},
			"MODE":      {Enum: []string{"dev", "prod"}},
			"API_TOKEN": {Required: true, Sensitive: true, Source: envschema.SourceWorkspace},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Type: envschema.TypeInt, Required: true},
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {
				ConnType: "postgres", Required: true, Description: "the analytics DB",
				Source: envschema.SourceWorkspace,
			},
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
[tool.astro.env.secrets]              # unknown section

[tool.astro.env.vars."BAD-NAME"]      # not an env-var name

[tool.astro.env.vars."1LEADING"]      # env vars can't start with a digit

[tool.astro.env.airflow_variables."1ok"] # but AIRFLOW_VAR_1OK is legal

[tool.astro.env.vars.TYPO]
typo = true                           # unknown field
type = "tensor"                       # unknown value type
required = "yes"                      # wrong field type
enum = [1, 2]                         # non-string enum values

[tool.astro.env.connections."my.conn"] # not a conn id

[tool.astro.env.vars.BADSRC]
source = "cloud"                      # unknown source

[tool.astro.env.vars.EMPTYSRC]
source = ""                           # empty source rejected (omit for local-only)
`)
	_, err := ParseSchema(env)
	var se *SchemaError
	if !errors.As(err, &se) {
		t.Fatalf("want *SchemaError, got %v", err)
	}
	wantKeys := []string{
		"tool.astro.env.connections.my.conn",
		"tool.astro.env.secrets",
		"tool.astro.env.vars.1LEADING",
		"tool.astro.env.vars.BAD-NAME",
		"tool.astro.env.vars.BADSRC.source",
		"tool.astro.env.vars.EMPTYSRC.source",
		"tool.astro.env.vars.TYPO.enum[0]",
		"tool.astro.env.vars.TYPO.enum[1]",
		"tool.astro.env.vars.TYPO.required",
		"tool.astro.env.vars.TYPO.type",
		"tool.astro.env.vars.TYPO.typo",
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
	_, err := ParseSchema(map[string]any{"vars": "nope"})
	var se *SchemaError
	if !errors.As(err, &se) || len(se.Problems) != 1 || se.Problems[0].Reason != "expected a table" {
		t.Fatalf("want one 'expected a table' problem, got %v", err)
	}
}
