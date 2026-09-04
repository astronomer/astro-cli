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
// A value that is present, even the empty string, satisfies this function: it
// asks only whether anything is missing, and it is what `astro local start`
// gates on.
//
// Whether a present value is the SHAPE it was declared to be is CheckValues'
// question, in conform.go. A second function rather than a flag here, so a
// caller chooses; callers wanting both call both and concatenate, as
// internal/envresolve does. The other present-but-wrong case, a corrupt
// connection JSON, is a violation the resolver adds directly, since only it can
// decode.
func Validate(s *Schema, v Values) []Violation {
	if s == nil {
		return nil
	}
	var out []Violation

	check := func(section Section, names map[string]ValueSpec, present map[string]string) {
		for key := range names {
			// An optional declaration does not gate, and this is the only
			// annotation this function reads: Type, Enum and ConnType are
			// CheckValues' business.
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

	SortViolations(out)
	return out
}

// SortViolations orders findings by section then key, in place. Map iteration
// leaves the order random and every producer needs it stable.
//
// Exported because a caller concatenating Validate and CheckValues has to
// re-sort the result — joining two sorted slices does not give a sorted one —
// and internal/envresolve's missing-value report depends on that order. One
// definition, so the two cannot drift.
func SortViolations(out []Violation) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Key < out[j].Key
	})
}
