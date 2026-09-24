// Package envschema declares and validates the environment a project
// expects: env vars, Airflow variables, and connections. The manifest carries
// non-secret defaults; environments carry the overrides; secrets never go
// inline. The schema lives in the manifest ([tool.astro.env]); this package
// does no I/O.
//
// pkg/manifest decodes the file and hands [tool.astro.env] over as plain
// untyped data; ParseSchema here turns that into a Schema, and Validate and
// CheckValues judge it. Composition happens in each consumer. This module is a
// declared exception to the no-sibling-imports rule — it imports pkg/airflowenv
// for the three name predicates the parser validates against — see
// docs/v2-architecture.md.
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
// Writers route on it, and this package does not: it has no I/O, so the rule
// lives with each writer. `astro local env <noun> set` stores a declared-sensitive
// name in the vault shared with Astro Desktop without --secret, refuses
// --secret=false for one, and refuses a save that depends on the declarations
// when they do not parse, rather than reading an unparseable section as
// "nothing is sensitive". Astro Desktop's Environment Manager keeps the same
// rule for its saves. Both tools also vault every connection and Airflow
// variable by default, declared or not, so for those two kinds the declaration
// decides only whether --secret=false may keep one in a plain file.
//
// A sensitive declaration may not carry a `default`. A default is committed to
// the manifest and injected at start, so `{ sensitive = true, default = ... }`
// would put the credential in git and in the compose file docker mode writes —
// the exact outcome the flag exists to prevent.
//
// Connections are sensitive unconditionally, and `sensitive` under
// [tool.astro.env.connections] is refused whichever value it carries. A
// connection holds a credential by construction, so a per-declaration opt-in
// would mean every connection that forgot the flag read as plaintext-safe —
// and accepting the agreeing value would leave a key that does nothing looking
// like a key that works, which is the same reason `type` and `enum` are refused
// there rather than ignored.
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
	// strings costs 16 bytes of alignment padding. That is tidiness rather than
	// a constraint: no code depends on the size, since the loops over
	// map[string]ValueSpec index rather than copying.
	//
	// HasDefault distinguishes a string declaration from a table one; see
	// Default. Optional exempts this declaration from the missing-value gate,
	// and false — the zero value, and what every manifest written before the key
	// means — is required; the package doc says why it is not `Required`.
	// Sensitive says the value belongs in a vault rather than a plaintext file,
	// declared rather than inferred because guessing it from the name is how a
	// credential ends up in .env.
	//
	// HasSensitive says the key was WRITTEN, whatever its value, which Sensitive
	// alone cannot express: a connection starts sensitive, so an absent key and
	// `sensitive = true` both arrive as true. Check needs the difference,
	// because under connections the two mean different things — one is the
	// normal case, the other is an author who believes the flag is theirs to
	// choose. Same job HasDefault does for Default.
	HasDefault   bool
	Optional     bool
	Sensitive    bool
	HasSensitive bool
}

// ProblemCode names the rule a problem came from.
//
// The Reason beside it is a sentence for a person, and a sentence for a
// person is not a thing to branch on: it gets reworded, it will one day be
// translated, and a caller matching on its text breaks both times. The code
// is what stays put — so a caller decides on the code, the prose improves
// freely, and a test asserts the rule fired rather than restating its
// wording in a second place.
//
// One type covers both kinds of finding the package reports. A SpecProblem
// from Check becomes a Problem on the way out (see checkSpec), so a single
// code follows it across that seam rather than being translated into a second
// vocabulary at the boundary.
//
// Shape and rule are both here, and the difference is which one repeats. A
// shape code — CodeExpectedTable, CodeExpectedString — is one rule applied to
// whichever key is wrong, so many keys share it and the Key says which. A
// rule code belongs to one refusal.
//
// Values are written out rather than derived from the constant names,
// because they are the stable part: renaming a Go identifier must not change
// what a caller sees.
type ProblemCode string

