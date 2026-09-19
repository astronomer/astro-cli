package envschema

import (
	"errors"
	"strings"
	"testing"
)

// Which rule fired, asserted on the code rather than on the sentence.
//
// TestValueSpecCheck already holds which annotation each rule blames and in
// what order. What it could not say is WHICH rule blamed it: `sensitive` is
// the Field for two different refusals, and `type` for three. The code says
// which, so a caller can branch on it — and so this file can assert the rule
// without copying its prose into a second place, where the prose would have
// to be updated twice and, once these are translated, would pin English as
// though it were the contract.
func TestCheckReportsTheRuleThatFired(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spec    ValueSpec
		section Section
		want    []ProblemCode
	}{
		{
			name:    "a plain declaration is fine",
			spec:    ValueSpec{},
			section: SectionEnvVar,
		},
		{
			name:    "a sensitive value with a default",
			spec:    ValueSpec{Sensitive: true, HasDefault: true},
			section: SectionEnvVar,
			want:    []ProblemCode{CodeSensitiveDefault},
		},
		{
			// A connection reaches the same rule without anybody writing
			// `sensitive`, which is why the two share one code and the
			// sentence differs.
			name:    "a connection with a default",
			spec:    ValueSpec{Sensitive: true, HasDefault: true},
			section: SectionConnection,
			want:    []ProblemCode{CodeSensitiveDefault},
		},
		{
			// Field is "sensitive" for both of these; only the code tells
			// them apart.
			name:    "a connection declared not sensitive",
			spec:    ValueSpec{HasSensitive: true},
			section: SectionConnection,
			want:    []ProblemCode{CodeConnectionNotSensitive},
		},
		{
			name:    "a connection declared sensitive",
			spec:    ValueSpec{Sensitive: true, HasSensitive: true},
			section: SectionConnection,
			want:    []ProblemCode{CodeConnectionSensitiveRedundant},
		},
		{
			name:    "a type on a connection",
			spec:    ValueSpec{Sensitive: true, Type: TypeString},
			section: SectionConnection,
			want:    []ProblemCode{CodeTypeOnConnection},
		},
		{
			name:    "an enum on a connection",
			spec:    ValueSpec{Sensitive: true, Enum: []string{"a"}},
			section: SectionConnection,
			want:    []ProblemCode{CodeEnumOnConnection},
		},
		{
			// Both, and each on its own terms: the pair rules are skipped so
			// the connection is not also told to add the type it may not
			// declare.
			name:    "a type and an enum on a connection",
			spec:    ValueSpec{Sensitive: true, Type: TypeString, Enum: []string{"a"}},
			section: SectionConnection,
			want:    []ProblemCode{CodeTypeOnConnection, CodeEnumOnConnection},
		},
		{
			name:    "conn_type outside connections",
			spec:    ValueSpec{ConnType: "postgres"},
			section: SectionEnvVar,
			want:    []ProblemCode{CodeConnTypeOutsideConnections},
		},
		{
			// Field is "type" here and for the two enum-coherence rules
			// below; again only the code distinguishes them.
			name:    "an unknown type",
			spec:    ValueSpec{Type: "magenta"},
			section: SectionEnvVar,
			want:    []ProblemCode{CodeUnknownType},
		},
		{
			name:    "an enum without the enum type",
			spec:    ValueSpec{Enum: []string{"a"}},
			section: SectionEnvVar,
			want:    []ProblemCode{CodeEnumNeedsType},
		},
		{
			name:    "the enum type with no values",
			spec:    ValueSpec{Type: TypeEnum},
			section: SectionEnvVar,
			want:    []ProblemCode{CodeEnumTypeNeedsValues},
		},
		{
			// An unknown type beside an enum reports the spelling only: the
			// coherence rules would otherwise contradict what the author
			// wrote.
			name:    "an unknown type beside an enum",
			spec:    ValueSpec{Type: "magenta", Enum: []string{"a"}},
			section: SectionEnvVar,
			want:    []ProblemCode{CodeUnknownType},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []ProblemCode
			for _, p := range tc.spec.Check(tc.section) {
				got = append(got, p.Code)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("codes = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("code %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// Every problem carries a code, and no two rules share one.
//
// Without this the field is optional in practice: a rule added with the code
// left off still compiles, still reports, and is indistinguishable to a
// caller from one that has no rule at all. And two rules sharing a code would
// make a caller's branch fire on the wrong one — the failure a code exists to
// prevent, reintroduced by a copy-paste.
func TestEveryRuleHasItsOwnCode(t *testing.T) {
	seen := map[ProblemCode]string{}
	for _, tc := range checkFixtures() {
		for _, p := range tc.spec.Check(tc.section) {
			if p.Code == "" {
				t.Errorf("a problem on %q carries no code: %q", p.Field, p.Reason)
				continue
			}
			if prior, dup := seen[p.Code]; dup && prior != p.Reason {
				t.Errorf("code %q is used by two rules:\n  %s\n  %s", p.Code, prior, p.Reason)
			}
			seen[p.Code] = p.Reason
		}
	}
}

// A rule that Check found keeps its code on the way out.
//
// The package reports two types. Check returns SpecProblems; ParseSchema
// returns Problems, and checkSpec copies one into the other — so that copy is
// where a code can quietly go missing, and it did: SpecProblem carried a code
// before Problem had a field to put it in, and every spec-level refusal
// reached its caller with the code dropped.
//
// TestEveryCodeIsReachable cannot see this. It collects from Check as well as
// from ParseSchema, so a code Check produces counts as raised whether or not
// the copy preserves it. This drives the same rules through ParseSchema alone
// and asks what came out the other end.
func TestSpecProblemCodesSurviveParse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    ProblemCode
	}{
		{
			name:    "a sensitive value with a default",
			content: "[tool.astro.env]\nAPI_TOKEN = { sensitive = true, default = 'v' }\n",
			want:    CodeSensitiveDefault,
		},
		{
			name:    "type on a connection",
			content: "[tool.astro.env.connections]\nwarehouse = { type = 'string' }\n",
			want:    CodeTypeOnConnection,
		},
		{
			name:    "conn_type outside the connections section",
			content: "[tool.astro.env]\nDB = { conn_type = 'postgres' }\n",
			want:    CodeConnTypeOutsideConnections,
		},
		{
			name:    "an unknown type",
			content: "[tool.astro.env]\nLEVEL = { type = 'magenta' }\n",
			want:    CodeUnknownType,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema(decodeEnv(t, tc.content))
			var se *SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("want *SchemaError, got %v", err)
			}
			var got []ProblemCode
			for _, p := range se.Problems {
				got = append(got, p.Code)
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("codes = %v, want exactly [%s]", got, tc.want)
			}
		})
	}
}

