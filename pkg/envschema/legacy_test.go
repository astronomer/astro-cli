package envschema

import (
	"testing"
)

const legacyFile = `env_vars:
  - { key: API_URL, type: url, required: true }
  - { key: BATCH_SIZE, type: int, default: "100" }
  - { key: SAYS_NOTHING }
  - { key: OPENAI_API_KEY, type: string, required: true, sensitive: true }
airflow_variables:
  - { key: region, type: string, required: true }
connections:
  - { conn_id: warehouse, conn_type: snowflake, required: true }
  - { conn_id: crm, conn_type: salesforce }
`

func TestParseLegacyCarriesEveryField(t *testing.T) {
	s, err := ParseLegacy([]byte(legacyFile))
	if err != nil {
		t.Fatal(err)
	}

	api := s.EnvVars["API_URL"]
	if api.Type != TypeURL || api.Optional {
		t.Errorf("API_URL = %+v, want a gated url", api)
	}
	batch := s.EnvVars["BATCH_SIZE"]
	if batch.Default != "100" || !batch.HasDefault {
		t.Errorf("BATCH_SIZE default = %+v", batch)
	}
	key := s.EnvVars["OPENAI_API_KEY"]
	if !key.Sensitive {
		t.Errorf("OPENAI_API_KEY lost sensitive: %+v", key)
	}
	if got := s.AirflowVariables["region"]; got.Optional || got.Type != TypeString {
		t.Errorf("region = %+v", got)
	}
	if got := s.Connections["warehouse"]; got.ConnType != "snowflake" || got.Optional {
		t.Errorf("warehouse = %+v", got)
	}
}

// The row nearly every declaration is on. `required` and `optional` are
// opposite flags whose ZERO VALUES disagree, so a declaration that spells
// neither is not gated by the v1 file and would be gated by the manifest.
// Carrying the field across by name turns a project's documented-but-optional
// names into ones it refuses to start without.
func TestParseLegacyInvertsTheGateRatherThanRenamingIt(t *testing.T) {
	s, err := ParseLegacy([]byte(legacyFile))
	if err != nil {
		t.Fatal(err)
	}

	// Spelled nothing: not gated by the file, so not gated after the carry.
	if !s.EnvVars["SAYS_NOTHING"].Optional {
		t.Fatal("the flag was carried across rather than inverted")
	}
	// Spelled required: gated on both sides.
	if s.EnvVars["API_URL"].Optional {
		t.Error("a required declaration stopped gating")
	}
	// And the mistake this guards has a visible cost: under a rename the
	// unspelled one gates.
	renamed := ValueSpec{Optional: !s.EnvVars["SAYS_NOTHING"].Optional}
	got := Validate(&Schema{EnvVars: map[string]ValueSpec{"SAYS_NOTHING": renamed}}, Values{})
	if len(got) == 0 {
		t.Error("a rename no longer changes gating — if a zero value moved, " +
			"re-derive this conversion rather than deleting the test")
	}
}

// The v1 file has no `sensitive` for a connection and the manifest grammar
// refuses one declared otherwise, so every carried connection has to arrive
// sensitive or Check rejects it.
func TestParseLegacyMakesEveryConnectionSensitive(t *testing.T) {
	s, err := ParseLegacy([]byte(legacyFile))
	if err != nil {
		t.Fatal(err)
	}
	for id, spec := range s.Connections {
		if !spec.Sensitive {
			t.Errorf("connection %q is not sensitive: %+v", id, spec)
		}
		if p := spec.Check(SectionConnection); len(p) > 0 {
			t.Errorf("connection %q would be refused by the grammar: %+v", id, p)
		}
	}
}

