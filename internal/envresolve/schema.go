package envresolve

import (
	"fmt"
	"sort"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

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

// validValueTypes is the closed set a spec's type field may name.
var validValueTypes = map[envschema.ValueType]bool{
	envschema.TypeString: true,
	envschema.TypeInt:    true,
	envschema.TypeNumber: true,
	envschema.TypeBool:   true,
	envschema.TypePort:   true,
	envschema.TypeURL:    true,
	envschema.TypeJSON:   true,
}

// ParseSchema types the decoded [tool.astro.env] section (manifest
// Astro.Env, plain data). Sections are vars (plain env vars),
// airflow_variables, and connections, each a table keyed by name:
//
//	[tool.astro.env.vars.API_URL]
//	type = "url"
//	required = true
//
//	[tool.astro.env.connections.warehouse]
//	conn_type = "postgres"
//	required = true
//	[tool.astro.env.connections.warehouse.bindings.prod]
//	source = "deployment"
//	deployment = "prod"
//
// Decoding is strict — unknown sections, unknown spec fields, wrong field
// types, and names that can't be env vars are all Problems — because this
// is authored config and a typo silently dropping a declaration would
// defeat the clone-and-run check. A nil/empty map yields an empty schema.
// Binding environment keys and deployment names are not cross-checked
// against [tool.astro.deployments] here; that join happens where both are
// in hand (stage 2, with the deployment source itself).
func ParseSchema(env map[string]any) (*envschema.Schema, error) {
	p := &schemaParser{}
	s := &envschema.Schema{}
	for section, raw := range env {
		key := envRoot + "." + section
		switch section {
		case "vars":
			s.EnvVars = p.valueSpecs(key, raw, airflowenv.ValidEnvKey,
				"not a legal env-var name (letters, digits, _; no leading digit)")
		case "airflow_variables":
			s.AirflowVariables = p.valueSpecs(key, raw, airflowenv.ValidVarKey,
				"not a valid variable key (letters, digits, _)")
		case "connections":
			s.Connections = p.connSpecs(key, raw)
		default:
			p.add(key, "unknown section (expected vars, airflow_variables, or connections)")
		}
	}
	if len(p.problems) > 0 {
		sort.Slice(p.problems, func(i, j int) bool { return p.problems[i].Key < p.problems[j].Key })
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

// table asserts raw is a TOML table (map[string]any).
func (p *schemaParser) table(key string, raw any) (map[string]any, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		p.add(key, "expected a table")
	}
	return m, ok
}

// valueSpecs decodes one section of ValueSpec declarations. validName gates
// the declared name: every name must be expressible as an env var. The
// rules differ per section — a plain var must itself be a legal env-var
// name (ValidEnvKey), while an Airflow Variable key may start with a digit
// because the AIRFLOW_VAR_ prefix supplies the leading letter (ValidVarKey).
func (p *schemaParser) valueSpecs(key string, raw any, validName func(string) bool, invalidReason string) map[string]envschema.ValueSpec {
	table, ok := p.table(key, raw)
	if !ok || len(table) == 0 {
		return nil
	}
	out := make(map[string]envschema.ValueSpec, len(table))
	for name, specRaw := range table {
		specKey := key + "." + name
		if !validName(name) {
			p.add(specKey, invalidReason)
			continue
		}
		specTable, ok := p.table(specKey, specRaw)
		if !ok {
			continue
		}
		var spec envschema.ValueSpec
		for field, v := range specTable {
			fieldKey := specKey + "." + field
			switch field {
			case "type":
				if t, ok := p.str(fieldKey, v); ok {
					spec.Type = envschema.ValueType(t)
					if !validValueTypes[spec.Type] {
						p.add(fieldKey, fmt.Sprintf("%q is not a value type (string, int, number, bool, port, url, json)", t))
					}
				}
			case "required":
				spec.Required, _ = p.boolean(fieldKey, v)
			case "sensitive":
				spec.Sensitive, _ = p.boolean(fieldKey, v)
			case "description":
				spec.Description, _ = p.str(fieldKey, v)
			case "enum":
				spec.Enum = p.strSlice(fieldKey, v)
			case "bindings":
				spec.Bindings = p.bindings(fieldKey, v)
			default:
				p.add(fieldKey, "unknown field")
			}
		}
		out[name] = spec
	}
	return out
}

// connSpecs decodes the connections section.
func (p *schemaParser) connSpecs(key string, raw any) map[string]envschema.ConnSpec {
	table, ok := p.table(key, raw)
	if !ok || len(table) == 0 {
		return nil
	}
	out := make(map[string]envschema.ConnSpec, len(table))
	for connID, specRaw := range table {
		specKey := key + "." + connID
		if !airflowenv.ValidConnID(connID) {
			p.add(specKey, "not a valid connection id (letters, digits, _)")
			continue
		}
		specTable, ok := p.table(specKey, specRaw)
		if !ok {
			continue
		}
		var spec envschema.ConnSpec
		for field, v := range specTable {
			fieldKey := specKey + "." + field
			switch field {
			case "conn_type":
				spec.ConnType, _ = p.str(fieldKey, v)
			case "required":
				spec.Required, _ = p.boolean(fieldKey, v)
			case "description":
				spec.Description, _ = p.str(fieldKey, v)
			case "bindings":
				spec.Bindings = p.bindings(fieldKey, v)
			default:
				p.add(fieldKey, "unknown field")
			}
		}
		out[connID] = spec
	}
	return out
}

// bindings decodes a bindings table: environment name -> {source, deployment}.
func (p *schemaParser) bindings(key string, raw any) map[string]envschema.Binding {
	table, ok := p.table(key, raw)
	if !ok || len(table) == 0 {
		return nil
	}
	out := make(map[string]envschema.Binding, len(table))
	for env, bindingRaw := range table {
		bindingKey := key + "." + env
		bindingTable, ok := p.table(bindingKey, bindingRaw)
		if !ok {
			continue
		}
		var b envschema.Binding
		for field, v := range bindingTable {
			fieldKey := bindingKey + "." + field
			switch field {
			case "source":
				if s, ok := p.str(fieldKey, v); ok {
					b.Source = envschema.BindingSource(s)
					if b.Source != envschema.SourceVault && b.Source != envschema.SourceDeployment {
						p.add(fieldKey, fmt.Sprintf("%q is not a source (vault, deployment)", s))
					}
				}
			case "deployment":
				b.Deployment, _ = p.str(fieldKey, v)
			default:
				p.add(fieldKey, "unknown field")
			}
		}
		switch b.Source {
		case "":
			p.add(bindingKey+".source", "required")
		case envschema.SourceDeployment:
			if b.Deployment == "" {
				p.add(bindingKey+".deployment", "required when source is deployment")
			}
		case envschema.SourceVault:
			if b.Deployment != "" {
				p.add(bindingKey+".deployment", "only meaningful when source is deployment")
			}
		}
		out[env] = b
	}
	return out
}

func (p *schemaParser) str(key string, v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		p.add(key, "expected a string")
	}
	return s, ok
}

func (p *schemaParser) boolean(key string, v any) (val, ok bool) {
	val, ok = v.(bool)
	if !ok {
		p.add(key, "expected a boolean")
	}
	return val, ok
}

func (p *schemaParser) strSlice(key string, v any) []string {
	items, ok := v.([]any)
	if !ok {
		p.add(key, "expected an array of strings")
		return nil
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			p.add(fmt.Sprintf("%s[%d]", key, i), "expected a string")
			continue
		}
		out = append(out, s)
	}
	return out
}
