package envschema

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// LegacyRelPath is where a v1 project's declarations live, relative to the
// project root. The file predates [tool.astro.env] and is written by the
// desktop app; the CLI has never produced one.
const LegacyRelPath = ".astro/env.schema.yaml"

// The two reserved keys inside [tool.astro.env]. Everything else directly under
// it is a plain env var, which is why one cannot be NAMED either.
const (
	sectionKeyConnections      = "connections"
	sectionKeyAirflowVariables = "airflow_variables"
)

// ParseLegacy reads a v1 .astro/env.schema.yaml into the shape this package
// owns, so a conversion can carry what it says into [tool.astro.env] and the
// file can then retire.
//
// It exists here rather than in the conversion because the two spellings of one
// grammar belong together: this is the only place that knows both, and a second
// copy is how they come to disagree about what a declaration means.
//
// # The gate flag inverts
//
// The v1 file spells it `required` and gates on TRUE; the manifest spells it
// `optional` and gates on FALSE. Their zero values disagree, so a declaration
// that mentions neither is not gated in the file and would be gated in the
// manifest — and that is the row nearly every declaration is on. Carrying the
// field across by name would quietly make a project refuse to start until every
// documented-but-optional name is set.
//
// # Connections are sensitive by construction
//
// The v1 file has no flag for it and the manifest grammar refuses a connection
// declared otherwise, so it is set here rather than left to a default.
//
// What this does NOT do is decide whether a declaration is legal to write. A v1
// file can hold names the manifest grammar refuses and combinations it rejects,
// and a conversion has to report those rather than drop them silently: run
// CheckName and ValueSpec.Check over the result and carry what passes.
//
// # Strict
//
// This reader's caller is about to DELETE the file, so anything it cannot carry
// faithfully is refused and reported while the file still exists. That is three
// things, not one:
//
//   - an unknown FIELD, which is a typo in authored config;
//   - a repeated key, because the v1 format is a LIST and this is a map: one of
//     the two survives and the other goes with the file;
//   - an entry with no key at all, which would land under the empty name.
//
// KnownFields alone catches only the first.
func ParseLegacy(data []byte) (*Schema, error) {
	return parseLegacy(data, true)
}