// Every declared code is one something actually produces.
//
// A code nothing raises is either a rule deleted out from under it or one
// whose call site never fires, and both are invisible while the only checks on
// the list are that its members are spelled well. Both halves of the package
// report findings — Check judges a decoded declaration, ParseSchema decodes
// one — so both are driven here; the parse half had no code at all until the
// review that prompted this, which is the gap the test exists to keep shut.
func TestEveryCodeIsReachable(t *testing.T) {
	raised := map[ProblemCode]bool{}
	for _, tc := range checkFixtures() {
		for _, p := range tc.spec.Check(tc.section) {
			raised[p.Code] = true
		}
	}
	for _, content := range parseFixtures {
		_, err := ParseSchema(decodeEnv(t, content))
		var se *SchemaError
		if !errors.As(err, &se) {
			t.Errorf("fixture reported nothing:\n%s", content)
			continue
		}
		for _, p := range se.Problems {
			raised[p.Code] = true
		}
	}
	for _, code := range problemCodes {
		if !raised[code] {
			t.Errorf("no fixture raises %q", code)
		}
	}
}

// checkFixtures are specs chosen to make Check produce every message it has.
func checkFixtures() []struct {
	spec    ValueSpec
	section Section
} {
	return []struct {
		spec    ValueSpec
		section Section
	}{
		{ValueSpec{Sensitive: true, HasDefault: true}, SectionEnvVar},
		{ValueSpec{HasSensitive: true}, SectionConnection},
		{ValueSpec{Sensitive: true, HasSensitive: true}, SectionConnection},
		{ValueSpec{Sensitive: true, Type: TypeString}, SectionConnection},
		{ValueSpec{Sensitive: true, Enum: []string{"a"}}, SectionConnection},
		{ValueSpec{ConnType: "postgres"}, SectionEnvVar},
		{ValueSpec{Type: "magenta"}, SectionEnvVar},
		{ValueSpec{Enum: []string{"a"}}, SectionEnvVar},
		{ValueSpec{Type: TypeEnum}, SectionEnvVar},
	}
}

// parseFixtures are sections chosen to make ParseSchema report every refusal
// it has. One declaration per line, each tripping a different rule.
var parseFixtures = []string{
	`
[tool.astro.env]
"BAD-NAME" = {}
NOTVALUE = 5
TYPO = { typo = true }
BADSRC = { source = 'cloud' }
EMPTYTYPE = { type = '' }
EMPTYENUM = { type = 'enum', enum = [] }
NOTBOOL = { sensitive = 'yes' }
NOTARRAY = { enum = 3 }
NOTSTRINGS = { type = 'enum', enum = ['a', 3] }
NOTSCALAR = { default = [1] }
NOTSTRING = { description = 5 }
`,
	`
[tool.astro.env]
connections = 3
`,
	`
[tool.astro.env.connections]
c = { conn_type = '' }
`,
}

// A code is a stable identifier, so it is spelled like one.
//
// Not a check of taste: a caller writes these into a config file or a jq
// filter, and a code carrying a space or a capital is one somebody will quote
// wrongly. Lowercase words joined by underscores, which is what the existing
// ones are.
func TestCodesAreSpelledLikeIdentifiers(t *testing.T) {
	for _, code := range problemCodes {
		got := string(code)
		switch {
		case got == "":
			t.Error("an empty code")
		case got != strings.ToLower(got):
			t.Errorf("%q should be lowercase", got)
		case strings.ContainsAny(got, " \t-."):
			t.Errorf("%q should join its words with underscores", got)
		}
	}
}

// No two codes share a value.
func TestCodesAreDistinct(t *testing.T) {
	seen := map[ProblemCode]bool{}
	for _, code := range problemCodes {
		if seen[code] {
			t.Errorf("%q is declared twice", code)
		}
		seen[code] = true
	}
}
