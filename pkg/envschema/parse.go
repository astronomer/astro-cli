package envschema

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// Problem is one schema-decoding finding, addressed by the dotted TOML key
// it concerns (mirroring manifest.Problem).
type Problem struct {
	// Code names the rule, and is the part a caller may rely on. Reason is
	// the part a person reads; see ProblemCode for why they are separate.
	Code   ProblemCode
	Key    string
	Reason string
}

// SchemaError reports a [tool.astro.env] section that does not decode.
// Problems are sorted by key and cover every finding, not just the first.
type SchemaError struct {
	Problems []Problem
}

// Error lists every problem, one keyed line each, the way
// manifest.ValidationError renders its own.
//
// It used to print "%d problems, first is …" and stop, which contradicted the
// type's own promise directly above and made this section the one place a
// manifest could not be fixed in one pass: the author corrected the named key,
// re-ran, and met the next one. The findings were all collected already —
// ParseSchema walks the whole section — so only the rendering was throwing them
// away.
func (e *SchemaError) Error() string {
	if len(e.Problems) == 1 {
		return fmt.Sprintf("invalid [tool.astro.env]: %s: %s", e.Problems[0].Key, e.Problems[0].Reason)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "invalid [tool.astro.env]: %d problems", len(e.Problems))
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "\n  %s: %s", p.Key, p.Reason)
	}
	return b.String()
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
func ParseSchema(env map[string]any) (*Schema, error) {
	p := &schemaParser{}
	s := &Schema{}
	// Plain env vars sit directly under [tool.astro.env]; the two sub-sections
	// are reserved keys within it. Everything else is a plain env var.
	vars := map[string]any{}
	for name, raw := range env {
		switch name {
		case "connections":
			s.Connections = p.specs(envRoot+".connections", raw, airflowenv.ValidConnID,
				"not a valid connection id (letters, digits, _)", SectionConnection)
		case "airflow_variables":
			s.AirflowVariables = p.specs(envRoot+".airflow_variables", raw, airflowenv.ValidVarKey,
				"not a valid variable key (letters, digits, _)", SectionAirflowVariable)
		default:
			vars[name] = raw
		}
	}
	s.EnvVars = p.specs(envRoot, vars, airflowenv.ValidEnvKey,
		"not a legal env-var name (letters, digits, _; no leading digit)", SectionEnvVar)

	if len(p.problems) > 0 {
		sort.SliceStable(p.problems, func(i, j int) bool { return p.problems[i].Key < p.problems[j].Key })
		return nil, &SchemaError{Problems: p.problems}
	}
	return s, nil
}

type schemaParser struct {
	problems []Problem
}

func (p *schemaParser) add(code ProblemCode, key, reason string) {
	p.problems = append(p.problems, Problem{Code: code, Key: key, Reason: reason})
}

// specs decodes one map of declarations. validName gates each declared name:
// every name must be expressible as an env var, and the rule differs per
// section — a plain var must itself be a legal env-var name (ValidEnvKey), an
// Airflow Variable key may start with a digit (ValidVarKey), a connection id is
// ValidConnID.
func (p *schemaParser) specs(key string, raw any, validName func(string) bool, invalidReason string, section Section) map[string]ValueSpec {
	table, ok := raw.(map[string]any)
	if !ok {
		p.add(CodeExpectedTable, key, "expected a table")
		return nil
	}
	if len(table) == 0 {
		return nil
	}
	out := make(map[string]ValueSpec, len(table))
	for name, specRaw := range table {
		specKey := key + "." + name
		if !validName(name) {
			p.add(CodeNameInvalid, specKey, invalidReason)
			continue
		}
		if spec, ok := p.decodeSpec(specKey, specRaw, section); ok {
			out[name] = spec
		}
	}
	return out
}

// decodeSpec decodes one declaration: a string is a committed default (the
// empty string included); a table means the value lives outside the manifest,
// with an optional `source`. ok is false when the declaration is malformed and
// a Problem was recorded.
func (p *schemaParser) decodeSpec(key string, raw any, section Section) (ValueSpec, bool) {
	switch v := raw.(type) {
	case string:
		// The shorthand, and literally sugar: it sets the same Default the
		// table's `default` key sets, and nothing else — except that a
		// connection is sensitive whichever spelling declared it, which makes
		// the shorthand a way to write a connection with a committed default.
		// Check refuses that, so the shorthand cannot be the way around a rule
		// the table form obeys.
		spec := ValueSpec{
			Default:    v,
			HasDefault: true,
			Sensitive:  section == SectionConnection,
		}
		if !p.checkSpec(key, spec, section, nil) {
			return ValueSpec{}, false
		}
		return spec, true
	case map[string]any:
		return p.decodeSpecTable(key, v, section)
	default:
		p.add(CodeExpectedDeclaration, key, "expected a string default or a table")
		return ValueSpec{}, false
	}
}

