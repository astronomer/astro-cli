package envschema

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
)

// Violation reason strings, shared with the golden tests.
const (
	reasonRequired = "required but not set"
	reasonInt      = "expected an integer"
	reasonPort     = "expected a port (1-65535)"
)

// Validate checks the assembled values against the schema and returns every
// violation (empty when satisfied), sorted by section then key so the output
// is stable across the map-keyed schema. A required name that's absent is a
// violation; a present value that fails its type or enum check is a
// violation; a present-but-optional value is still checked (cheap, catches
// typos). A nil schema yields no violations.
//
// "Required" means present, not non-empty: a required name set to the empty
// string counts as satisfied (behavior lifted from Astro Desktop). Whether an
// empty required value should instead be a violation is an open product
// question.
//
// Lifted from Astro Desktop's envschema.Validate, adapted to the map-keyed
// Schema and the ValueType consts. Desktop's "enum" type became the Enum
// field, which constrains any base type: the type check runs first, then
// membership.
func Validate(s *Schema, v Values) []Violation {
	if s == nil {
		return nil
	}
	var out []Violation

	checkValues := func(section Section, specs map[string]ValueSpec, present map[string]string) {
		for key, spec := range specs {
			val, ok := present[key]
			if !ok {
				if spec.Required {
					out = append(out, Violation{Kind: ViolationMissing, Section: section, Key: key, Reason: reasonRequired})
				}
				continue
			}
			if reason := valueError(&spec, val); reason != "" {
				out = append(out, Violation{Kind: ViolationWrongType, Section: section, Key: key, Reason: reason})
			}
		}
	}
	checkValues(SectionEnvVar, s.EnvVars, v.EnvVars)
	checkValues(SectionAirflowVariable, s.AirflowVariables, v.AirflowVariables)

	for connID, spec := range s.Connections {
		connType, ok := v.Connections[connID]
		if !ok {
			if spec.Required {
				out = append(out, Violation{Kind: ViolationMissing, Section: SectionConnection, Key: connID, Reason: reasonRequired})
			}
			continue
		}
		if spec.ConnType != "" && connType != "" && connType != spec.ConnType {
			out = append(out, Violation{
				Kind:    ViolationWrongType,
				Section: SectionConnection,
				Key:     connID,
				Reason:  fmt.Sprintf("expected type %q, got %q", spec.ConnType, connType),
			})
		}
	}

	// Map iteration made the order random; findings must be stable.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// valueError returns a reason string if value doesn't satisfy spec, or ""
// if it's fine. Unknown / empty / string / json types are always accepted
// (json validity isn't enforced — values are often templated).
func valueError(spec *ValueSpec, value string) string {
	switch spec.Type {
	case TypeInt:
		if _, err := strconv.Atoi(value); err != nil {
			return reasonInt
		}
	case TypeNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return "expected a number"
		}
	case TypeBool:
		if _, err := strconv.ParseBool(value); err != nil {
			return "expected a boolean"
		}
	case TypePort:
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return reasonPort
		}
	case TypeURL:
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "expected a URL"
		}
	case TypeString, TypeJSON:
	}
	if len(spec.Enum) > 0 && !slices.Contains(spec.Enum, value) {
		return fmt.Sprintf("expected one of %v", spec.Enum)
	}
	return ""
}