func parseLegacy(data []byte, strict bool) (*Schema, error) {
	var doc legacyDoc
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(strict)
	// io.EOF is an empty or comment-only file, which declares nothing. Treating
	// it as a failure would leave a conversion unable to carry the file and so
	// unable to retire it, forever, over a file that says nothing.
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", LegacyRelPath, err)
	}

	s := &Schema{
		EnvVars:          make(map[string]ValueSpec, len(doc.EnvVars)),
		AirflowVariables: make(map[string]ValueSpec, len(doc.AirflowVariables)),
		Connections:      make(map[string]ValueSpec, len(doc.Connections)),
	}
	// Indexed rather than ranged by value: ranging would copy a struct large
	// enough that this loop's legality depends on legacyValue staying under
	// gocritic's threshold, so the next field added to the v1 format would fail
	// lint here instead of where it was added.
	for i := range doc.EnvVars {
		v := &doc.EnvVars[i]
		if err := keep(s.EnvVars, "env_vars", v.Key, v.specPtr(), strict); err != nil {
			return nil, err
		}
	}
	for i := range doc.AirflowVariables {
		v := &doc.AirflowVariables[i]
		if err := keep(s.AirflowVariables, sectionKeyAirflowVariables, v.Key, v.specPtr(), strict); err != nil {
			return nil, err
		}
	}
	for i := range doc.Connections {
		c := &doc.Connections[i]
		spec := ValueSpec{
			Optional:    !c.Required,
			Sensitive:   true,
			ConnType:    c.ConnType,
			Description: c.Description,
		}
		if err := keep(s.Connections, sectionKeyConnections, c.ConnID, &spec, strict); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// keep puts one declaration into the map its section is read into.
//
// The v1 format is a list and this is a map, so two losses live here that no
// field-level check sees: a repeated key, where the map keeps one and the other
// is gone, and an entry with no key, which lands under the empty name and is
// then refused by the grammar as an illegal env-var name. Both are silent,
// which is the thing the strict reader exists to prevent.
func keep(into map[string]ValueSpec, section, name string, spec *ValueSpec, strict bool) error {
	if name == "" {
		if strict {
			return fmt.Errorf("%s: an entry has no key", section)
		}
		return nil
	}
	if _, dup := into[name]; dup && strict {
		return fmt.Errorf("%s: %q is declared twice, and only one of them can be carried", section, name)
	}
	into[name] = *spec
	return nil
}

// CheckName reports why a declared NAME cannot be written under
// [tool.astro.env], or nil when it can.
//
// Separate from ValueSpec.Check, which judges a declaration's annotations and
// never sees what it is called. Both have to pass before anything writes: the
// parser refuses an illegal name, and it refuses the WHOLE section over one of
// them — so a single bad name makes every other declaration in the project
// unreadable, and a caller that deletes its source first has destroyed the only
// copy.
//
// The rule is the parser's own, reached through the same package, so the two
// cannot drift.
func CheckName(section Section, name string) error {
	switch section {
	case SectionConnection:
		if !airflowenv.ValidConnID(name) {
			return fmt.Errorf("%q is not a valid connection id (letters, digits, _)", name)
		}
	case SectionAirflowVariable:
		if !airflowenv.ValidVarKey(name) {
			return fmt.Errorf("%q is not a valid Airflow Variable key (%s). Rename the variable, and the Dags reading it", name, airflowenv.VarKeyRule)
		}
	case SectionEnvVar:
		if !airflowenv.ValidEnvKey(name) {
			return fmt.Errorf("%q is not a legal env-var name (letters, digits, _; no leading digit)", name)
		}
		// A plain env var sits directly under [tool.astro.env], where these two
		// name the sub-sections. One called either is not expressible: the
		// parser reads the name as a section, and a writer would put the var's
		// own annotation keys where connection or variable declarations go.
		if name == sectionKeyConnections || name == sectionKeyAirflowVariables {
			return fmt.Errorf("%q is a reserved section name in [tool.astro.env] and cannot be an env var", name)
		}
	}
	return nil
}

// legacyDoc is the v1 file's own shape: lists carrying their key inline, rather
// than the manifest's tables keyed by name.
type legacyDoc struct {
	EnvVars          []legacyValue `yaml:"env_vars,omitempty"`
	AirflowVariables []legacyValue `yaml:"airflow_variables,omitempty"`
	Connections      []legacyConn  `yaml:"connections,omitempty"`
}

type legacyValue struct {
	Key         string   `yaml:"key"`
	Type        string   `yaml:"type,omitempty"`
	Required    bool     `yaml:"required,omitempty"`
	Sensitive   bool     `yaml:"sensitive,omitempty"`
	Default     string   `yaml:"default,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Enum        []string `yaml:"enum,omitempty"`
}

// specPtr is spec() addressable, so keep can take a pointer and the struct is
// never copied into a parameter.
func (v *legacyValue) specPtr() *ValueSpec {
	s := v.spec()
	return &s
}

func (v *legacyValue) spec() ValueSpec {
	return ValueSpec{
		// HasDefault rides with Default: the v1 file has no way to say
		// "declared with an empty default", so an empty string is no default.
		Default:    v.Default,
		HasDefault: v.Default != "",
		Optional:   !v.Required,
		Sensitive:  v.Sensitive,
		// HasSensitive rides with Sensitive for the same reason, and only here:
		// a v1 env var or variable STATES the flag, while a v1 connection has it
		// derived a few lines up — which is why the connection branch leaves
		// HasSensitive false and the grammar does not then refuse every
		// converted connection.
		//
		// `sensitive: false` reads as unstated, which loses nothing: no rule
		// outside connections reads the flag, and the renderer writes it only
		// when true, so a converted manifest still parses back to this spec.
		HasSensitive: v.Sensitive,
		Type:         ValueType(v.Type),
		Description:  v.Description,
		Enum:         v.Enum,
	}
}

type legacyConn struct {
	ConnID      string `yaml:"conn_id"`
	ConnType    string `yaml:"conn_type,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description,omitempty"`
}
