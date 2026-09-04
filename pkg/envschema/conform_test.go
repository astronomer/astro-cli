package envschema

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  ValueSpec
		value string
		ok    bool
	}{
		{name: "no type accepts anything", spec: ValueSpec{}, value: "anything at all", ok: true},
		{name: "string accepts anything", spec: ValueSpec{Type: TypeString}, value: "!!", ok: true},
		// Not an omission: these values are routinely templated, so a value
		// that is not valid JSON at rest is normal.
		{name: "json accepts anything", spec: ValueSpec{Type: TypeJSON}, value: "{{ var.value.x }}", ok: true},

		{name: "int accepts an integer", spec: ValueSpec{Type: TypeInt}, value: "3", ok: true},
		{name: "int accepts a negative", spec: ValueSpec{Type: TypeInt}, value: "-3", ok: true},
		{name: "int refuses a word", spec: ValueSpec{Type: TypeInt}, value: "abc"},
		{name: "int refuses a float", spec: ValueSpec{Type: TypeInt}, value: "3.5"},
		// Wider than a 32-bit int on purpose: with strconv.Atoi this was out of
		// range on the linux/386 build .goreleaser.yml ships, so the same
		// manifest warned there and passed on amd64.
		//
		// Honest about what this case can prove: on a 64-bit host Atoi accepts
		// it too, so it does NOT discriminate the fix here — it documents the
		// intent, and only a 386 run distinguishes them. The 64-bit boundary
		// below is the part that holds everywhere: it pins the explicit width,
		// so narrowing ParseInt's bitSize fails it.
		{name: "int accepts a value wider than a 32-bit int", spec: ValueSpec{Type: TypeInt}, value: "3000000000", ok: true},
		{name: "int refuses a value wider than 64 bits", spec: ValueSpec{Type: TypeInt}, value: "99999999999999999999"},

		{name: "number accepts a float", spec: ValueSpec{Type: TypeNumber}, value: "3.5", ok: true},
		{name: "number accepts an integer", spec: ValueSpec{Type: TypeNumber}, value: "3", ok: true},
		{name: "number refuses a word", spec: ValueSpec{Type: TypeNumber}, value: "abc"},
		// ParseFloat accepts all of these; a NaN reaching Airflow makes every
		// comparison against it silently false, which is worse than a warning.
		{name: "number refuses NaN", spec: ValueSpec{Type: TypeNumber}, value: "NaN"},
		{name: "number refuses Inf", spec: ValueSpec{Type: TypeNumber}, value: "Inf"},
		{name: "number refuses +Inf", spec: ValueSpec{Type: TypeNumber}, value: "+Inf"},
		{name: "number refuses infinity", spec: ValueSpec{Type: TypeNumber}, value: "infinity"},
		{name: "number refuses -Inf", spec: ValueSpec{Type: TypeNumber}, value: "-Inf"},

		{name: "bool accepts true", spec: ValueSpec{Type: TypeBool}, value: "true", ok: true},
		// ParseBool's wider set, deliberately kept: it is what Airflow's own
		// env-var parsing accepts.
		{name: "bool accepts 1", spec: ValueSpec{Type: TypeBool}, value: "1", ok: true},
		{name: "bool accepts TRUE", spec: ValueSpec{Type: TypeBool}, value: "TRUE", ok: true},
		{name: "bool refuses maybe", spec: ValueSpec{Type: TypeBool}, value: "maybe"},

		{name: "port accepts 8080", spec: ValueSpec{Type: TypePort}, value: "8080", ok: true},
		{name: "port accepts 1", spec: ValueSpec{Type: TypePort}, value: "1", ok: true},
		{name: "port accepts 65535", spec: ValueSpec{Type: TypePort}, value: "65535", ok: true},
		{name: "port refuses 0", spec: ValueSpec{Type: TypePort}, value: "0"},
		{name: "port refuses 65536", spec: ValueSpec{Type: TypePort}, value: "65536"},
		{name: "port refuses a word", spec: ValueSpec{Type: TypePort}, value: "http"},

		{name: "url accepts an https url", spec: ValueSpec{Type: TypeURL}, value: "https://x.io/y", ok: true},
		{name: "url accepts a postgres url", spec: ValueSpec{Type: TypeURL}, value: "postgres://u:p@h:5432/db", ok: true},
		// The reason scheme AND host are both required: url.Parse alone accepts
		// a bare host as a relative path and returns no error, so checking only
		// err would pass nearly everything.
		{name: "url refuses a bare host", spec: ValueSpec{Type: TypeURL}, value: "example.com"},
		{name: "url refuses a path", spec: ValueSpec{Type: TypeURL}, value: "/just/a/path"},
		{name: "url refuses a scheme with no host", spec: ValueSpec{Type: TypeURL}, value: "https://"},
		// The mirror of the case above, and the one that was missing: a host
		// with no scheme. Mutation testing found the scheme half of the check
		// untested, because every other refused fixture lacked a host too.
		{name: "url refuses a protocol-relative url", spec: ValueSpec{Type: TypeURL}, value: "//example.com/x"},

		{name: "enum accepts a member", spec: ValueSpec{Type: TypeEnum, Enum: []string{"a", "b"}}, value: "a", ok: true},
		{name: "enum refuses a non-member", spec: ValueSpec{Type: TypeEnum, Enum: []string{"a", "b"}}, value: "c"},
		{name: "enum is case sensitive", spec: ValueSpec{Type: TypeEnum, Enum: []string{"a"}}, value: "A"},
		// An incomplete declaration, not a set admitting nothing. Check refuses
		// the pairing, so a manifest cannot reach it — but CheckValue is
		// advertised for a caller holding a declaration mid-edit, where `enum`
		// chosen and the values not yet typed is a normal intermediate state.
		{name: "the enum type with no values constrains nothing", spec: ValueSpec{Type: TypeEnum}, value: "anything", ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := tc.spec.CheckValue(tc.value)
			if tc.ok && reason != "" {
				t.Errorf("CheckValue(%q) = %q, want no complaint", tc.value, reason)
			}
			if !tc.ok {
				if reason == "" {
					t.Fatalf("CheckValue(%q) accepted a value it should refuse", tc.value)
				}
				// The offending value belongs in the message: a bare "expected
				// an integer" against a schema of forty names does not say
				// which value, and the caller reports these without the value
				// to hand.
				if !strings.Contains(reason, tc.value) {
					t.Errorf("reason %q does not quote the offending value %q", reason, tc.value)
				}

				// ...unless the declaration is sensitive, and this asserts it
				// for EVERY refusing type rather than one sample, because the
				// leak it guards was systemic: the value was formatted into
				// every arm's message.
				sens := tc.spec
				sens.Sensitive = true
				sreason := sens.CheckValue(tc.value)
				if sreason == "" {
					t.Errorf("a sensitive declaration still has to refuse %q", tc.value)
				}
				// Asserted on the mechanism rather than on the value: a
				// substring test cannot be used here, because a one-character
				// fixture like the enum's "c" appears inside the word
				// "expected" and reports a leak that is not one. The invariant
				// is that no `, got "…"` clause is appended at all.
				if strings.Contains(sreason, ", got ") {
					t.Errorf("LEAK: sensitive reason %q still appends the value clause", sreason)
				}
			}
		})
	}
}

