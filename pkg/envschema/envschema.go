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
// # Annotations
//
// A table declaration may carry annotations describing the value, all optional:
//
//	DB_PASSWORD  = { sensitive = true, description = 'Warehouse password' }
//	LOG_LEVEL    = { default = 'info', type = 'enum', enum = ['debug', 'info'] }
//	SLACK_WEBHOOK = { optional = true, type = 'url' }
//
// `sensitive` says a value belongs in a vault rather than a plaintext file.
// Inferring sensitivity from a name is the guess that leaks a credential, so it
// is declared instead.
//
// It is a DECLARATION, and nothing in this repo routes on it yet: `astro local
// env set` still picks its store from --secret alone. So today it is a promise
// to consumers — Astro Desktop's Environment Manager is the first — rather than
// a behavior of this CLI. Saying otherwise in the present tense would be a
// claim about code that does not exist, in the one annotation whose whole point
// is keeping a credential out of a file.
//
// A sensitive declaration may not carry a `default`. A default is committed to
// the manifest and injected at start, so `{ sensitive = true, default = ... }`
// would put the credential in git and in the compose file docker mode writes —
// the exact outcome the flag exists to prevent.
//
// Connections are sensitive unconditionally and cannot say otherwise: a
// connection carries a credential by construction, and a per-declaration opt-in
// would mean every connection that forgot the flag read as plaintext-safe.
//
// `default` in the table is the same default the string shorthand sets, which
// makes `LOG_LEVEL = 'info'` sugar for `LOG_LEVEL = { default = 'info' }`. The
// shorthand exists because most declarations are exactly that; the long form
// exists because a value with a default may also need a type or a description,
// and the shorthand has nowhere to put them.
//
// `conn_type` is accepted only under [tool.astro.env.connections]. The spec type
// is shared across all three sections, so the parser is what refuses it
// elsewhere rather than the type system.
//
// # Required and defaults
//
// Every declared name is required by default: it must resolve from somewhere or
// the run is refused. That is the clone-and-run gate — the reason declaring a
// name is worth doing. A default counts as resolved (an empty-string default too
// — a string is a string, no magic values) and sits at the bottom of the chain:
//
//	shell > project .env > global file > workspace EM > manifest default
//
// `optional = true` opts one declaration out of that gate: it is documented,
// typed and routed like any other, and its absence does not refuse the run.
//
// Optionality is a FIELD rather than a spelling, because the obvious alternative
// does not work. Expressing "optional" as an empty default (`X = \'\'`) makes the
// value RESOLVE, and a resolved default is injected into Airflow's environment —
// so the variable is set-and-empty rather than absent. `os.environ['X']` then
// succeeds where it should raise, `os.environ.get('X', fallback)` returns "" and
// not the fallback, and an empty AIRFLOW_CONN_* is a malformed connection URI
// rather than a missing one, which is worse than either.
//
// It is spelled `optional` and not `required = false` so that the zero value is
// the safe one. Absent means required, which is what every manifest written
// before this key meant; a `Required bool` would have to default to true, and any
// decode path that skipped the field would silently drop the gate.
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
	// Source is where a table declaration resolves from when it is not set
	// locally. Empty is local-only. See the package doc.
	Source Source
	// Type is the value's shape. It is metadata for consumers — Validate does
	// not check it, see that function's doc. Empty means TypeString.
	Type ValueType
	// Description is prose for whoever has to supply the value.
	Description string
	// ConnType is the expected Airflow connection type, and is meaningful only
	// under the connections section. The parser refuses it elsewhere. Like Type,
	// it is metadata: nothing here compares it to the resolved connection.
	ConnType string
	// Enum is the allowed set when Type is TypeEnum, and is empty otherwise.
	// Metadata, as above.
	Enum []string
	// The flags sit together at the end because interleaving them between the
	// strings costs 16 bytes of alignment padding — 128 rather than 112. That is
	// tidiness rather than a constraint: no code depends on the size, since the
	// loops over map[string]ValueSpec index rather than copying. An earlier
	// version of this comment said the order was load-bearing for a lint
	// threshold, which was true at the time and was a tripwire for whoever added
	// the next field.
	//
	// HasDefault distinguishes a string declaration from a table one; see
	// Default. Optional exempts this declaration from the missing-value gate,
	// and false — the zero value, and what every manifest written before the key
	// means — is required; the package doc says why it is not `Required`.
	// Sensitive says the value belongs in a vault rather than a plaintext file,
	// declared rather than inferred because guessing it from the name is how a
	// credential ends up in .env.
	HasDefault bool
	Optional   bool
	Sensitive  bool
}

// ValueType is the declared shape of a value. It describes what a value should
// be, for validation and for choosing an editor; it does not change how the
// value resolves.
type ValueType string

// The value types, matching the set the desktop's schema already uses so a
// project's declarations survive the move into the manifest unchanged.
const (
	TypeString ValueType = "string"
	TypeInt    ValueType = "int"
	TypeNumber ValueType = "number"
	TypeBool   ValueType = "bool"
	TypeEnum   ValueType = "enum"
	TypeURL    ValueType = "url"
	TypePort   ValueType = "port"
	TypeJSON   ValueType = "json"
)

// ValidType reports whether v names a type this grammar knows. The empty string
// is valid and means TypeString.
func ValidType(v ValueType) bool {
	switch v {
	case "", TypeString, TypeInt, TypeNumber, TypeBool, TypeEnum, TypeURL, TypePort, TypeJSON:
		return true
	}
	return false
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
