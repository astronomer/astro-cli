package envresolve

import (
	"fmt"
	"sort"
	"strconv"

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
				"not a valid connection id (letters, digits, _)", true)
		case "airflow_variables":
			s.AirflowVariables = p.specs(envRoot+".airflow_variables", raw, airflowenv.ValidVarKey,
				"not a valid variable key (letters, digits, _)", false)
		default:
			vars[name] = raw
		}
	}
	s.EnvVars = p.specs(envRoot, vars, airflowenv.ValidEnvKey,
		"not a legal env-var name (letters, digits, _; no leading digit)", false)

	if len(p.problems) > 0 {
		sort.SliceStable(p.problems, func(i, j int) bool { return p.problems[i].Key < p.problems[j].Key })
		return nil, &SchemaError{Problems: p.problems}
	}
	return s, nil
}

// sensitiveDefaultRefusal is the one wording for "a value that belongs in a
// vault must not carry a committed default", used by both paths that can reach
// it — the table form and the string shorthand. It was written out twice, and
// two copies of a user-facing sentence diverge the first time one is reworded.
const sensitiveDefaultRefusal = " must not carry a default: it would be committed to the manifest and written into the environment on start"

// connectionAlwaysSensitive names why a connection is refused a default without
// the author ever having typed `sensitive`.
const connectionAlwaysSensitive = "a connection, which is always sensitive,"

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
func (p *schemaParser) specs(key string, raw any, validName func(string) bool, invalidReason string, isConnections bool) map[string]envschema.ValueSpec {
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
		if spec, ok := p.decodeSpec(specKey, specRaw, isConnections); ok {
			out[name] = spec
		}
	}
	return out
}

// decodeSpec decodes one declaration: a string is a committed default (the
// empty string included); a table means the value lives outside the manifest,
// with an optional `source`. ok is false when the declaration is malformed and
// a Problem was recorded.
func (p *schemaParser) decodeSpec(key string, raw any, isConnections bool) (envschema.ValueSpec, bool) {
	switch v := raw.(type) {
	case string:
		// The shorthand, and now literally sugar: it sets the same Default the
		// table's `default` key sets, and nothing else — except that a
		// connection is sensitive whichever spelling declared it.
		//
		// Which makes the shorthand a way to write a connection with a committed
		// default, i.e. a credential in the manifest. It is refused here for the
		// same reason the table form is: the shorthand cannot say `sensitive`,
		// so without this it would be the way around the rule rather than sugar
		// for it.
		if isConnections {
			p.add(key+".default", connectionAlwaysSensitive+sensitiveDefaultRefusal)
			return envschema.ValueSpec{}, false
		}
		return envschema.ValueSpec{Default: v, HasDefault: true}, true
	case map[string]any:
		return p.decodeSpecTable(key, v, isConnections)
	default:
		p.add(key, "expected a string default or a table")
		return envschema.ValueSpec{}, false
	}
}

func (p *schemaParser) decodeSpecTable(key string, table map[string]any, isConnections bool) (envschema.ValueSpec, bool) {
	var spec envschema.ValueSpec
	before := len(p.problems)
	// A connection carries a credential by construction, so it is sensitive
	// whether or not anyone said so. Per-declaration opt-in would mean every
	// connection that forgot the flag read as plaintext-safe, which is the
	// downgrade this grammar must not introduce: the schema it replaces treats
	// connections as unconditionally sensitive.
	spec.Sensitive = isConnections
	// Which fields failed to decode, so the coherence rules below do not read a
	// zero value left by a failure and report a second, contradictory problem.
	failed := map[string]bool{}
	for field, v := range table {
		fieldKey := key + "." + field
		switch field {
		case "source":
			spec.Source, _ = p.source(fieldKey, v)
		case "default":
			// Scalars are accepted and stringified. Declaring `type = 'int'` is
			// an invitation to write `default = 8080`, and refusing that as
			// "expected a string" would be a rule discoverable only by hitting
			// it. Every value resolves as a string downstream, so this is a
			// spelling convenience rather than a typed default.
			if d, ok := p.scalar(fieldKey, v); ok {
				spec.Default, spec.HasDefault = d, true
			} else {
				failed["default"] = true
			}
		case "optional":
			spec.Optional = p.boolField(fieldKey, v)
		case "sensitive":
			if isConnections {
				// Not "ignored": saying it here is either redundant or an
				// attempt to turn it off, and the second one must not look like
				// it worked.
				p.add(fieldKey, "connections are always sensitive, so this says nothing")
				continue
			}
			spec.Sensitive = p.boolField(fieldKey, v)
		case "description":
			spec.Description, _ = p.str(fieldKey, v)
		case "type":
			t, ok := p.str(fieldKey, v)
			switch {
			case !ok:
				failed["type"] = true
			case t == "":
				// ValidType accepts "" so that an ABSENT type is valid, not so
				// an author can write one. An empty source and an empty
				// conn_type are both refused; this is the same typo.
				p.add(fieldKey, "expected a type, not an empty string — omit the key for a plain string")
				failed["type"] = true
			case !envschema.ValidType(envschema.ValueType(t)):
				p.add(fieldKey, fmt.Sprintf("%q is not a known type (string, int, number, bool, enum, url, port, json)", t))
				failed["type"] = true
			default:
				spec.Type = envschema.ValueType(t)
			}
		case "enum":
			e, ok := p.strList(fieldKey, v)
			switch {
			case !ok:
				failed["enum"] = true
			case len(e) == 0:
				// Says nothing, and the coherence rule below is gated on a
				// non-empty enum so it would have passed silently. It also
				// stored a non-nil empty slice, which is not DeepEqual to the
				// nil an undeclared enum leaves — so two specs that mean the
				// same thing compared unequal.
				p.add(fieldKey, "expected at least one value — omit the key if there is no fixed set")
				failed["enum"] = true
			default:
				spec.Enum = e
			}
		case "conn_type":
			// Gated here rather than by the type system, because one ValueSpec
			// is shared by all three sections on purpose. An env var with a
			// conn_type is a mistake worth naming rather than a field to ignore.
			if !isConnections {
				p.add(fieldKey, "conn_type is only meaningful under connections")
				continue
			}
			ct, ok := p.str(fieldKey, v)
			if ok && ct == "" {
				p.add(fieldKey, "expected a connection type, not an empty string")
				continue
			}
			spec.ConnType = ct
		default:
			p.add(fieldKey, "unknown field")
		}
	}
	p.checkCoherence(key, spec, failed, isConnections)
	// ok only if no field recorded a problem; map order must not matter.
	return spec, len(p.problems) == before
}