// A sensitive value's contents never reach a Violation, which is what a caller
// prints and streams as JSON.
//
// `{ sensitive = true, type = 'url' }` is a legal declaration — only
// sensitive+default is refused — and the value resolves from a vault or the
// shell. Quoting it into Reason put a live credential on stdout, in CI logs, and
// in the --output json stream, where being embedded in prose meant no consumer
// could redact it either.
func TestASensitiveValueNeverReachesAReason(t *testing.T) {
	const secret = "hooks.slack.com/services/T00000/B00000/SuperSecretToken123"
	spec := ValueSpec{Sensitive: true, Type: TypeURL}

	// The premise: this declaration is legal, so the path is reachable.
	if probs := spec.Check(SectionEnvVar); len(probs) != 0 {
		t.Fatalf("sensitive+type must stay legal or this test proves nothing: %+v", probs)
	}

	if reason := spec.CheckValue(secret); reason == "" {
		t.Error("the value is still wrong and must still be refused")
	} else if strings.Contains(reason, secret) {
		t.Errorf("LEAK: CheckValue reason %q carries the secret", reason)
	}

	out := CheckValues(
		&Schema{EnvVars: map[string]ValueSpec{"SLACK_WEBHOOK": spec}},
		Values{EnvVars: map[string]string{"SLACK_WEBHOOK": secret}},
	)
	if len(out) != 1 {
		t.Fatalf("want the finding reported, got %+v", out)
	}
	if strings.Contains(out[0].Reason, secret) {
		t.Errorf("LEAK: Violation.Reason %q carries the secret", out[0].Reason)
	}
	// Nor any fragment of it: a truncated credential is still a credential.
	if strings.Contains(out[0].Reason, "SuperSecretToken") {
		t.Errorf("LEAK: Violation.Reason %q carries part of the secret", out[0].Reason)
	}
}

