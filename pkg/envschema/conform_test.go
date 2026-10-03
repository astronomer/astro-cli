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
		// Wider than a 32-bit int, which the linux/386 build .goreleaser.yml
		// ships would reject under strconv.Atoi. On a 64-bit host this case
		// passes either way, so the 64-bit boundary below is the one that pins
		// the explicit width everywhere.
		{name: "int accepts a value wider than a 32-bit int", spec: ValueSpec{Type: TypeInt}, value: "3000000000", ok: true},
		{name: "int refuses a value wider than 64 bits", spec: ValueSpec{Type: TypeInt}, value: "99999999999999999999"},

		{name: "number accepts a float", spec: ValueSpec{Type: TypeNumber}, value: "3.5", ok: true},
		{name: "number accepts an integer", spec: ValueSpec{Type: TypeNumber}, value: "3", ok: true},
		{name: "number refuses a word", spec: ValueSpec{Type: TypeNumber}, value: "abc"},
		// ParseFloat accepts all of these, and a NaN reaching Airflow makes
		// every comparison against it silently false.
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
		// Scheme AND host are both required: url.Parse alone accepts a bare
		// host as a relative path and returns no error.
		{name: "url refuses a bare host", spec: ValueSpec{Type: TypeURL}, value: "example.com"},
		{name: "url refuses a path", spec: ValueSpec{Type: TypeURL}, value: "/just/a/path"},
		{name: "url refuses a scheme with no host", spec: ValueSpec{Type: TypeURL}, value: "https://"},
		// A host with no scheme, the mirror of the case above.
		{name: "url refuses a protocol-relative url", spec: ValueSpec{Type: TypeURL}, value: "//example.com/x"},

		{name: "enum accepts a member", spec: ValueSpec{Type: TypeEnum, Enum: []string{"a", "b"}}, value: "a", ok: true},
		{name: "enum refuses a non-member", spec: ValueSpec{Type: TypeEnum, Enum: []string{"a", "b"}}, value: "c"},
		{name: "enum is case secret", spec: ValueSpec{Type: TypeEnum, Enum: []string{"a"}}, value: "A"},
		// An incomplete declaration, not a set admitting nothing. Check refuses
		// the pairing so a manifest cannot reach it, but a caller holding a
		// declaration mid-edit can.
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

				// ...unless the declaration is secret. Asserted for every
				// refusing type, since every arm formats a value.
				sens := tc.spec
				sens.Secret = true
				sreason := sens.CheckValue(tc.value)
				if sreason == "" {
					t.Errorf("a secret declaration still has to refuse %q", tc.value)
				}
				// Asserted on the mechanism, not the value: a one-character
				// fixture like the enum's "c" appears inside the word
				// "expected", so a substring test would report a leak that is
				// not one. The invariant is that no `, got "…"` clause is
				// appended.
				if strings.Contains(sreason, ", got ") {
					t.Errorf("LEAK: secret reason %q still appends the value clause", sreason)
				}
			}
		})
	}
}

