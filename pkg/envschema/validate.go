package envschema

import "sort"

// Violation reason strings, shared with the golden tests.
const reasonRequired = "required but not set"

// Validate checks the assembled values against the schema and returns every
// violation (empty when satisfied), sorted by section then key so the output
// is stable across the map-keyed schema. A declared name that resolved from
// nowhere — no file, no workspace, no default — is a missing violation. A nil
// schema yields no violations.
//
// A value that is present (even the empty string) satisfies its declaration:
// there is no type check. The one present-but-wrong case, a corrupt connection
// JSON, is a violation the resolver adds directly, since only it can decode.
func Validate(s *Schema, v Values) []Violation {
	if s == nil {
		return nil
	}
	var out []Violation

	check := func(section Section, names map[string]ValueSpec, present map[string]string) {
		for key := range names {
			// An optional declaration does not gate. This is the only thing the
			// flag does, and the only thing this function does with any
			// annotation: Type, Enum and ConnType are metadata for consumers,
			// not checks performed here — see this function's own doc above,
			// which says there is no type check.
			//
			// Indexed rather than ranged by value, and deliberately not because
			// of the struct's current size: ranging by value made this loop's
			// legality depend on ValueSpec staying under gocritic's copy
			// threshold, so the next field added to the struct would fail lint
			// HERE, in an unrelated change, pointing at this line instead of at
			// the field. Indexing costs nothing and removes the tripwire.
			if names[key].Optional {
				continue
			}
			if _, ok := present[key]; !ok {
				out = append(out, Violation{Kind: ViolationMissing, Section: section, Key: key, Reason: reasonRequired})
			}
		}
	}
	check(SectionEnvVar, s.EnvVars, v.EnvVars)
	check(SectionAirflowVariable, s.AirflowVariables, v.AirflowVariables)
	check(SectionConnection, s.Connections, v.Connections)

	// Map iteration made the order random; findings must be stable.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Key < out[j].Key
	})
	return out
}
