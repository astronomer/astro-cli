// Package envschema declares and validates the environment a project
// expects: env vars, Airflow variables, and connections — the shape, not the
// values. The schema lives in the manifest ([tool.astro.env]); this package
// does no I/O. pkg/manifest parses, this validates, and the composition
// happens in each consumer (sub-modules do not import each other).
//
// The validator is lifted from Astro Desktop's envschema package
//.
//
// # Source
//
// A spec says what a project needs; its optional `source` says where the
// value resolves from when it is not set locally. The default (empty source)
// resolves from the local chain only. SourceWorkspace also resolves from the
// workspace's Environment Manager objects for a logged-in user, below the
// local files.
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
	// Source is where the value resolves from when it is not set locally.
	// Empty (the default) is local-only. See the package doc.
	Source Source
}

// ConnSpec declares one expected Airflow connection.
type ConnSpec struct {
	ConnType    string
	Required    bool
	Description string
	// Source is where the value resolves from when it is not set locally.
	// Empty (the default) is local-only. See the package doc.
	Source Source
}

// Source names where a declared value resolves from when it is not set
// locally. The empty source is local-only.
type Source string

const (
	// SourceWorkspace resolves a name from the workspace's Environment Manager
	// objects — the team-shared tier — below the local files, for a logged-in
	// user. It is the only non-local source today.
	SourceWorkspace Source = "workspace"
)

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

// Section says which part of the schema a finding belongs to, so a
// violation for connection "warehouse" cannot be confused with one for an
// env var of the same name.
type Section string

const (
	SectionEnvVar          Section = "env_var"
	SectionAirflowVariable Section = "airflow_variable"
	SectionConnection      Section = "connection"
)

// Violation is one validation finding.
type Violation struct {
	Kind    ViolationKind
	Section Section
	Key     string
	Reason  string
}
