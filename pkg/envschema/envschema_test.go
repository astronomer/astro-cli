package envschema

import (
	"reflect"
	"testing"
)

// Check is callable without a TOML reader, which is the reason it exists: a
// writer building a ValueSpec from UI state and serializing it must obey the
// same rules, or it can emit a manifest the next `astro local start` refuses to
// load. Every case here constructs a spec directly, the way a writer does.
func TestValueSpecCheck(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spec    ValueSpec
		section Section
		want    []string // the Field of each problem, in order
	}{
		{
			name:    "a plain declaration is fine",
			spec:    ValueSpec{},
			section: SectionEnvVar,
		},
		{
			name:    "a sensitive value with a default",
			spec:    ValueSpec{Sensitive: true, Default: "hunter2", HasDefault: true},
			section: SectionEnvVar,
			want:    []string{"default"},
		},
		{
			// The rule a writer is most likely to trip.
			name:    "a connection that is not sensitive",
			spec:    ValueSpec{},
			section: SectionConnection,
			want:    []string{"sensitive"},
		},
		{
			name:    "a sensitive connection is fine",
			spec:    ValueSpec{Sensitive: true},
			section: SectionConnection,
		},
		{
			// Both fire and both are true: it is not marked sensitive, AND the
			// default it carries would be committed either way. The credential
			// rule keys on the section as well as the flag, so a spec built by
			// hand cannot carry one past it by leaving Sensitive unset.
			name:    "a connection with a default and no flag",
			spec:    ValueSpec{Default: "postgres://u:p@h/db", HasDefault: true},
			section: SectionConnection,
			want:    []string{"sensitive", "default"},
		},
		{
			name:    "a sensitive connection with a default",
			spec:    ValueSpec{Sensitive: true, Default: "postgres://u:p@h/db", HasDefault: true},
			section: SectionConnection,
			want:    []string{"default"},
		},
		{
			name:    "conn_type outside connections",
			spec:    ValueSpec{ConnType: "postgres"},
			section: SectionEnvVar,
			want:    []string{"conn_type"},
		},
		{
			name:    "conn_type on an airflow variable",
			spec:    ValueSpec{ConnType: "postgres"},
			section: SectionAirflowVariable,
			want:    []string{"conn_type"},
		},
		{
			name:    "conn_type on a connection is fine",
			spec:    ValueSpec{Sensitive: true, ConnType: "postgres"},
			section: SectionConnection,
		},
		{
			name:    "an unknown type",
			spec:    ValueSpec{Type: "integer"},
			section: SectionEnvVar,
			want:    []string{"type"},
		},
		{
			// A connection's resolved value is its conn_type, so nothing can
			// enforce a `type` here.
			name:    "a type on a connection",
			spec:    ValueSpec{Sensitive: true, Type: TypeURL},
			section: SectionConnection,
			want:    []string{"type"},
		},
		{
			name:    "an enum on a connection",
			spec:    ValueSpec{Sensitive: true, Enum: []string{"a", "b"}},
			section: SectionConnection,
			want:    []string{"enum"},
		},
		{
			// One message, and it is not the spelling: the problem is that a
			// connection declared a type at all.
			name:    "an unknown type on a connection",
			spec:    ValueSpec{Sensitive: true, Type: "enom"},
			section: SectionConnection,
			want:    []string{"type"},
		},
		{
			// Both refused on their own terms, and the coherence rule does not
			// also fire telling it to add `type = "enum"`.
			name:    "a type and an enum on a connection",
			spec:    ValueSpec{Sensitive: true, Type: TypeEnum, Enum: []string{"a"}},
			section: SectionConnection,
			want:    []string{"type", "enum"},
		},
		{
			name:    "a connection declaring neither is fine",
			spec:    ValueSpec{Sensitive: true, ConnType: "postgres"},
			section: SectionConnection,
		},
		{
			// The enum rules read a type already rejected, so they stay quiet.
			name:    "an unknown type beside an enum",
			spec:    ValueSpec{Type: "enom", Enum: []string{"a"}},
			section: SectionEnvVar,
			want:    []string{"type"},
		},
		{
			name:    "an enum without the enum type",
			spec:    ValueSpec{Enum: []string{"a"}},
			section: SectionEnvVar,
			want:    []string{"enum"},
		},
		{
			name:    "the enum type with no values",
			spec:    ValueSpec{Type: TypeEnum},
			section: SectionEnvVar,
			want:    []string{"type"},
		},
		{
			name:    "an enum and its type together are fine",
			spec:    ValueSpec{Type: TypeEnum, Enum: []string{"a", "b"}},
			section: SectionEnvVar,
		},
		{
			name:    "an empty type means string",
			spec:    ValueSpec{Type: ""},
			section: SectionEnvVar,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, p := range tc.spec.Check(tc.section) {
				got = append(got, p.Field)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Check fields = %v, want %v", got, tc.want)
			}
		})
	}
}

// Every problem names what it read, so a caller that already knows a field is
// broken can drop the rules that consulted it. Without it, a reader cannot tell
// "type = enum needs a non-empty enum" — which points at type and reads enum —
// from a rule about type alone.
func TestSpecProblemNamesWhatItRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  ValueSpec
		field string
		reads []string
	}{
		{
			name:  "enum needs its type",
			spec:  ValueSpec{Enum: []string{"a"}},
			field: "enum",
			reads: []string{"enum", "type"},
		},
		{
			name:  "the enum type needs values",
			spec:  ValueSpec{Type: TypeEnum},
			field: "type",
			reads: []string{"type", "enum"},
		},
		{
			name:  "a sensitive default reads both",
			spec:  ValueSpec{Sensitive: true, HasDefault: true},
			field: "default",
			reads: []string{"default", "sensitive"},
		},
		{
			name:  "a single-field rule reads only itself",
			spec:  ValueSpec{Type: "integer"},
			field: "type",
			reads: []string{"type"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := tc.spec.Check(SectionEnvVar)
			if len(problems) != 1 {
				t.Fatalf("want exactly one problem, got %+v", problems)
			}
			if problems[0].Field != tc.field {
				t.Errorf("Field = %q, want %q", problems[0].Field, tc.field)
			}
			if !reflect.DeepEqual(problems[0].Reads, tc.reads) {
				t.Errorf("Reads = %v, want %v", problems[0].Reads, tc.reads)
			}
		})
	}
}
