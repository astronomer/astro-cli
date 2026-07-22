// Package envschema declares and validates the environment a project
// expects: env vars, Airflow variables, and connections — the shape, not the
// values. The schema lives in the manifest ([tool.astro.env]); this package
// does no I/O. pkg/manifest parses, this validates, and the composition
// happens in each consumer (sub-modules do not import each other).
//
// The validator is lifted from Astro Desktop's envschema package
//.
//
// # Bindings
//
// A spec says what a project needs; a binding says where the value comes
// from in one environment. Bindings are keyed by environment name —
// EnvLocal for this machine, or a [tool.astro.deployments.<name>] name —
// and resolution reads only the binding for the environment it resolves
// for. A name with no binding for that environment uses DefaultBinding
// (the vault). This per-environment indirection is why local resolution
// can never pick up another environment's values: the prod credentials sit
// behind the "prod" binding, which a local resolve never reads.
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
	// Bindings maps environment name -> value source. See the package doc.
	Bindings map[string]Binding
}

// ConnSpec declares one expected Airflow connection.
type ConnSpec struct {
	ConnType    string
	Required    bool
	Description string
	// Bindings maps environment name -> value source. See the package doc.
	Bindings map[string]Binding
}

// EnvLocal is the binding key for the machine the CLI runs on. Every other
// environment name is a [tool.astro.deployments.<name>] entry.
const EnvLocal = "local"

// BindingSource names where a bound value comes from.
type BindingSource string

const (
	// SourceVault: the shared local vault (pkg/secrets), plus whatever the
	// resolver layers with it (the process env, in the CLI).
	SourceVault BindingSource = "vault"
	// SourceDeployment: the environment of a linked Astro deployment.
	// Stage 2 — resolvers must fail with a typed error, never fall back
	// to another source.
	SourceDeployment BindingSource = "deployment"
)

// Binding names the value source for one declared name in one environment.
type Binding struct {
	Source BindingSource
	// Deployment names the [tool.astro.deployments.<name>] entry supplying
	// the value. Meaningful only when Source is SourceDeployment.
	Deployment string
}

// DefaultBinding is what an unbound name resolves with: the vault.
var DefaultBinding = Binding{Source: SourceVault}

// BindingFor returns the binding to resolve with for env: the explicit
// entry when one exists, else DefaultBinding. Both consumers (CLI,
// desktop) share this rule so an unbound name means the same thing
// everywhere.
func BindingFor(bindings map[string]Binding, env string) Binding {
	if b, ok := bindings[env]; ok {
		return b
	}
	return DefaultBinding
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