// A secret value's contents never reach a Violation, which is what a caller
// prints and streams as JSON.
//
// `{ secret = true, type = 'url' }` is legal — only secret+default is
// refused — and the value resolves from a vault or the shell, so a reason
// carrying it would put a credential on stdout and in CI logs, unredactable
// because it sits inside prose.
func TestASecretValueNeverReachesAReason(t *testing.T) {
	const secret = "hooks.slack.com/services/T00000/B00000/SuperSecretToken123"
	spec := ValueSpec{Secret: true, Type: TypeURL}

	// The premise: this declaration is legal, so the path is reachable.
	if probs := spec.Check(SectionEnvVar); len(probs) != 0 {
		t.Fatalf("secret+type must stay legal or this test proves nothing: %+v", probs)
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
	// even when the value is withheld.
	spec.Secret = true
	sreason := spec.CheckValue("staging")
	for _, want := range []string{"dev", "prod"} {
		if !strings.Contains(sreason, want) {
			t.Errorf("secret reason %q omits the allowed value %q", sreason, want)
		}
	}
	if strings.Contains(sreason, ", got ") {
		t.Errorf("LEAK: secret reason %q still appends the value clause", sreason)
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
			"warehouse": {Secret: true, ConnType: "snowflake"},
			"anykind":   {Secret: true},
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
		// Absence is Validate's finding; reporting it here would double every
		// missing value for a caller that runs both.
		if got := CheckValues(schema, Values{}); len(got) != 0 {
			t.Fatalf("want no violations for absent values, got %+v", got)
		}
	})

	t.Run("a present but empty value is checked against its declared type", func(t *testing.T) {
		// This used to be skipped, on the grounds that an empty string
		// satisfies Validate and reporting it here would have the two
		// functions disagree about a value the gate allows. They are meant to
		// disagree in exactly that way: Validate gates on presence and this
		// reports shape as a warning, which is the whole reason the type check
		// lives beside it. Every other type violation is already a value
		// Validate allows.
		//
		// So `type = 'port'` now means something for PORT= as well as for
		// PORT=abc, which is what an author writing the annotation asked for.
		got := CheckValues(schema, Values{EnvVars: map[string]string{"PORT": "", "COUNT": ""}})
		if len(got) != 2 {
			t.Fatalf("want a violation for each typed empty value, got %+v", got)
		}
		for _, v := range got {
			if v.Kind != ViolationWrongType || v.Section != SectionEnvVar {
				t.Errorf("unexpected violation shape: %+v", v)
			}
		}
	})

	t.Run("an empty value with no declared type is still fine", func(t *testing.T) {
		// The narrowing has to stay narrow: an empty value only reports where
		// the declaration says something a value has to be. CheckValue already
		// draws that line — an absent type, `string` and `json` are the absence
		// of a constraint — so nothing here special-cases emptiness.
		s := &Schema{EnvVars: map[string]ValueSpec{
			"PLAIN": {},
			"STR":   {Type: TypeString},
			"BLOB":  {Type: TypeJSON},
		}}
		got := CheckValues(s, Values{EnvVars: map[string]string{"PLAIN": "", "STR": "", "BLOB": ""}})
		if len(got) != 0 {
			t.Fatalf("want no violations where nothing was declared about the value, got %+v", got)
		}
	})

	t.Run("an empty connection value is still skipped", func(t *testing.T) {
		// Load-bearing, not stylistic: a connection's value here is its
		// resolved conn_type, so an empty one is a kind the resolver could not
		// determine rather than the wrong kind. Reporting it would tell the
		// user their connection is the wrong type when the truth is that its
		// value never decoded.
		s := &Schema{Connections: map[string]ValueSpec{"warehouse": {ConnType: "postgres"}}}
		got := CheckValues(s, Values{Connections: map[string]string{"warehouse": ""}})
		if len(got) != 0 {
			t.Fatalf("want no violation for an undetermined conn_type, got %+v", got)
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
		// Neither end normalizes: DecodeConnEnv passes conn_type through
		// verbatim and the manifest side is verbatim TOML.
		{name: "case differences are the same kind", declared: "postgres", resolved: "Postgres"},
		{name: "case differences the other way", declared: "Snowflake", resolved: "snowflake"},
		{name: "mismatched", declared: "snowflake", resolved: "postgres", flagged: true},
		// No conn_type declared means any kind satisfies it.
		{name: "undeclared accepts anything", declared: "", resolved: "postgres"},
		// A kind the resolver could not determine is not the wrong kind.
		{name: "unknown resolved type is not a mismatch", declared: "postgres", resolved: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckValues(
				&Schema{Connections: map[string]ValueSpec{"c": {Secret: true, ConnType: tc.declared}}},
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

// The two functions answer different questions, and neither answers the
// other's. Folding them together, or moving the type check inside Validate,
// would make `astro local start` refuse a project over a working value.
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
