package envresolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		// Sensitive without anyone saying so: a connection carries a credential
		// by construction, and the schema this grammar replaces treats them as
		// unconditionally sensitive.
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {Sensitive: true},
			"reporting": {Source: envschema.SourceWorkspace, Sensitive: true},
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

// The annotation grammar: what a table declaration may carry.
//
// D3 owed this and O19 gated the desktop's move onto it, because
// [tool.astro.env] could express a default and a source and nothing else —
// while the schema it has to replace carries type, sensitive, description and
// enum.
func TestParseSchemaAnnotations(t *testing.T) {
	env := decodeEnv(t, `
[tool.astro.env]
LOG_LEVEL = { default = 'info', type = 'enum', enum = ['debug', 'info', 'warn'], description = 'How much Airflow says' }
DB_PASSWORD = { sensitive = true, description = 'Warehouse password' }
SLACK_WEBHOOK = { optional = true, type = 'url' }
PORT = { type = 'port', default = 8080 }
DEBUG = { type = 'bool', default = false }

[tool.astro.env.connections]
warehouse = { conn_type = 'postgres' }
`)
	s, err := ParseSchema(env)
	if err != nil {
		t.Fatal(err)
	}

	logLevel := s.EnvVars["LOG_LEVEL"]
	if !logLevel.HasDefault || logLevel.Default != "info" {
		t.Errorf("default = %q/%v, want info/true — the table's default is the shorthand's default", logLevel.Default, logLevel.HasDefault)
	}
	if logLevel.Type != envschema.TypeEnum {
		t.Errorf("type = %q, want enum", logLevel.Type)
	}
	if !reflect.DeepEqual(logLevel.Enum, []string{"debug", "info", "warn"}) {
		t.Errorf("enum = %v, want the three declared values", logLevel.Enum)
	}
	if logLevel.Description != "How much Airflow says" {
		t.Errorf("description = %q", logLevel.Description)
	}

	if !s.EnvVars["DB_PASSWORD"].Sensitive {
		t.Error("sensitive did not decode")
	}
	if got := s.EnvVars["SLACK_WEBHOOK"]; !got.Optional || got.Type != envschema.TypeURL {
		t.Errorf("SLACK_WEBHOOK = %+v, want optional and url", got)
	}
	// Declaring a type invites writing the default in that type.
	if got := s.EnvVars["PORT"]; got.Default != "8080" || !got.HasDefault {
		t.Errorf("PORT default = %q/%v, want 8080/true", got.Default, got.HasDefault)
	}
	if got := s.EnvVars["DEBUG"]; got.Default != "false" || !got.HasDefault {
		t.Errorf("DEBUG default = %q/%v, want false/true", got.Default, got.HasDefault)
	}
	if got := s.Connections["warehouse"].ConnType; got != "postgres" {
		t.Errorf("conn_type = %q, want postgres", got)
	}
}