const (
	// Shape: the value is not the kind of thing the key takes. One rule,
	// many keys.
	CodeExpectedTable       ProblemCode = "expected_table"
	CodeExpectedString      ProblemCode = "expected_string"
	CodeExpectedBool        ProblemCode = "expected_bool"
	CodeExpectedStringArray ProblemCode = "expected_string_array"
	CodeExpectedScalar      ProblemCode = "expected_scalar"
	CodeExpectedDeclaration ProblemCode = "expected_declaration"
	CodeUnknownField        ProblemCode = "unknown_field"

	// Decoding one declaration.
	CodeNameInvalid   ProblemCode = "name_invalid"
	CodeEmptyType     ProblemCode = "empty_type"
	CodeEmptyEnum     ProblemCode = "empty_enum"
	CodeEmptyConnType ProblemCode = "empty_conn_type"
	CodeUnknownSource ProblemCode = "unknown_source"

	// Checking a decoded declaration against the rules (see SpecProblem).
	// CodeSensitiveDefault: a sensitive value carries a default, which would
	// be committed to the manifest.
	CodeSensitiveDefault ProblemCode = "sensitive_default"
	// CodeConnectionNotSensitive: a connection declared sensitive = false,
	// which contradicts what a connection is.
	CodeConnectionNotSensitive ProblemCode = "connection_not_sensitive"
	// CodeConnectionSensitiveRedundant: a connection declared sensitive =
	// true, which says nothing its section has not already said.
	CodeConnectionSensitiveRedundant ProblemCode = "connection_sensitive_redundant"
	// CodeTypeOnConnection: `type` on a connection, which declares its kind
	// with conn_type.
	CodeTypeOnConnection ProblemCode = "type_on_connection"
	// CodeEnumOnConnection: `enum` on a connection, which cannot declare the
	// type it would need.
	CodeEnumOnConnection ProblemCode = "enum_on_connection"
	// CodeConnTypeOutsideConnections: conn_type somewhere that is not a
	// connection.
	CodeConnTypeOutsideConnections ProblemCode = "conn_type_outside_connections"
	// CodeUnknownType: a `type` that is not one of the known ones.
	CodeUnknownType ProblemCode = "unknown_type"
	// CodeEnumNeedsType: an enum without type = "enum".
	CodeEnumNeedsType ProblemCode = "enum_needs_type"
	// CodeEnumTypeNeedsValues: type = "enum" with no values.
	CodeEnumTypeNeedsValues ProblemCode = "enum_type_needs_values"
)

// problemCodes is every code above, in declaration order — the closed set, in
// the package rather than in a test, so the tests that check the set read it
// instead of keeping a second copy a new code could be left out of.
var problemCodes = []ProblemCode{
	CodeExpectedTable, CodeExpectedString, CodeExpectedBool,
	CodeExpectedStringArray, CodeExpectedScalar, CodeExpectedDeclaration,
	CodeUnknownField,

	CodeNameInvalid, CodeEmptyType, CodeEmptyEnum, CodeEmptyConnType,
	CodeUnknownSource,

	CodeSensitiveDefault, CodeConnectionNotSensitive,
	CodeConnectionSensitiveRedundant, CodeTypeOnConnection,
	CodeEnumOnConnection, CodeConnTypeOutsideConnections, CodeUnknownType,
	CodeEnumNeedsType, CodeEnumTypeNeedsValues,
}

// SpecProblem is one way a declaration is not well formed: the annotation at
// fault, and why.
//
// Field-relative rather than fully qualified, because this package does not know
// where the declaration came from. A TOML reader prefixes its dotted key; a
// writer building a spec from a form can point at the input.
type SpecProblem struct {
	// Code names the rule, and is the part a caller may rely on. Reason is
	// the part a person reads; see ProblemCode for why they are separate.
	Code ProblemCode
	// Field is the annotation at fault — "default", "sensitive", "type",
	// "enum", "conn_type" — or "" for the declaration as a whole.
	Field string
	// Reads names every annotation the rule consulted, which is not always just
	// Field: "type = enum needs a non-empty enum" points at `type` and reads
	// `enum`.
	//
	// It is for a caller that already knows a field is broken — a reader whose
	// `enum` decode failed should not report a rule that read the zero value it
	// left behind, since that message contradicts what the author wrote. Field
	// alone is not enough to decide, so the rule states what it looked at.
	Reads []string
	// Reason is a sentence for whoever wrote the declaration.
	Reason string
}