func (p *schemaParser) decodeSpecTable(key string, table map[string]any, section Section) (ValueSpec, bool) {
	var spec ValueSpec
	before := len(p.problems)
	// Connections start sensitive, so an absent key means what it should. An
	// explicit `sensitive` still decodes over the top and Check judges the
	// result — which is how `sensitive = false` on a connection gets told it is
	// not allowed rather than that it "says nothing".
	spec.Sensitive = section == SectionConnection
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
			// Recorded the same way as sensitive below. No Check rule reads
			// Optional; the package checklist does (internal/pack's
			// valueMeta), so a mutant that drops this line fails the root
			// module's tests rather than this one's. The two bool
			// annotations are handled identically, so a rule that starts
			// reading Optional does not reacquire the double-problem the
			// sensitive case had.
			if b, ok := p.boolField(fieldKey, v); ok {
				spec.Optional = b
			} else {
				failed["optional"] = true
			}
		case "sensitive":
			// Left at its default when the decode failed: for a
			// connection that default is true, and overwriting it would
			// invite a second, contradictory problem from Check.
			//
			// HasSensitive records that the key was there at all, which is what
			// lets Check refuse `sensitive` on a connection without also
			// refusing every connection that simply omitted it. Not set on a
			// failed decode: the value is unknown, the decode already reported
			// itself, and claiming the author stated something would add a
			// second problem about a key they may have meant either way.
			if b, ok := p.boolField(fieldKey, v); ok {
				spec.Sensitive, spec.HasSensitive = b, true
			} else {
				failed["sensitive"] = true
			}
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
				// conn_type are both refused; this is the same typo. Check
				// cannot see the difference, since both arrive as "".
				p.add(CodeEmptyType, fieldKey, "expected a type, not an empty string — omit the key for a plain string")
				failed["type"] = true
			default:
				// An unknown type is Check's rule, not this loop's.
				spec.Type = ValueType(t)
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
				p.add(CodeEmptyEnum, fieldKey, "expected at least one value — omit the key if there is no fixed set")
				failed["enum"] = true
			default:
				spec.Enum = e
			}
		case "conn_type":
			// Decoded in every section and judged by Check, which is where
			// "conn_type means nothing here" belongs — one ValueSpec is shared
			// by all three sections, so that is a property of the spec plus its
			// section rather than of this loop.
			ct, ok := p.str(fieldKey, v)
			switch {
			case !ok:
				failed["conn_type"] = true
			case ct == "":
				p.add(CodeEmptyConnType, fieldKey, "expected a connection type, not an empty string")
				failed["conn_type"] = true
			default:
				spec.ConnType = ct
			}
		default:
			p.add(CodeUnknownField, fieldKey, "unknown field")
		}
	}
	p.checkSpec(key, spec, section, failed)
	// ok only if no field recorded a problem; map order must not matter.
	return spec, len(p.problems) == before
}

// checkSpec runs the type's own well-formedness rules and records what they
// find, returning whether the declaration is usable.
//
// The rules live on ValueSpec rather than here because this parser is
// not the only thing that builds one: a writer constructing a spec from UI
// state must obey them too.
//
// What stays here is everything needing the SOURCE TEXT rather than the
// finished spec: an unknown key, an empty string where a value was required, a
// field of the wrong TOML type. Check cannot see those — `type = ""` and an
// absent type both arrive as "".
//
// failed names the fields whose own decode already recorded a problem, and
// their findings are dropped, so one authoring mistake does not produce a
// second contradictory message: `{ type = 'enum', enum = ['a', 2] }` reports the
// bad element, not also "needs a non-empty enum".
// take a value receiver — a pointer here is dereferenced and copied by
// spec.Check below, so it silences the finding without avoiding the copy.
//
//nolint:gocritic // hugeParam: by value to match Check and CheckValue, which
func (p *schemaParser) checkSpec(key string, spec ValueSpec, section Section, failed map[string]bool) bool {
	ok := true
	for _, problem := range spec.Check(section) {
		// Filtered on what the rule READ, not on where it points: a rule can
		// report against one annotation while consulting another, as
		// "type = enum needs a non-empty enum" points at `type` and reads
		// `enum`.
		if anyFailed(problem.Reads, failed) {
			continue
		}
		problemKey := key
		if problem.Field != "" {
			problemKey = key + "." + problem.Field
		}
		p.add(problem.Code, problemKey, problem.Reason)
		ok = false
	}
	return ok
}