// A connection is sensitive whether or not anyone said so, in both spellings.
//
// The schema this replaces treats connections as unconditionally sensitive
// ("Connections are inherently credential-bearing"). A per-declaration opt-in
// would mean every connection that omitted the flag read as plaintext-safe,
// which is a silent downgrade of every connection in every converted project.
func TestConnectionsAreAlwaysSensitive(t *testing.T) {
	s, err := ParseSchema(decodeEnv(t, `
[tool.astro.env]
PLAIN = {}

[tool.astro.env.connections]
bare = {}
typed = { conn_type = 'postgres' }
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bare", "typed"} {
		if !s.Connections[name].Sensitive {
			t.Errorf("connection %q is not sensitive; a connection carries a credential by construction", name)
		}
	}
	if s.EnvVars["PLAIN"].Sensitive {
		t.Error("a plain env var must not become sensitive by default; that is what the flag is for")
	}
}

// The string shorthand is literally sugar for `{ default = ... }`.
func TestParseSchemaShorthandEqualsDefaultKey(t *testing.T) {
	for _, tc := range []struct{ name, short, long string }{
		{"a value", `LOG_LEVEL = 'info'`, `LOG_LEVEL = { default = 'info' }`},
		// The empty default is a real default, not a magic value.
		{"the empty default", `LOG_LEVEL = ''`, `LOG_LEVEL = { default = '' }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			short, err := ParseSchema(decodeEnv(t, "[tool.astro.env]\n"+tc.short+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			long, err := ParseSchema(decodeEnv(t, "[tool.astro.env]\n"+tc.long+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(short.EnvVars, long.EnvVars) {
				t.Errorf("shorthand %+v and table %+v disagree", short.EnvVars, long.EnvVars)
			}
		})
	}
}

// An optional declaration does not gate a run; every other declaration does.
//
// This is why it is a field rather than an empty default: a resolved default is
// INJECTED, so `X = \'\'` would set the variable to empty rather than leave it
// absent. See the package doc on pkg/envschema.
func TestOptionalDeclarationDoesNotGate(t *testing.T) {
	s, err := ParseSchema(decodeEnv(t, `
[tool.astro.env]
REQUIRED_ONE = {}
OPTIONAL_ONE = { optional = true }
`))
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, v := range envschema.Validate(s, envschema.Values{}) {
		if v.Kind == envschema.ViolationMissing {
			missing = append(missing, v.Key)
		}
	}
	if !reflect.DeepEqual(missing, []string{"REQUIRED_ONE"}) {
		t.Errorf("missing = %v, want only REQUIRED_ONE", missing)
	}
}

// Optional alongside a default is allowed, and has to be.
//
// The first version refused it, on the grounds that the default already
// satisfies the run gate so optional says nothing. True of the gate and false of
// everything else: the schema this grammar exists to absorb carries an
// independent Required flag whose zero value is optional, so optional-with-a-
// default is its ordinary shape — its own fixture has a defaulted BATCH_SIZE
// that is not required. Refusing it left no spelling for that declaration at
// all, which would have blocked the migration this PR is for.
func TestOptionalWithDefaultIsAllowed(t *testing.T) {
	s, err := ParseSchema(decodeEnv(t, `
[tool.astro.env]
BATCH_SIZE = { type = 'int', default = '100', optional = true }
`))
	if err != nil {
		t.Fatalf("optional with a default must be expressible: %v", err)
	}
	got := s.EnvVars["BATCH_SIZE"]
	if !got.Optional || !got.HasDefault || got.Default != "100" {
		t.Errorf("BATCH_SIZE = %+v, want optional with the default kept", got)
	}
}

// A sensitive value may not carry a default.
//
// This is the hole the first version of these checks left. A default is
// committed to the manifest and injected at start — resolve.go puts it in the
// injected set, which becomes Plan.Env, which docker mode writes into the
// compose file on disk. So `{ sensitive = true, default = 'hunter2' }` put the
// credential in git AND on disk, from the one annotation whose purpose is
// keeping it out of both.
func TestSensitiveMustNotCarryADefault(t *testing.T) {
	for _, tc := range []struct{ name, body, wantKey string }{
		{
			name:    "declared sensitive",
			body:    "[tool.astro.env]\nDB_PASSWORD = { sensitive = true, default = 'hunter2' }\n",
			wantKey: "tool.astro.env.DB_PASSWORD.default",
		},
		{
			// Never typed `sensitive`, but a connection always is.
			name:    "a connection, which is sensitive without saying so",
			body:    "[tool.astro.env.connections]\nwarehouse = { default = 'postgres://u:p@h/db' }\n",
			wantKey: "tool.astro.env.connections.warehouse.default",
		},
		{
			// The shorthand cannot say `sensitive`, and must not be a way around it.
			name:    "a connection declared with the string shorthand",
			body:    "[tool.astro.env.connections]\nwarehouse = 'postgres://u:p@h/db'\n",
			wantKey: "tool.astro.env.connections.warehouse.default",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema(decodeEnv(t, tc.body))
			var se *SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("want a refusal, got %v", err)
			}
			found := false
			for _, pr := range se.Problems {
				if pr.Key == tc.wantKey {
					found = true
				}
			}
			if !found {
				t.Errorf("want a problem at %s, got %+v", tc.wantKey, se.Problems)
			}
		})
	}
}

// What the grammar refuses, asserted as the EXACT problem set.
//
// Exact, not a membership check: the first version asserted only that each
// expected key appeared, which let a spurious extra problem ship green — a
// failed field decode left a zero value that the coherence rules then reported
// as a second, contradictory problem. A strict-parse grammar whose promise is
// naming exactly what to fix has to be tested for what it does NOT say too.
func TestParseSchemaAnnotationProblems(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{
			name: "unknown field is still refused",
			body: "TYPO = { sensitiv = true }",
			want: []string{"tool.astro.env.TYPO.sensitiv"},
		},
		{
			name: "unknown type",
			body: "BADTYPE = { type = 'integer' }",
			want: []string{"tool.astro.env.BADTYPE.type"},
		},
		{
			name: "non-bool sensitive",
			body: "BADBOOL = { sensitive = 'yes' }",
			want: []string{"tool.astro.env.BADBOOL.sensitive"},
		},
		{
			// One mistake, one problem: the enum IS non-empty, so the coherence
			// rule must not also claim it is missing.
			name: "a bad enum element names only that element",
			body: "BADENUM = { type = 'enum', enum = ['a', 2] }",
			want: []string{"tool.astro.env.BADENUM.enum[001]"},
		},
		{
			// Every bad element, not just the first.
			name: "several bad enum elements",
			body: "MANYBAD = { type = 'enum', enum = ['a', 2, 3] }",
			want: []string{"tool.astro.env.MANYBAD.enum[001]", "tool.astro.env.MANYBAD.enum[002]"},
		},
		{
			// A bad type must not also produce "enum needs type = enum".
			name: "a bad type names only the type",
			body: "BADTYPE2 = { type = 'enom', enum = ['a', 'b'] }",
			want: []string{"tool.astro.env.BADTYPE2.type"},
		},
		{
			name: "enum without type = enum",
			body: "LONELY = { enum = ['a'] }",
			want: []string{"tool.astro.env.LONELY.enum"},
		},
		{
			name: "type = enum with no values",
			body: "EMPTY = { type = 'enum' }",
			want: []string{"tool.astro.env.EMPTY.type"},
		},
		{
			name: "conn_type on a plain env var",
			body: "CONNKEY = { conn_type = 'postgres' }",
			want: []string{"tool.astro.env.CONNKEY.conn_type"},
		},
		{
			name: "a default that is not a scalar",
			body: "BADDEF = { default = ['a'] }",
			want: []string{"tool.astro.env.BADDEF.default"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema(decodeEnv(t, "[tool.astro.env]\n"+tc.body+"\n"))
			var se *SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("want *SchemaError, got %v", err)
			}
			var got []string
			for _, p := range se.Problems {
				got = append(got, p.Key)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("problems = %v, want exactly %v", got, tc.want)
			}
		})
	}
}