// Check reports the ways this declaration is not well formed, given the section
// it sits in. It returns nil for a valid one.
//
// The rules live on the type rather than in the TOML reader because the reader
// is not the only thing that builds a ValueSpec: a writer constructing one from
// UI state and serializing it must obey them too, or it can emit a manifest the
// next `astro local start` refuses to load.
//
// Well-formedness only. Whether a value RESOLVES is Validate's question, and
// whether the TOML decoded at all is the reader's. `{ sensitive = true,
// default = 'x' }` decodes perfectly and is still not something anyone may
// mean.
//
// The value receiver is deliberate: Schema holds specs as
// map[string]ValueSpec, and a pointer method cannot be called on a map index
// expression, so walking a schema and checking each declaration would not
// compile.
//
//nolint:gocritic // hugeParam: see above; by value on purpose.
func (s ValueSpec) Check(section Section) []SpecProblem {
	var out []SpecProblem
	// reads defaults to the field itself, which is right for every rule that
	// consults only what it points at.
	add := func(code ProblemCode, field, reason string, reads ...string) {
		if len(reads) == 0 {
			reads = []string{field}
		}
		out = append(out, SpecProblem{Code: code, Field: field, Reads: reads, Reason: reason})
	}

	skipTypeCoherence := false

	if section == SectionConnection {
		// A connection declares its KIND with conn_type, not its shape with
		// type. What resolves for one is reduced to a conn_type before anything
		// judges it, so `type` and `enum` here could not be enforced even in
		// principle, and are refused rather than accepted and ignored.
		//
		// Both are reported on their own terms, so the pair rules are skipped
		// wholesale: a connection with an enum and no type would otherwise also
		// be told to add `type = "enum"`, the one thing it may not do.
		skipTypeCoherence = true
		if s.Type != "" {
			// Instead of, not as well as, "not a known type": one mistake, and
			// it is not the spelling.
			add(CodeTypeOnConnection, "type", "type describes an env var or an Airflow variable, so it means nothing on a connection — conn_type is how a connection declares its kind")
		}
		if len(s.Enum) > 0 {
			add(CodeEnumOnConnection, "enum", "enum needs type = \"enum\", which a connection cannot declare", "enum", "type")
		}
	} else if !ValidType(s.Type) {
		// Reported once: otherwise `{ type = 'enom', enum = ['a'] }` also gets
		// "enum needs type = enum", contradicting the type the author wrote.
		add(CodeUnknownType, "type", fmt.Sprintf("%q is not a known type (string, int, number, bool, enum, url, port, json)", s.Type))
		skipTypeCoherence = true
	}

	// A connection carries a credential by construction, so `sensitive` is not
	// an author's to state — either value is refused, rather than one being
	// rejected and the other quietly ignored. That is the same rule `type` and
	// `enum` get above, and for the same reason: a key that is accepted and
	// does nothing reads as a key that works.
	//
	// The two values are wrong in different ways, so they are told apart.
	// `sensitive = false` contradicts what a connection is. `sensitive = true`
	// agrees with it, but writing it means believing the flag decides — and an
	// author who believes that has a reason to think omitting it would make the
	// connection plaintext.
	//
	// Refusing the agreeing value does turn a manifest that parses into one that
	// does not, and this is the moment to do it: [tool.astro.env] is a v2
	// section, no v2 is released, and every shipped CLI is a v1 that never reads
	// it. The converter does not write the key either (pkg/scaffold asserts
	// that), so the only file carrying it was hand-written against the docs that
	// already called it an error. The same change after a release would be worth
	// arguing about; before one it costs nothing.
	//
	// Keyed on HasSensitive, not on the value: a reader defaults an absent key
	// to true for a connection, so Sensitive alone cannot tell "said so" from
	// "said nothing". The !s.Sensitive arm needs no such guard — nothing
	// defaults a connection to false, so reaching it means either an explicit
	// false or a constructed spec that skipped the rule.
	switch {
	case section != SectionConnection:
	case !s.Sensitive:
		add(CodeConnectionNotSensitive, "sensitive", "a connection always holds a credential, so it cannot be declared not sensitive")
	case s.HasSensitive:
		add(CodeConnectionSensitiveRedundant, "sensitive", "a connection is always sensitive, so sensitive = true adds nothing — remove it; the vault is chosen by the section, not by this flag")
	}

	// A sensitive value may not carry a default. A default is committed to the
	// manifest and injected into the environment at start, so this would put the
	// credential in version control and on disk — the one thing the flag exists
	// to prevent.
	// Keyed on the section as well as the flag: a connection is sensitive by
	// definition, so a constructed spec carrying a default must be told about
	// the credential even when nothing set Sensitive.
	if (s.Sensitive || section == SectionConnection) && s.HasDefault {
		what := "a sensitive value"
		if section == SectionConnection {
			// Nobody had to write `sensitive` for a connection, so name the
			// reason it is one.
			what = "a connection, which is always sensitive,"
		}
		add(CodeSensitiveDefault, "default", what+" must not carry a default: it would be committed to the manifest and written into the environment on start", "default", "sensitive")
	}

	if s.ConnType != "" && section != SectionConnection {
		add(CodeConnTypeOutsideConnections, "conn_type", "conn_type describes a connection, so it means nothing in "+string(section))
	}

	// enum and type = 'enum' each require the other, and each rule reads both.
	if !skipTypeCoherence {
		if len(s.Enum) > 0 && s.Type != TypeEnum {
			add(CodeEnumNeedsType, "enum", "enum needs type = \"enum\"", "enum", "type")
		}
		if s.Type == TypeEnum && len(s.Enum) == 0 {
			add(CodeEnumTypeNeedsValues, "type", "type = \"enum\" needs a non-empty enum", "type", "enum")
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
	// "snowflake" — and never to the connection itself. CheckValues formats
	// this string into a message a caller prints, so a whole AIRFLOW_CONN_
	// payload here produces a false mismatch and dumps the connection,
	// password included.
	//
	// The empty string means the kind could not be determined, which is not a
	// mismatch and is skipped.
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
