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
// Separate from Validate on purpose, and the split is the whole point of this
// file. Validate answers "is anything missing" and is what `astro local start`
// gates on; a start that refuses an unusual-but-working value is a worse failure
// than one that runs. This answers "is what resolved the shape it was declared
// to be", which a caller can report without refusing to proceed. Two questions,
// two functions, so a caller picks rather than inherits.
//
// Why it exists at all: `type`, `enum` and `conn_type` were parsed, validated
// for coherence, and then enforced by nothing on this side — the desktop
// type-checked resolved values and the CLI did not, so a project declaring
// `type = "port"` with `PORT=99999` got a clean start here and a violation
// there. One contract, two answers. A declared type that nothing enforces is
// worse than no type, because the author believes it is enforced.
//
// A value that is present but empty is NOT checked. An empty string satisfies
// Validate (present is present), and type-checking it would refuse a value the
// gate above deliberately allowed — an inconsistency between the two functions
// rather than a finding about the project.
//
// # What the conn_type check does not reach
//
// A connection is compared only when the resolver could determine its kind, and
// today that means one encoding. airflowenv.DecodeConnEnv is JSON-only, so:
//
//   - A URI value — AIRFLOW_CONN_X=snowflake://u:p@acct/db, Airflow's own
//     documented form — does not decode. The resolver reports "not valid
//     connection JSON" and leaves the kind empty, which the empty-value skip
//     passes over, so conn_type is never compared.
//   - A JSON blob that omits conn_type decodes to an empty kind and is
//     accepted against any declaration, even though the resolver DID read the
//     value and there simply is no kind in it.
//
// Both are silent rather than wrong: nothing is falsely reported. Closing them
// needs a signal the resolver does not pass today — "decoded, no kind" has to
// be distinguishable from "could not decode" — so it is a change on that side
// rather than here. Documented so a reader does not conclude from a passing
// start that their conn_type was checked.
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
			// Indexed rather than ranged by value, for the reason
			// validate.go's own loop records: ranging by value would make this
			// loop's legality depend on ValueSpec staying under a lint copy
			// threshold, so the next field added to the struct fails lint here,
			// in an unrelated change.
			spec := names[key]
			var reason string
			if section == SectionConnection {
				// A connection's "value" here is its conn_type, not its
				// contents — the resolver reduces it to that before handing it
				// over. So the question is whether it is the KIND of connection
				// declared, which is not the question CheckValue answers, and
				// running CheckValue on it would type-check the string
				// "postgres" against `type`. Check refuses `type` and `enum` on
				// a connection for the same reason.
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
// It deliberately does NOT test resolved for "" — the resolver failing to
// determine a kind is not the same as the wrong kind and must not be reported as
// one. CheckValues' empty-value skip already covers that, and a second guard
// here was unreachable: mutation testing found removing it changed no test,
// because nothing can reach this function with an empty resolved value. The skip
// is what holds the behavior, and it has a test of its own.
func connTypeMismatch(declared, resolved string) string {
	// Case-folded. DecodeConnEnv lowercases the connection ID but passes
	// conn_type through verbatim, and the manifest side is verbatim TOML, so
	// neither end normalizes: a stored "Postgres" against a declared "postgres"
	// is the same working connection and was reported as the wrong kind on
	// every start. That is the false-positive class this file's own rationale
	// exists to avoid.
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
// TypeString and TypeJSON accept anything. String is the absence of a
// constraint; JSON is deliberate rather than an omission — these values are
// routinely templated (`{{ var.value.x }}`), so a value that is not valid JSON
// at rest is normal and refusing it would fail the common case.
//
//nolint:gocritic // hugeParam: by value on purpose, see Check.
func (s ValueSpec) CheckValue(value string) string {
	// The offending value is quoted back for everything EXCEPT a sensitive
	// declaration, whose contents must not appear in a reason at all.
	//
	// This is not a hypothetical. `{ sensitive = true, type = 'url' }` is a
	// legal declaration — only sensitive+default is refused — and the value
	// resolves from a vault or the shell. Callers put Reason straight on stdout
	// and into their JSON event stream, so quoting it there wrote a live
	// credential into the terminal, into CI logs, and into anything consuming
	// the stream. Embedded in prose it could not even be redacted downstream.
	//
	// Naming the value is worth real usability, which is why it is kept for the
	// rest: "expected an integer" against a schema of forty names does not say
	// which value, and the caller reports these with no value to hand. For a
	// sensitive one the type alone has to do, and the author knows what they
	// set.
	got := func() string {
		if s.Sensitive {
			return ""
		}
		return fmt.Sprintf(", got %q", value)
	}

	switch s.Type {
	case "", TypeString, TypeJSON:
		// Nothing to check, and for two different reasons. An absent type and
		// `string` are the absence of a constraint. JSON is a deliberate
		// non-check: these values are routinely templated
		// (`{{ var.value.x }}`), so a value that is not valid JSON at rest is
		// normal, and refusing it would fail the common case.
		//
		// Behaviourally redundant with the return at the bottom, and kept
		// anyway: falling out of a switch reads as an oversight, and it is the
		// difference the `exhaustive` linter asks for. No test can distinguish
		// this arm from its absence — do not go looking for one.
		return ""
	case TypeInt:
		// ParseInt with an explicit 64, not Atoi, whose width is the platform's
		// int. .goreleaser.yml ships a linux/386 build, where Atoi makes
		// "3000000000" out of range — so the same manifest was clean on
		// amd64 and warned on 386. A type check whose verdict depends on which
		// release artifact you downloaded is the "one contract, two answers"
		// split this file exists to close.
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return "expected an integer" + got()
		}
	case TypeNumber:
		// ParseFloat accepts NaN, Inf, +Inf and infinity. Those are refused
		// rather than inherited: a NaN reaching Airflow makes every comparison
		// against it false, silently, which is a worse outcome than the warning
		// this returns. Contrast the bool arm below, which keeps ParseBool's
		// wider set deliberately.
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
			// An incomplete declaration, not a set that admits nothing. Check
			// refuses this pairing, so a manifest cannot reach here — but
			// CheckValue is advertised for a caller holding one declaration
			// mid-edit (a form field), where `enum` selected and the values not
			// yet typed is a normal intermediate state. Refusing every value
			// with "expected one of , got …" would fire on every keystroke.
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
