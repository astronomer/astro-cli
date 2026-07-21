// Package envschema declares and validates the environment a project
// expects: env vars, Airflow variables, and connections — the shape, not the
// values. The schema lives in the manifest ([tool.astro.env]); this package
// does no I/O. pkg/manifest parses, this validates, and the composition
// happens in each consumer (sub-modules do not import each other).
//
// The validator is lifted from Astro Desktop's envschema package in
// an earlier fix. Per-environment value sources ("bindings") also arrive with
// an earlier fix, as additive fields on the specs.
package envschema

// Schema is the [tool.astro.env] section, keyed by name.
type Schema struct {
	EnvVars          map[string]ValueSpec
	AirflowVariables map[string]ValueSpec
	Connections      map[string]ConnSpec
}

// ValueType says how a declared value is checked. A named string rather
// than an int enum so it reads the same in the manifest, in code, and in
// --output json.
type ValueType string

const (
	TypeString ValueType = "string"
	TypeInt    ValueType = "int"
	TypeNumber ValueType = "number"
	TypeBool   ValueType = "bool"
	TypePort   ValueType = "port"
	TypeURL    ValueType = "url"
	// TypeJSON is deliberately not validated: json values are often
	// templated.
	TypeJSON ValueType = "json"
)

// ValueSpec declares one expected value.
type ValueSpec struct {
	Type        ValueType
	Required    bool
	Sensitive   bool
	Description string
	// Enum, when non-empty, constrains the value to this set, whatever the
	// base Type.
	Enum []string
}

// ConnSpec declares one expected Airflow connection.
type ConnSpec struct {
	ConnType    string
	Required    bool
	Description string
}

// Values is what the caller actually assembled, for the validator to
// inspect. The caller owns layering and resolution order; this package only
// judges the result.
type Values struct {
	EnvVars          map[string]string
	AirflowVariables map[string]string
	Connections      map[string]string
}

// ViolationKind classifies a validation finding.
type ViolationKind string

const (
	// ViolationMissing: a required value is absent.
	ViolationMissing ViolationKind = "missing"
	// ViolationWrongType: a value is present but fails its type check.
	ViolationWrongType ViolationKind = "type"
)

// Violation is one validation finding.
type Violation struct {
	Kind   ViolationKind
	Key    string
	Reason string
}

// Validate reports schema violations in the assembled values.
// Implementation arrives with an earlier fix (lifted from desktop); until then it
// reports nothing and must not be wired into any command.
func Validate(s *Schema, v Values) []Violation {
	return nil
}
