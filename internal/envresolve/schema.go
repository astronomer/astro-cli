package envresolve

import (
	"fmt"
	"sort"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// DeclaredEnvKeys is every Airflow env-var name a schema declares, sorted.
//
// It lives here because it is a property of the schema rather than of any one
// source, and because both places that gate a global tier on "did this project
// ask for it?" need exactly this list: the plain global file (internal/localenv)
// and the global vault tier (internal/vaultenv). Two copies of it would be two
// answers to that question.
func DeclaredEnvKeys(schema *envschema.Schema) []string {
	if schema == nil {
		return nil
	}
	keys := make([]string, 0, len(schema.EnvVars)+len(schema.AirflowVariables)+len(schema.Connections))
	for name := range schema.EnvVars {
		keys = append(keys, name)
	}
	for key := range schema.AirflowVariables {
		keys = append(keys, airflowenv.EnvKeyForVarKey(key))
	}
	for id := range schema.Connections {
		keys = append(keys, airflowenv.EnvKeyForConnID(id))
	}
	sort.Strings(keys)
	return keys
}

// Problem is one schema-decoding finding, addressed by the dotted TOML key
// it concerns (mirroring manifest.Problem).
type Problem struct {
	Key    string
	Reason string
}

// SchemaError reports a [tool.astro.env] section that does not decode.
// Problems are sorted by key and cover every finding, not just the first.
type SchemaError struct {
	Problems []Problem
}

func (e *SchemaError) Error() string {
	if len(e.Problems) == 1 {
		return fmt.Sprintf("invalid [tool.astro.env]: %s: %s", e.Problems[0].Key, e.Problems[0].Reason)
	}
	return fmt.Sprintf("invalid [tool.astro.env]: %d problems, first is %s: %s",
		len(e.Problems), e.Problems[0].Key, e.Problems[0].Reason)
}

// envRoot prefixes every Problem key: the section the plain data came from.
const envRoot = "tool.astro.env"

// ParseSchema types the decoded [tool.astro.env] section (manifest Astro.Env,
// plain data). A string value is a committed default; a table means the value
// lives outside the manifest:
//
//	LOG_LEVEL = 'info'                   # a string: a committed default
//	WAREHOUSE_URI = {}                   # an empty table: the developer supplies it
//	API_TOKEN = { source = 'workspace' } # also resolves from the workspace's EM values
//
// Plain env vars live directly under [tool.astro.env]; connections and Airflow
// variables use the same grammar under their own sub-sections, which pick the
// AIRFLOW_CONN_/AIRFLOW_VAR_ encoding:
//
//	[tool.astro.env]
//	WAREHOUSE_URI = {}
//
//	[tool.astro.env.connections]
//	warehouse = {}
//
//	[tool.astro.env.airflow_variables]
//	batch_size = '500'
//
// Decoding is strict — an unknown sub-section, an unknown table key, a wrong
// value shape, an unknown source, and names that can't be env vars are all
// Problems — because this is authored config and a typo silently dropping a
// declaration would defeat the clone-and-run check. A nil/empty map yields an
// empty schema.
func ParseSchema(env map[string]any) (*envschema.Schema, error) {
	p := &schemaParser{}
	s := &envschema.Schema{}
	// Plain env vars sit directly under [tool.astro.env]; the two sub-sections
	// are reserved keys within it. Everything else is a plain env var.
	vars := map[string]any{}
	for name, raw := range env {
		switch name {
		case "connections":
			s.Connections = p.specs(envRoot+".connections", raw, airflowenv.ValidConnID,
				"not a valid connection id (letters, digits, _)")
		case "airflow_variables":
			s.AirflowVariables = p.specs(envRoot+".airflow_variables", raw, airflowenv.ValidVarKey,
				"not a valid variable key (letters, digits, _)")
		default:
			vars[name] = raw
		}
	}
	s.EnvVars = p.specs(envRoot, vars, airflowenv.ValidEnvKey,
		"not a legal env-var name (letters, digits, _; no leading digit)")

	if len(p.problems) > 0 {
		sort.SliceStable(p.problems, func(i, j int) bool { return p.problems[i].Key < p.problems[j].Key })
		return nil, &SchemaError{Problems: p.problems}
	}
	return s, nil
}

type schemaParser struct {
	problems []Problem
}

func (p *schemaParser) add(key, reason string) {
	p.problems = append(p.problems, Problem{Key: key, Reason: reason})
}

// specs decodes one map of declarations. validName gates each declared name:
// every name must be expressible as an env var, and the rule differs per
// section — a plain var must itself be a legal env-var name (ValidEnvKey), an
// Airflow Variable key may start with a digit (ValidVarKey), a connection id is
// ValidConnID.
func (p *schemaParser) specs(key string, raw any, validName func(string) bool, invalidReason string) map[string]envschema.ValueSpec {
	table, ok := raw.(map[string]any)
	if !ok {
		p.add(key, "expected a table")
		return nil
	}
	if len(table) == 0 {
		return nil
	}
	out := make(map[string]envschema.ValueSpec, len(table))
	for name, specRaw := range table {
		specKey := key + "." + name
		if !validName(name) {
			p.add(specKey, invalidReason)
			continue
		}
		if spec, ok := p.decodeSpec(specKey, specRaw); ok {
			out[name] = spec
		}
	}
	return out
}

// decodeSpec decodes one declaration: a string is a committed default (the
// empty string included); a table means the value lives outside the manifest,
// with an optional `source`. ok is false when the declaration is malformed and
// a Problem was recorded.
func (p *schemaParser) decodeSpec(key string, raw any) (envschema.ValueSpec, bool) {
	switch v := raw.(type) {
	case string:
		return envschema.ValueSpec{Default: v, HasDefault: true}, true
	case map[string]any:
		return p.decodeSpecTable(key, v)
	default:
		p.add(key, "expected a string default or a { source = \"workspace\" } table")
		return envschema.ValueSpec{}, false
	}
}

func (p *schemaParser) decodeSpecTable(key string, table map[string]any) (envschema.ValueSpec, bool) {
	var spec envschema.ValueSpec
	before := len(p.problems)
	for field, v := range table {
		fieldKey := key + "." + field
		switch field {
		case "source":
			spec.Source, _ = p.source(fieldKey, v)
		default:
			p.add(fieldKey, "unknown field")
		}
	}
	// ok only if no field recorded a problem; map order must not matter.
	return spec, len(p.problems) == before
}

// source decodes a declaration's `source` field. The only value today is
// "workspace"; anything else is a schema problem, per the package's strict
// parse. An empty string is rejected too — omit it for local-only.
func (p *schemaParser) source(key string, raw any) (envschema.Source, bool) {
	s, ok := p.str(key, raw)
	if !ok {
		return "", false
	}
	src := envschema.Source(s)
	if src != envschema.SourceWorkspace {
		p.add(key, fmt.Sprintf("%q is not a source (workspace)", s))
		return "", false
	}
	return src, true
}

func (p *schemaParser) str(key string, v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		p.add(key, "expected a string")
	}
	return s, ok
}