// The enum message lists what IS allowed, since "expected one of" without the
// set leaves the reader to go and find the manifest.
func TestEnumReasonNamesTheAllowedValues(t *testing.T) {
	spec := ValueSpec{Type: TypeEnum, Enum: []string{"dev", "prod"}}
	reason := spec.CheckValue("staging")
	for _, want := range []string{"dev", "prod", "staging"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q omits %q", reason, want)
		}
	}

	// The allowed set comes from the manifest, so it is not secret and stays
	// even when the value must be withheld — otherwise a sensitive enum's
	// message would say nothing actionable at all.
	spec.Sensitive = true
	sreason := spec.CheckValue("staging")
	for _, want := range []string{"dev", "prod"} {
		if !strings.Contains(sreason, want) {
			t.Errorf("sensitive reason %q omits the allowed value %q", sreason, want)
		}
	}
	if strings.Contains(sreason, ", got ") {
		t.Errorf("LEAK: sensitive reason %q still appends the value clause", sreason)
	}
}

func TestCheckValues(t *testing.T) {
	schema := &Schema{
		EnvVars: map[string]ValueSpec{
			"PORT":     {Type: TypePort},
			"COUNT":    {Type: TypeInt},
			"FREEFORM": {},
		},
		AirflowVariables: map[string]ValueSpec{
			"mode": {Type: TypeEnum, Enum: []string{"a", "b"}},
		},
		Connections: map[string]ValueSpec{
			"warehouse": {Sensitive: true, ConnType: "snowflake"},
			"anykind":   {Sensitive: true},
		},
	}

	t.Run("everything conforming yields nothing", func(t *testing.T) {
		got := CheckValues(schema, Values{
			EnvVars:          map[string]string{"PORT": "8080", "COUNT": "3", "FREEFORM": "!!"},
			AirflowVariables: map[string]string{"mode": "a"},
			Connections:      map[string]string{"warehouse": "snowflake", "anykind": "sqlite"},
		})
		if len(got) != 0 {
			t.Fatalf("want no violations, got %+v", got)
		}
	})

	t.Run("every section is checked", func(t *testing.T) {
		got := CheckValues(schema, Values{
			EnvVars:          map[string]string{"PORT": "99999", "COUNT": "abc"},
			AirflowVariables: map[string]string{"mode": "c"},
			Connections:      map[string]string{"warehouse": "postgres"},
		})
		var keys []string
		for _, v := range got {
			keys = append(keys, string(v.Section)+"."+v.Key)
			if v.Kind != ViolationWrongType {
				t.Errorf("%s: Kind = %q, want %q", v.Key, v.Kind, ViolationWrongType)
			}
		}
		want := []string{
			"airflow_variable.mode",
			"connection.warehouse",
			"env_var.COUNT",
			"env_var.PORT",
		}
		if !reflect.DeepEqual(keys, want) {
			t.Errorf("violations = %v, want exactly %v (sorted by section then key)", keys, want)
		}
	})

	t.Run("a declared name with no value is not this function's business", func(t *testing.T) {
		// Absence is Validate's finding. Reporting it here too would double
		// every missing value for a caller that runs both.
		if got := CheckValues(schema, Values{}); len(got) != 0 {
			t.Fatalf("want no violations for absent values, got %+v", got)
		}
	})

	t.Run("a present but empty value is not checked", func(t *testing.T) {
		// An empty string satisfies Validate — present is present — so
		// refusing it here would make the two functions disagree about a value
		// the gate deliberately allowed.
		got := CheckValues(schema, Values{EnvVars: map[string]string{"PORT": "", "COUNT": ""}})
		if len(got) != 0 {
			t.Fatalf("want no violations for empty values, got %+v", got)
		}
	})

	t.Run("an undeclared value is ignored", func(t *testing.T) {
		got := CheckValues(schema, Values{EnvVars: map[string]string{"UNDECLARED": "not-a-port"}})
		if len(got) != 0 {
			t.Fatalf("want no violations for undeclared names, got %+v", got)
		}
	})

	t.Run("nil schema", func(t *testing.T) {
		if got := CheckValues(nil, Values{EnvVars: map[string]string{"X": "y"}}); got != nil {
			t.Fatalf("CheckValues(nil) = %+v, want nil", got)
		}
	})
}

func TestConnTypeMismatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared string
		resolved string
		flagged  bool
	}{
		{name: "matching", declared: "postgres", resolved: "postgres"},
		// DecodeConnEnv lowercases the connection id but passes conn_type
		// through verbatim, and the manifest side is verbatim TOML, so neither
		// end normalizes. A stored "Postgres" is the same working connection.
		{name: "case differences are the same kind", declared: "postgres", resolved: "Postgres"},
		{name: "case differences the other way", declared: "Snowflake", resolved: "snowflake"},
		{name: "mismatched", declared: "snowflake", resolved: "postgres", flagged: true},
		// No conn_type declared means any kind satisfies it.
		{name: "undeclared accepts anything", declared: "", resolved: "postgres"},
		// The resolver could not determine a kind. Not the same as the wrong
		// kind, and reporting it as a mismatch would blame the project for
		// something the resolver did not know.
		{name: "unknown resolved type is not a mismatch", declared: "postgres", resolved: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckValues(
				&Schema{Connections: map[string]ValueSpec{"c": {Sensitive: true, ConnType: tc.declared}}},
				Values{Connections: map[string]string{"c": tc.resolved}},
			)
			if tc.flagged && len(got) != 1 {
				t.Fatalf("want one violation, got %+v", got)
			}
			if !tc.flagged && len(got) != 0 {
				t.Fatalf("want no violation, got %+v", got)
			}
			if tc.flagged {
				for _, want := range []string{tc.declared, tc.resolved} {
					if !strings.Contains(got[0].Reason, want) {
						t.Errorf("reason %q omits %q", got[0].Reason, want)
					}
				}
			}
		})
	}
}

// The split O29 settled, pinned: the two functions answer different questions,
// and neither answers the other's.
//
// This is the test that would fail if someone "simplified" the two into one, or
// moved the type check inside Validate — which would make `astro local start`
// refuse a project over an unusual-but-working value, the failure the split
// exists to avoid.
func TestValidateAndCheckValuesAnswerDifferentQuestions(t *testing.T) {
	schema := &Schema{EnvVars: map[string]ValueSpec{
		"MISSING": {},
		"WRONG":   {Type: TypeInt},
	}}
	values := Values{EnvVars: map[string]string{"WRONG": "abc"}}

	missing := Validate(schema, values)
	if len(missing) != 1 || missing[0].Key != "MISSING" || missing[0].Kind != ViolationMissing {
		t.Errorf("Validate should report only the absent name, got %+v", missing)
	}

	wrong := CheckValues(schema, values)
	if len(wrong) != 1 || wrong[0].Key != "WRONG" || wrong[0].Kind != ViolationWrongType {
		t.Errorf("CheckValues should report only the malformed value, got %+v", wrong)
	}
}

// A value that is optional does not escape the type check when it is present.
// Optional says "you need not set this", not "anything goes if you do".
func TestOptionalValuesAreStillTypeChecked(t *testing.T) {
	schema := &Schema{EnvVars: map[string]ValueSpec{"PORT": {Type: TypePort, Optional: true}}}

	if got := Validate(schema, Values{}); len(got) != 0 {
		t.Fatalf("an absent optional value should not be a Validate violation, got %+v", got)
	}
	got := CheckValues(schema, Values{EnvVars: map[string]string{"PORT": "99999"}})
	if len(got) != 1 {
		t.Fatalf("want the optional value type-checked when present, got %+v", got)
	}
}
