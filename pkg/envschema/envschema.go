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

import "fmt"

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
	// Type is the value's shape. Validate does not check it — presence is its
	// only question — but CheckValues and CheckValue do, against the resolved
	// value. Empty means TypeString, which constrains nothing.
	Type ValueType
	// Description is prose for whoever has to supply the value.
	Description string
	// ConnType is the expected Airflow connection type, and is meaningful only
	// under the connections section; Check refuses it elsewhere. CheckValues
	// compares it, case-insensitively, against the kind the connection actually
	// resolved to — but only when that kind is known. See CheckValues' doc for
	// the encodings where it is not, which are not rare.
	ConnType string
	// Enum is the allowed set when Type is TypeEnum, and is empty otherwise.
	// CheckValue tests membership. Check refuses one without the other, so an
	// empty Enum beside TypeEnum is an incomplete declaration rather than a set
	// admitting nothing — CheckValue treats it as unconstrained on that basis.
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

// SpecProblem is one way a declaration is not well formed: the annotation at
// fault, and why.
//
// Field-relative rather than fully qualified, because this package does not know
// where the declaration came from. A TOML reader prefixes its dotted key; a
// writer building a spec from a form can point at the input.
type SpecProblem struct {
	// Field is the annotation at fault — "default", "sensitive", "type",
	// "enum", "conn_type" — or "" for the declaration as a whole.
	Field string
	// Reads names every annotation the rule consulted, which is not always just
	// Field: "type = enum needs a non-empty enum" points at `type` and reads
	// `enum`.
	//
	// It exists for a caller that already knows a field is broken. A TOML reader
	// that failed to decode `enum` should not then report a rule that read the
	// zero value it left behind, because the second message contradicts what the
	// author actually wrote. Pointing at the field is not enough to work that
	// out — the rule has to say what it looked at.
	Reads []string
	// Reason is a sentence for whoever wrote the declaration.
	Reason string
}