// Everything the reader produces from an ordinary file is legal to write, so a
// conversion carrying it cannot render a manifest that will not read back.
func TestParseLegacyProducesWritableDeclarations(t *testing.T) {
	s, err := ParseLegacy([]byte(legacyFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		section Section
		specs   map[string]ValueSpec
	}{
		{SectionEnvVar, s.EnvVars},
		{SectionAirflowVariable, s.AirflowVariables},
		{SectionConnection, s.Connections},
	} {
		for name, spec := range tc.specs {
			if p := spec.Check(tc.section); len(p) > 0 {
				t.Errorf("%s %q: %+v", tc.section, name, p)
			}
		}
	}
}

// A v1 file can say things the manifest grammar refuses. The reader carries
// them through verbatim so the conversion can REPORT them; dropping them here
// would leave the caller unable to tell the user what was lost.
func TestParseLegacyCarriesWhatTheGrammarWillRefuse(t *testing.T) {
	s, err := ParseLegacy([]byte(`env_vars:
  - { key: TOKEN, sensitive: true, default: "abc" }
`))
	if err != nil {
		t.Fatal(err)
	}
	spec := s.EnvVars["TOKEN"]
	if !spec.Sensitive || !spec.HasDefault {
		t.Fatalf("the pairing was not carried: %+v", spec)
	}
	if p := spec.Check(SectionEnvVar); len(p) == 0 {
		t.Error("a sensitive value with a default should be refused, so the " +
			"conversion has something to report")
	}
}

// An unknown key is a typo or a guessed spelling in authored config, and dropping it silently is how
// a declaration disappears in a conversion nobody could review.
func TestParseLegacyRefusesAnUnknownKey(t *testing.T) {
	if _, err := ParseLegacy([]byte("env_vars:\n  - { key: A, mandatory: true }\n")); err == nil {
		t.Error("a misspelled key was accepted")
	}
}

func TestParseLegacyOnAnEmptyFile(t *testing.T) {
	s, err := ParseLegacy([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || len(s.EnvVars)+len(s.AirflowVariables)+len(s.Connections) != 0 {
		t.Errorf("want an empty schema, got %+v", s)
	}
}

// The two readers differ in exactly one thing, and the difference is not
// stylistic: the strict one is about to delete the file it just read.
func TestParseLegacyTolerantIgnoresAnUnknownKey(t *testing.T) {
	const typo = "env_vars:\n  - { key: A, mandatory: true }\n"

	if _, err := ParseLegacy([]byte(typo)); err == nil {
		t.Error("the strict reader accepted a typo, so a conversion would drop the declaration silently")
	}

	s, err := ParseLegacyTolerant([]byte(typo))
	if err != nil {
		t.Fatalf("the tolerant reader refused a file it only reads: %v", err)
	}
	if _, found := s.EnvVars["A"]; !found {
		t.Errorf("the declaration itself was lost: %+v", s.EnvVars)
	}
	// And the shared conversion still ran: A spelled no usable gate flag, so it
	// is not gated.
	if !s.EnvVars["A"].Optional {
		t.Error("the tolerant reader skipped the gate inversion")
	}
}

// The v1 format is a LIST and this is a map, so a repeated key keeps one entry
// and loses the other. KnownFields cannot see it: every field is spelled right.
// The strict reader's caller deletes the file, so the lost declaration would be
// unrecoverable.
func TestParseLegacyRefusesARepeatedKey(t *testing.T) {
	for _, body := range []string{
		"env_vars:\n  - { key: API_URL, type: url, required: true }\n  - { key: API_URL }\n",
		"airflow_variables:\n  - { key: region }\n  - { key: region, type: string }\n",
		"connections:\n  - { conn_id: warehouse }\n  - { conn_id: warehouse, conn_type: snowflake }\n",
	} {
		if _, err := ParseLegacy([]byte(body)); err == nil {
			t.Errorf("a repeated key was accepted, so one declaration would be lost: %s", body)
		}
	}
}

// An entry with no key lands under the empty name, which the manifest grammar
// then refuses as an illegal env-var name — failing the whole section.
func TestParseLegacyRefusesAKeylessEntry(t *testing.T) {
	if _, err := ParseLegacy([]byte("env_vars:\n  - { type: string }\n")); err == nil {
		t.Error("an entry with no key was accepted")
	}
}

// The tolerant reader takes what it can from the same faults, because its
// caller only displays the result and deletes nothing.
func TestParseLegacyTolerantTakesWhatItCan(t *testing.T) {
	s, err := ParseLegacyTolerant([]byte(
		"env_vars:\n  - { key: API_URL, type: url }\n  - { key: API_URL }\n  - { type: string }\n"))
	if err != nil {
		t.Fatalf("the tolerant reader refused a file it only reads: %v", err)
	}
	if len(s.EnvVars) != 1 {
		t.Errorf("want the one named declaration, got %+v", s.EnvVars)
	}
	if _, found := s.EnvVars[""]; found {
		t.Error("a keyless entry became a declaration under the empty name")
	}
}

// CheckName is a separate rule from ValueSpec.Check, which judges annotations
// and never sees what the declaration is called. The parser refuses an illegal
// name by failing the WHOLE section, so a caller that writes one and deletes
// its source has made every other declaration unreadable.
func TestCheckNameRefusesWhatTheParserRefuses(t *testing.T) {
	refused := []struct {
		section Section
		name    string
	}{
		{SectionEnvVar, "MY-VAR"},
		{SectionEnvVar, "1ST"},
		{SectionEnvVar, ""},
		{SectionEnvVar, "batch.size"},
		// Directly under [tool.astro.env] these two name the sub-sections.
		{SectionEnvVar, "connections"},
		{SectionEnvVar, "airflow_variables"},
		{SectionAirflowVariable, "batch.size"},
		{SectionAirflowVariable, "2nd"},
		{SectionConnection, "my-warehouse"},
	}
	for _, tc := range refused {
		if err := CheckName(tc.section, tc.name); err == nil {
			t.Errorf("%s %q was accepted", tc.section, tc.name)
		}
	}

	accepted := []struct {
		section Section
		name    string
	}{
		{SectionEnvVar, "API_URL"},
		{SectionEnvVar, "KeepMyCase"},
		{SectionAirflowVariable, "region"},
		{SectionAirflowVariable, "_2nd"},
		{SectionConnection, "warehouse"},
		// The reserved names only collide for a plain env var: as a section's
		// own key they are ordinary.
		{SectionConnection, "connections"},
		{SectionAirflowVariable, "airflow_variables"},
	}
	for _, tc := range accepted {
		if err := CheckName(tc.section, tc.name); err != nil {
			t.Errorf("%s %q was refused: %v", tc.section, tc.name, err)
		}
	}
}

// The claim CheckName exists for: everything it accepts, ParseSchema reads
// back. Asserted against the parser itself rather than a copy of its rule.
func TestEveryNameCheckNameAcceptsParsesBack(t *testing.T) {
	for _, tc := range []struct {
		section Section
		name    string
		sub     string
	}{
		{SectionEnvVar, "API_URL", ""},
		{SectionEnvVar, "KeepMyCase", ""},
		{SectionAirflowVariable, "region", "airflow_variables"},
		{SectionConnection, "warehouse", "connections"},
	} {
		if err := CheckName(tc.section, tc.name); err != nil {
			t.Fatalf("%s %q: %v", tc.section, tc.name, err)
		}
		decl := map[string]any{tc.name: map[string]any{}}
		env := decl
		if tc.sub != "" {
			env = map[string]any{tc.sub: decl}
		}
		if _, err := ParseSchema(env); err != nil {
			t.Errorf("%s %q was accepted but does not parse: %v", tc.section, tc.name, err)
		}
	}
}