// anyFailed reports whether any of these annotations already recorded a problem
// of its own.
func anyFailed(fields []string, failed map[string]bool) bool {
	for _, f := range fields {
		if failed[f] {
			return true
		}
	}
	return false
}

// boolField decodes a bool-valued annotation, reporting whether it decoded so
// the caller can record the field in failed. A wrong type records a problem and
// yields false.
//
// The second return matters for `sensitive` on a connection, which defaults to
// true: without it a failed decode overwrote that default with false and Check
// then added "cannot be declared not sensitive", so one mistake produced two
// problems and the second contradicted an author who wrote `sensitive = 'yes'`
// meaning true.
func (p *schemaParser) boolField(key string, raw any) (value, ok bool) {
	v, isBool := raw.(bool)
	if !isBool {
		p.add(CodeExpectedBool, key, "expected true or false")
		return false, false
	}
	return v, true
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
		p.add(CodeExpectedStringArray, key, "expected an array of strings")
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
			p.add(CodeExpectedString, fmt.Sprintf("%s[%03d]", key, i), "expected a string")
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
		p.add(CodeExpectedScalar, key, "expected a string, number, or boolean")
		return "", false
	}
}

// source decodes a declaration's `source` field. The only value today is
// "workspace"; anything else is a schema problem, per the package's strict
// parse. An empty string is rejected too — omit it for local-only.
func (p *schemaParser) source(key string, raw any) (Source, bool) {
	s, ok := p.str(key, raw)
	if !ok {
		return "", false
	}
	src := Source(s)
	if src != SourceWorkspace {
		p.add(CodeUnknownSource, key, fmt.Sprintf("%q is not a source (workspace)", s))
		return "", false
	}
	return src, true
}

func (p *schemaParser) str(key string, v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		p.add(CodeExpectedString, key, "expected a string")
	}
	return s, ok
}

// DeclaredEnvKeys is every Airflow env-var name a schema declares, sorted.
//
// A property of the schema rather than of any one source, so every consumer
// gating a tier on "did this project ask for this name?" gets the same list:
// the CLI's plain global file and global vault tier, and the desktop's global
// Environment Manager. The AIRFLOW_VAR_/AIRFLOW_CONN_ encoding is applied here
// so no consumer re-derives it.
func DeclaredEnvKeys(schema *Schema) []string {
	return envKeys(schema, func(ValueSpec) bool { return true })
}

// WorkspaceEnvKeys is the Airflow env-var name of every declaration marked
// `source = "workspace"`, sorted: the keys a workspace's Environment Manager
// objects must map to for a start to pull them. Connections come back as
// AIRFLOW_CONN_<ID> and Airflow variables as AIRFLOW_VAR_<KEY>, upper-cased the
// way Airflow reads them, and plain env vars as declared.
//
// Both apps pull only these names (docs/v2-workspace-link.md), so the set is
// derived once here rather than by each; a nil schema declares none.
func WorkspaceEnvKeys(schema *Schema) []string {
	return envKeys(schema, func(spec ValueSpec) bool { return spec.Source == SourceWorkspace })
}

// envKeys is the Airflow env-var name of every declaration keep accepts,
// sorted, with the AIRFLOW_VAR_/AIRFLOW_CONN_ encoding applied.
func envKeys(schema *Schema, keep func(ValueSpec) bool) []string {
	if schema == nil {
		return nil
	}
	keys := make([]string, 0, len(schema.EnvVars)+len(schema.AirflowVariables)+len(schema.Connections))
	for name, spec := range schema.EnvVars {
		if keep(spec) {
			keys = append(keys, name)
		}
	}
	for key, spec := range schema.AirflowVariables {
		if keep(spec) {
			keys = append(keys, airflowenv.EnvKeyForVarKey(key))
		}
	}
	for id, spec := range schema.Connections {
		if keep(spec) {
			keys = append(keys, airflowenv.EnvKeyForConnID(id))
		}
	}
	sort.Strings(keys)
	return keys
}
