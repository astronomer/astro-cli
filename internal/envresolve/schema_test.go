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

[tool.astro.env.airflow_variables.batch_size]
type = "int"
required = true

[tool.astro.env.connections.warehouse]
conn_type = "postgres"
required = true
description = "the analytics DB"

[tool.astro.env.connections.warehouse.bindings.local]
source = "vault"

[tool.astro.env.connections.warehouse.bindings.prod]
source = "deployment"
deployment = "prod"
`)
	got, err := ParseSchema(env)
	if err != nil {
		t.Fatal(err)
	}
	want := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"API_URL": {Type: envschema.TypeURL, Required: true, Description: "the upstream API"},
			"MODE":    {Enum: []string{"dev", "prod"}},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"batch_size": {Type: envschema.TypeInt, Required: true},
		},
		Connections: map[string]envschema.ConnSpec{
			"warehouse": {
				ConnType: "postgres", Required: true, Description: "the analytics DB",
				Bindings: map[string]envschema.Binding{
					"local": {Source: envschema.SourceVault},
					"prod":  {Source: envschema.SourceDeployment, Deployment: "prod"},
				},
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

[tool.astro.env.connections.a.bindings.local]
source = "cloud"                      # unknown source

[tool.astro.env.connections.b.bindings.local]
deployment = "prod"                   # source missing

[tool.astro.env.connections.c.bindings.prod]
source = "deployment"                 # deployment name missing

[tool.astro.env.connections.d.bindings.local]
source = "vault"
deployment = "prod"                   # deployment is deployment-source only
`)
	_, err := ParseSchema(env)
	var se *SchemaError
	if !errors.As(err, &se) {
		t.Fatalf("want *SchemaError, got %v", err)
	}
	wantKeys := []string{
		"tool.astro.env.connections.a.bindings.local.source",
		"tool.astro.env.connections.b.bindings.local.source",
		"tool.astro.env.connections.c.bindings.prod.deployment",
		"tool.astro.env.connections.d.bindings.local.deployment",
		"tool.astro.env.connections.my.conn",
		"tool.astro.env.secrets",
		"tool.astro.env.vars.1LEADING",
		"tool.astro.env.vars.BAD-NAME",
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

func TestVaultKeyRoundTrip(t *testing.T) {
	cases := []struct {
		key               string
		kind, scope, name string
		ok                bool
	}{
		{EnvVaultKey("", "API_URL"), VaultKindEnv, "", "API_URL", true},
		{EnvVaultKey("/Users/x/proj", "AIRFLOW_VAR_BATCH"), VaultKindEnv, "/Users/x/proj", "AIRFLOW_VAR_BATCH", true},
		{ConnVaultKey("/Users/x/proj", "Warehouse"), VaultKindConn, "/Users/x/proj", "warehouse", true},
		// a scope path containing colons still parses from both ends
		{EnvVaultKey(`C:\Users\x`, "FOO"), VaultKindEnv, `C:\Users\x`, "FOO", true},
		{"auth:token", "", "", "", false}, // outside the namespace
		{"env:", "", "", "", false},       // no name
		{"env:/x:", "", "", "", false},    // empty name
		{"plain", "", "", "", false},
	}
	for _, tc := range cases {
		kind, scope, name, ok := ParseVaultKey(tc.key)
		if kind != tc.kind || scope != tc.scope || name != tc.name || ok != tc.ok {
			t.Errorf("ParseVaultKey(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				tc.key, kind, scope, name, ok, tc.kind, tc.scope, tc.name, tc.ok)
		}
	}
}