// Section-specific refusals, which need their own sections to express.
func TestParseSchemaSectionProblems(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{
			name: "conn_type on an airflow variable",
			body: "[tool.astro.env.airflow_variables]\nv = { conn_type = 'postgres' }\n",
			want: []string{"tool.astro.env.airflow_variables.v.conn_type"},
		},
		{
			// Redundant at best, an attempt to turn it off at worst, and the
			// second must not look like it worked.
			name: "sensitive on a connection",
			body: "[tool.astro.env.connections]\nc = { sensitive = false }\n",
			want: []string{"tool.astro.env.connections.c.sensitive"},
		},
		{
			name: "an empty conn_type",
			body: "[tool.astro.env.connections]\nc = { conn_type = '' }\n",
			want: []string{"tool.astro.env.connections.c.conn_type"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema(decodeEnv(t, tc.body))
			var se *SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("want *SchemaError, got %v", err)
			}
			var got []string
			for _, p := range se.Problems {
				got = append(got, p.Key)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("problems = %v, want exactly %v", got, tc.want)
			}
		})
	}
}

// Every TOML block in the manifest reference parses.
//
// A doc example that does not load is worse than no example: it is the thing a
// user copy-pastes, and the last round of this grammar shipped one — the
// reference's own annotated connection carried `sensitive = true`, which the
// same commit made an error. Nothing connected the docs to the parser, so it was
// invisible until someone read both.
//
// Only the [tool.astro.env] blocks are extracted, since that is the section this
// package parses; a block naming any other table is skipped rather than
// half-parsed.
func TestManifestReferenceExamplesParse(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest-reference.md"))
	if err != nil {
		t.Fatal(err)
	}

	blocks := 0
	for _, block := range strings.Split(string(body), "```toml") {
		snippet, _, ok := strings.Cut(block, "```")
		if !ok || !strings.Contains(snippet, "[tool.astro.env") {
			continue
		}
		// A snippet declaring other sections too would fail decodeEnv's narrow
		// struct rather than the grammar; the env blocks in this page stand
		// alone, and this asserts that stays true.
		blocks++
		t.Run(fmt.Sprintf("block-%d", blocks), func(t *testing.T) {
			if _, err := ParseSchema(decodeEnv(t, snippet)); err != nil {
				t.Errorf("a documented example does not parse:\n%s\nerror: %v", snippet, err)
			}
		})
	}
	if blocks == 0 {
		t.Fatal("found no [tool.astro.env] examples; this test would pass vacuously")
	}
}
