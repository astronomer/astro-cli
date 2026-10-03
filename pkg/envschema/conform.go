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
// A value that is present but empty IS checked, against the same rules as any
// other value — so a declared `type` means something for `PORT=` as well as for
// `PORT=abc`. That does not make the two functions disagree: Validate reads only
// `optional` and still counts an empty value as present, so nothing here can
// turn a project that starts into one that does not. A finding is a warning on
// the caller's side, which is the whole reason the type check lives beside
// Validate rather than inside it.
//
// The exception is a connection, whose value here is its resolved conn_type: an
// empty one is a kind the resolver could not determine, not the wrong kind.
//
// # What the conn_type check does not reach
//
// A connection is compared only when the resolver determined its kind, which
// means a value airflowenv.DecodeConnEnv accepted — JSON carrying a conn_type.
// Anything else leaves the kind empty and the empty-value skip passes over it,
// so it is never compared here:
//
//   - A URI value — AIRFLOW_CONN_X=snowflake://u:p@acct/db, Airflow's own
//     documented form — is not JSON and does not decode.
//   - A JSON blob omitting conn_type does not decode either, because a value
//     with no conn_type is not a connection.
//
// Both reach the user as the resolver's wrong-type violation naming the value,
// rather than one of them being silently accepted against any declaration. So
// neither reports a false mismatch, and a clean start is still not proof that a
// conn_type MATCHED — only that one was present and parsed.
func CheckValues(s *Schema, v Values) []Violation {
	if s == nil {
		return nil
	}
	var out []Violation

	check := func(section Section, names map[string]ValueSpec, present map[string]string) {
		for key := range names {
			value, ok := present[key]
			if !ok {
				continue
			}
			// An empty value is checked like any other, because CheckValue
			// already answers correctly for one: an absent type, `string` and
			// `json` are the absence of a constraint and accept it, while every
			// typed arm rejects it. So `type = 'port'` with PORT= now reports
			// what it always should have, and a name with no declared type is
			// untouched.
			//
			// Connections are the exception, and it is not stylistic: their
			// value here is the resolved conn_type, so an empty one is a kind
			// the resolver could not determine, not the wrong kind.
			if value == "" && section == SectionConnection {
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
// not the wrong kind. CheckValues skips an empty connection value for exactly
// that reason, and is where the behavior lives; it no longer skips an empty
// value in the other sections.
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
	// The offending value is quoted back, except for a secret declaration,
	// whose contents must never appear in a reason: callers put Reason on
	// stdout and into their JSON event stream, and embedded in prose it cannot
	// be redacted downstream. `{ secret = true, type = 'url' }` is legal —
	// only secret+default is refused — so this arm is reachable.
	//
	// It is quoted for everything else because "expected an integer" against a
	// schema of forty names does not say which value, and the caller reports
	// these with no value to hand.
	got := func() string {
		if s.Secret {
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