// Check reports the ways this declaration is not well formed, given the section
// it sits in. It returns nil for a valid one.
//
// These rules live on the TYPE rather than in the TOML reader on purpose, and
// that purpose is a writer. The reader (internal/envresolve) is not the only
// thing that builds a ValueSpec: O19 moves Astro Desktop's declaration source
// into the manifest, read AND write together, so the Environment Manager will
// construct specs from UI state and serialize them. Enforced only on the way in,
// these rules would let it write a manifest that the next `astro local start`
// refuses to load — a file the tool that wrote it cannot read.
//
// Well-formedness only. Whether a value RESOLVES is Validate's question, and
// whether the TOML decoded at all is the reader's; this is whether the
// declaration describes something coherent. `{ sensitive = true, default = 'x' }`
// decodes perfectly and is not a thing anyone is allowed to mean.
//
// The value receiver is deliberate, and stays even as this struct grows past a
// copy threshold: Schema holds specs as map[string]ValueSpec, and a pointer
// method cannot be called on a map index expression, so the obvious caller —
// walking a schema and checking each declaration — would not compile. A 112-byte
// copy once per declaration, at parse and at write, is not worth that.
//
//nolint:gocritic // hugeParam: see above; by value on purpose.
func (s ValueSpec) Check(section Section) []SpecProblem {
	var out []SpecProblem
	// reads defaults to the field itself, which is right for every rule that
	// consults only what it points at.
	add := func(field, reason string, reads ...string) {
		if len(reads) == 0 {
			reads = []string{field}
		}
		out = append(out, SpecProblem{Field: field, Reads: reads, Reason: reason})
	}

	// skipTypeCoherence silences the enum/type pair at the bottom of this
	// function. Those two rules READ the type, so once the type has been
	// reported — or ruled out entirely — whatever they conclude from it is a
	// second message about one mistake.
	skipTypeCoherence := false

	if section == SectionConnection {
		// A connection declares its KIND with conn_type, not its shape with
		// type. What resolves for one is a URI or a JSON blob, and the resolver
		// reduces it to a conn_type before anything judges it — so `type` and
		// `enum` on a connection could not be enforced by CheckValues even in
		// principle. Refused rather than ignored for the reason this grammar
		// keeps running into: an annotation accepted and never applied is worse
		// than one rejected, because the author believes it took effect.
		//
		// Both are reported on their own terms, which is why the pair rules are
		// skipped wholesale here rather than per-field: a connection with an
		// enum and no type would otherwise also be told to add `type = "enum"`,
		// the one thing it may not do.
		skipTypeCoherence = true
		if s.Type != "" {
			// Instead of, not as well as, "not a known type": a connection
			// writing `type = 'enom'` has one mistake worth naming and it is
			// not the spelling.
			add("type", "type describes an env var or an Airflow variable, so it means nothing on a connection — conn_type is how a connection declares its kind")
		}
		if len(s.Enum) > 0 {
			add("enum", "enum needs type = \"enum\", which a connection cannot declare", "enum", "type")
		}
	} else if !ValidType(s.Type) {
		// An unrecognized type is reported once. Otherwise
		// `{ type = 'enom', enum = ['a'] }` reports the real mistake AND "enum
		// needs type = enum", the second contradicting a type the author plainly
		// tried to write — the same cascade a caller filters with Reads, one
		// level in.
		add("type", fmt.Sprintf("%q is not a known type (string, int, number, bool, enum, url, port, json)", s.Type))
		skipTypeCoherence = true
	}

	// A connection carries a credential by construction, so it may not be
	// declared otherwise. A reader defaults this to true for an absent key; the
	// rule is here so a constructed spec cannot skip it.
	if section == SectionConnection && !s.Sensitive {
		add("sensitive", "a connection always holds a credential, so it cannot be declared not sensitive")
	}

	// A sensitive value may not carry a default. A default is committed to the
	// manifest and injected into the environment at start, so this would put the
	// credential in version control and on disk — the one thing the flag exists
	// to prevent.
	if s.Sensitive && s.HasDefault {
		what := "a sensitive value"
		if section == SectionConnection {
			// Nobody had to write `sensitive` for a connection, so name the
			// reason it is one.
			what = "a connection, which is always sensitive,"
		}
		add("default", what+" must not carry a default: it would be committed to the manifest and written into the environment on start", "default", "sensitive")
	}

	if s.ConnType != "" && section != SectionConnection {
		add("conn_type", "conn_type describes a connection, so it means nothing in "+string(section))
	}

	// enum and type = 'enum' each require the other, and each rule reads both.
	if !skipTypeCoherence {
		if len(s.Enum) > 0 && s.Type != TypeEnum {
			add("enum", "enum needs type = \"enum\"", "enum", "type")
		}
		if s.Type == TypeEnum && len(s.Enum) == 0 {
			add("type", "type = \"enum\" needs a non-empty enum", "type", "enum")
		}
	}
	return out
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
	// EnvVars and AirflowVariables map a declared name to its resolved value.
	EnvVars          map[string]string
	AirflowVariables map[string]string
	// Connections maps a connection id to the connection's TYPE — "postgres",
	// "snowflake" — and never to the connection itself.
	//
	// Spelled out because it is easy to get wrong and no longer harmless. It
	// used to be: Validate asked only whether the string was non-empty, so a
	// caller passing the whole AIRFLOW_CONN_ payload got the same answer.
	// CheckValues now formats this string into a message a caller prints, so a
	// payload here produces both a false mismatch and a dump of the connection
	// — password included. The empty string means the kind could not be
	// determined, which is not a mismatch and is skipped.
	Connections map[string]string
}

// ViolationKind classifies a validation finding.
type ViolationKind string

const (
	// ViolationMissing: a declared value resolved from nowhere.
	ViolationMissing ViolationKind = "missing"
	// ViolationWrongType: a value is present but is not what its declaration
	// said — a value that fails its `type`, a connection that resolved to a
	// different `conn_type`, or a corrupt connection JSON. The first two come
	// from CheckValues; the last from the resolver, since only it can decode.
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