// checkCoherence applies the rules that need a finished spec rather than a
// single field, so map iteration order cannot decide whether a declaration is
// well formed.
//
// failed names the fields whose own decode already recorded a problem, and each
// rule skips when a field it reads is in there. Otherwise one authoring mistake
// produces two problems and the second contradicts what the user wrote:
// `{ type = 'enum', enum = ['a', 2] }` reporting the real bad element AND "needs
// a non-empty enum", when a non-empty enum is exactly what they supplied.
func (p *schemaParser) checkCoherence(key string, spec envschema.ValueSpec, failed map[string]bool, isConnections bool) {
	if !failed["enum"] && !failed["type"] {
		if len(spec.Enum) > 0 && spec.Type != envschema.TypeEnum {
			p.add(key+".enum", "enum needs type = \"enum\"")
		}
		if spec.Type == envschema.TypeEnum && len(spec.Enum) == 0 {
			p.add(key+".type", "type = \"enum\" needs a non-empty enum")
		}
	}
	// A sensitive value may not carry a default. A default is committed to the
	// manifest and injected at start — resolve.go puts it in the injected set,
	// which becomes Plan.Env, which docker mode writes into the compose file it
	// leaves on disk. So `{ sensitive = true, default = ... }` would put the
	// credential in git AND on disk, which is precisely what the flag exists to
	// prevent. This is the one combination the first version of these checks let
	// through.
	if !failed["default"] && spec.HasDefault && spec.Sensitive {
		what := "a sensitive value"
		if isConnections {
			// The author never typed `sensitive` here, so name the real reason.
			what = connectionAlwaysSensitive
		}
		p.add(key+".default", what+sensitiveDefaultRefusal)
	}
}

// boolField decodes a bool-valued annotation. A wrong type records a problem and
// yields false; there is no second return, because the problem list is already
// how a caller learns the declaration is malformed and no caller wanted to
// branch on it.
func (p *schemaParser) boolField(key string, raw any) bool {
	v, ok := raw.(bool)
	if !ok {
		p.add(key, "expected true or false")
		return false
	}
	return v
}

// strList decodes an array-of-strings annotation, naming every offending index
// rather than the whole array so a long enum points at the entries to fix.
//
// Every one of them, not the first: ParseSchema's contract is that a strict
// parse reports all the problems at once, each by its dotted key, so that a fix
// is one pass rather than one round trip per bad element.
func (p *schemaParser) strList(key string, raw any) ([]string, bool) {
	items, ok := raw.([]any)
	if !ok {
		p.add(key, "expected an array of strings")
		return nil, false
	}
	out := make([]string, 0, len(items))
	bad := false
	for i, item := range items {
		v, isStr := item.(string)
		if !isStr {
			// Zero-padded: ParseSchema sorts problems by key as a string, so a
			// bare index puts enum[10] before enum[1] — and a long enum is
			// exactly the case this indexing exists to serve.
			p.add(fmt.Sprintf("%s[%03d]", key, i), "expected a string")
			bad = true
			continue
		}
		out = append(out, v)
	}
	if bad {
		return nil, false
	}
	return out, true
}

// scalar decodes an annotation that may be written as a string or as a bare TOML
// scalar, returning the string form. int64 and float64 are what go-toml hands
// back for integers and floats.
func (p *schemaParser) scalar(key string, raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case float64:
		// 'f', not 'g': 'g' renders 1000000.0 as "1e+06", and this string is
		// injected into the environment verbatim — so a DAG doing
		// int(os.environ["X"]) raises on a default the author wrote as a plain
		// number. Exponent notation is not round-trippable for the int, number
		// and port types this grammar just added.
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		p.add(key, "expected a string, number, or boolean")
		return "", false
	}
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
