// Package envschema declares and validates the environment a project
// expects: env vars, Airflow variables, and connections. The manifest carries
// non-secret defaults; environments carry the overrides; secrets never go
// inline. The schema lives in the manifest ([tool.astro.env]); this package
// does no I/O. pkg/manifest parses, this validates, and the composition
// happens in each consumer (sub-modules do not import each other).
//
// # The grammar
//
// A string value is a committed default; a table means the value lives outside
// the manifest. All three spellings sit under [tool.astro.env]:
//
//	LOG_LEVEL = 'info'                   // a string: a committed default
//	WAREHOUSE_URI = {}                   // an empty table: the developer supplies it
//	API_TOKEN = { source = 'workspace' } // also resolves from the workspace's EM values
//
// Connections and Airflow variables use the same grammar under their own
// sub-sections, which pick the AIRFLOW_CONN_/AIRFLOW_VAR_ encoding.
//
// # Required and defaults
//
// Every declared name is required: it must resolve from somewhere or the run is
// refused. A default counts as resolved (an empty-string default too — a string
// is a string, no magic values) and sits at the bottom of the chain:
//
//	shell > project .env > global file > workspace EM > manifest default
//
// # Source
//
// A table declaration's optional `source` says where the value resolves from
// when no file supplies it. SourceWorkspace resolves from the workspace's
// Environment Manager objects for a logged-in user, below the local files. It
// is the only source today.
package envschema

// Schema is the [tool.astro.env] section, keyed by name.
type Schema struct {
	EnvVars          map[string]ValueSpec
	AirflowVariables map[string]ValueSpec
	Connections      map[string]ValueSpec
}

// ValueSpec declares one expected value.
type ValueSpec struct {
	// Default is the committed value used when nothing higher in the chain
	// supplies one. It is set only when HasDefault is true, and may then be the
	// empty string — a string declaration always carries a default.
	Default string
	// HasDefault distinguishes a string declaration (a default, possibly empty)
	// from a table declaration (no default: the value lives outside the
	// manifest). It is what tells "required, supply it" from "defaults to ''".
	HasDefault bool
	// Source is where a table declaration resolves from when it is not set
	// locally. Empty is local-only. See the package doc.
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
	// ViolationMissing: a declared value resolved from nowhere.
	ViolationMissing ViolationKind = "missing"
	// ViolationWrongType: a value is present but malformed (a corrupt
	// connection JSON is the only case today).
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
