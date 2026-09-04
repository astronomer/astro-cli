package envschema

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// CheckValues reports the ways the assembled values do not match what their
// declarations said they would be. It returns nil when every present value
// conforms, and judges only names the schema declares.
//
// Distinct from Validate, which asks only whether anything is MISSING and is
// what `astro local start` gates on. This asks whether what resolved is the
// shape it was declared to be, and a caller can report it without refusing to
// proceed. Callers wanting both call both.
//
// A value that is present but empty is not checked: an empty string satisfies
// Validate, so refusing it here would have the two functions disagree about a
// value the gate allows.
//
// # What the conn_type check does not reach
//
// A connection is compared only when the resolver determined its kind, which
// means a JSON value carrying a conn_type field. airflowenv.DecodeConnEnv is
// JSON-only, so:
//
//   - A URI value — AIRFLOW_CONN_X=snowflake://u:p@acct/db, Airflow's own
//     documented form — does not decode. The resolver reports "not valid
//     connection JSON" and leaves the kind empty, which the empty-value skip
//     passes over.
//   - A JSON blob omitting conn_type decodes to an empty kind and is accepted
//     against any declaration.
//
// Neither reports a false mismatch. Comparing them needs the resolver to
// distinguish "decoded, no kind" from "could not decode", so it is a change on
// that side. A clean start is therefore not proof that a conn_type matched.
func CheckValues(s *Schema, v Values) []Violation {
	if s == nil {
		return nil
	}
	var out []Violation

	check := func(section Section, names map[string]ValueSpec, present map[string]string) {
		for key := range names {
			value, ok := present[key]
			if !ok || value == "" {
				continue
			}
			// Indexed rather than ranged by value: ranging would make this
			// loop's legality depend on ValueSpec staying under a lint copy
			// threshold.
			spec := names[key]
			var reason string
			if section == SectionConnection {
				// A connection's value here is its conn_type, not its contents,
				// so the question is whether it is the declared KIND. Running
				// CheckValue on it would type-check the string "postgres"
				// against `type`, which is why Check refuses `type` and `enum`
				// on a connection.
				reason = connTypeMismatch(spec.ConnType, value)
			} else {
				reason = spec.CheckValue(value)
			}
			if reason != "" {
				out = append(out, Violation{
					Kind:    ViolationWrongType,
					Section: section,
					Key:     key,
					Reason:  reason,
				})
			}
		}
	}
	check(SectionEnvVar, s.EnvVars, v.EnvVars)
	check(SectionAirflowVariable, s.AirflowVariables, v.AirflowVariables)
	check(SectionConnection, s.Connections, v.Connections)

	SortViolations(out)
	return out
}

// connTypeMismatch reports a connection that resolved to a different kind than
// it declared. An undeclared conn_type accepts anything.
//
// It does not test resolved for "" — a kind the resolver could not determine is
// not the wrong kind. CheckValues' empty-value skip covers that case, and is
// where the behavior lives.
func connTypeMismatch(declared, resolved string) string {
	// Case-folded: DecodeConnEnv passes conn_type through verbatim and the
	// manifest side is verbatim TOML, so neither end normalizes, and a stored
	// "Postgres" against a declared "postgres" is the same connection.
	if declared == "" || strings.EqualFold(resolved, declared) {
		return ""
	}
	return fmt.Sprintf("declared conn_type %q but resolved to %q", declared, resolved)
}

// CheckValue reports why value does not satisfy this declaration's type, or ""
// when it does. It is the per-value half of CheckValues, on the type so that a
// caller holding one declaration and one value — a form field, an editor
// annotation — can ask without assembling a whole schema.
//
// TypeString and TypeJSON accept anything.
//
//nolint:gocritic // hugeParam: by value on purpose, see Check.
func (s ValueSpec) CheckValue(value string) string {
	// The offending value is quoted back, except for a sensitive declaration,
	// whose contents must never appear in a reason: callers put Reason on
	// stdout and into their JSON event stream, and embedded in prose it cannot
	// be redacted downstream. `{ sensitive = true, type = 'url' }` is legal —
	// only sensitive+default is refused — so this arm is reachable.
	//
	// It is quoted for everything else because "expected an integer" against a
	// schema of forty names does not say which value, and the caller reports
	// these with no value to hand.
	got := func() string {
		if s.Sensitive {
			return ""
		}
		return fmt.Sprintf(", got %q", value)
	}

	switch s.Type {
	case "", TypeString, TypeJSON:
		// Nothing to check. An absent type and `string` are the absence of a
		// constraint; JSON is a deliberate non-check, since these values are
		// routinely templated (`{{ var.value.x }}`) and are not valid JSON at
		// rest. Redundant with the return at the bottom, and explicit because
		// falling out of a switch reads as an oversight.
		return ""
	case TypeInt:
		// An explicit 64, not Atoi, whose width is the platform's int:
		// .goreleaser.yml ships a linux/386 build, and the verdict must not
		// depend on which artifact is running.
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return "expected an integer" + got()
		}
	case TypeNumber:
		// Non-finite values are refused, though ParseFloat accepts NaN, Inf,
		// +Inf and infinity: a NaN reaching Airflow makes every comparison
		// against it silently false. Contrast the bool arm, which keeps
		// ParseBool's wider set.
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "expected a finite number" + got()
		}
	case TypeBool:
		// ParseBool's set, which is wider than true/false: 1, t, T, TRUE, and
		// their false counterparts. Deliberately not narrowed, because it is
		// what Airflow's own env-var parsing accepts.
		if _, err := strconv.ParseBool(value); err != nil {
			return "expected a boolean" + got()
		}
	case TypePort:
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return "expected a port between 1 and 65535" + got()
		}
	case TypeURL:
		// Scheme AND host, because url.Parse alone rejects almost nothing: it
		// accepts a bare "example.com" as a relative path with no error.
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "expected a URL with a scheme and host" + got()
		}
	case TypeEnum:
		if len(s.Enum) == 0 {
			// An incomplete declaration, not a set admitting nothing. Check
			// refuses the pairing so a manifest cannot reach here, but a caller
			// holding a declaration mid-edit can, where `enum` chosen and the
			// values not yet typed is a normal intermediate state.
			return ""
		}
		for _, allowed := range s.Enum {
			if value == allowed {
				return ""
			}
		}
		return "expected one of " + strings.Join(quoteAll(s.Enum), ", ") + got()
	}
	return ""
}

// quoteAll quotes each value so an enum of empty or space-carrying strings reads
// unambiguously in the message.
func quoteAll(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = fmt.Sprintf("%q", v)
	}
	return out
}
