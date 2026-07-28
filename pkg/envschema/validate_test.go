package envschema

import (
	"reflect"
	"testing"
)

func TestValidateNilSchema(t *testing.T) {
	if got := Validate(nil, Values{}); got != nil {
		t.Errorf("Validate(nil) = %v, want nil", got)
	}
}

func TestValidateSatisfied(t *testing.T) {
	s := &Schema{
		EnvVars: map[string]ValueSpec{
			"API_URL":   {Type: TypeURL, Required: true},
			"BATCH":     {Type: TypeInt},
			"RATE":      {Type: TypeNumber},
			"DEBUG":     {Type: TypeBool},
			"PORT":      {Type: TypePort},
			"MODE":      {Type: TypeString, Enum: []string{"dev", "prod"}},
			"CFG":       {Type: TypeJSON},
			"FREEFORM":  {},
			"MYSTERY":   {Type: ValueType("tensor")}, // unknown types are accepted
			"OPTIONAL":  {Type: TypeInt},             // absent and not required: fine
			"TEMPLATED": {Type: TypeJSON},
		},
		AirflowVariables: map[string]ValueSpec{"batch_size": {Type: TypeInt, Required: true}},
		Connections:      map[string]ConnSpec{"warehouse": {ConnType: "postgres", Required: true}},
	}
	v := Values{
		EnvVars: map[string]string{
			"API_URL":   "https://example.com/x",
			"BATCH":     "100",
			"RATE":      "0.5",
			"DEBUG":     "true",
			"PORT":      "8080",
			"MODE":      "dev",
			"CFG":       "not even json", // json is deliberately unchecked
			"FREEFORM":  "anything",
			"MYSTERY":   "???",
			"TEMPLATED": "{{ var.value.x }}",
		},
		AirflowVariables: map[string]string{"batch_size": "10"},
		Connections:      map[string]string{"warehouse": "postgres"},
	}
	if got := Validate(s, v); len(got) != 0 {
		t.Errorf("Validate = %v, want none", got)
	}
}

// TestValidateGolden covers every violation kind against the full expected
// slice, including the sorted output order.
func TestValidateGolden(t *testing.T) {
	s := &Schema{
		EnvVars: map[string]ValueSpec{
			"API_URL":  {Type: TypeURL, Required: true},           // missing
			"BATCH":    {Type: TypeInt},                           // optional but present: still checked
			"RATE":     {Type: TypeNumber},                        // wrong number
			"DEBUG":    {Type: TypeBool},                          // wrong bool
			"PORT_A":   {Type: TypePort},                          // not an int
			"PORT_B":   {Type: TypePort},                          // out of range
			"MODE":     {Enum: []string{"dev", "prod"}},           // enum on default string type
			"WORKERS":  {Type: TypeInt, Enum: []string{"1", "2"}}, // right type, outside enum
			"BAD_INT":  {Type: TypeInt, Enum: []string{"1", "2"}}, // type failure wins over enum
			"OPTIONAL": {Type: TypeInt},                           // absent, not required: no finding
		},
		AirflowVariables: map[string]ValueSpec{
			"batch_size": {Type: TypeInt, Required: true}, // missing
		},
		Connections: map[string]ConnSpec{
			"warehouse": {ConnType: "postgres", Required: true}, // missing
			"api":       {ConnType: "http"},                     // type mismatch
			"anytype":   {},                                     // no declared type: any conn_type fine
		},
	}
	v := Values{
		EnvVars: map[string]string{
			"BATCH":   "ten",
			"RATE":    "fast",
			"DEBUG":   "yep",
			"PORT_A":  "http",
			"PORT_B":  "70000",
			"MODE":    "staging",
			"WORKERS": "3",
			"BAD_INT": "x",
		},
		Connections: map[string]string{"api": "postgres", "anytype": "snowflake"},
	}
	want := []Violation{
		{Kind: ViolationMissing, Section: SectionAirflowVariable, Key: "batch_size", Reason: reasonRequired},
		{Kind: ViolationWrongType, Section: SectionConnection, Key: "api", Reason: `expected type "http", got "postgres"`},
		{Kind: ViolationMissing, Section: SectionConnection, Key: "warehouse", Reason: reasonRequired},
		{Kind: ViolationMissing, Section: SectionEnvVar, Key: "API_URL", Reason: reasonRequired},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "BAD_INT", Reason: reasonInt},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "BATCH", Reason: reasonInt},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "DEBUG", Reason: "expected a boolean"},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "MODE", Reason: "expected one of [dev prod]"},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "PORT_A", Reason: reasonPort},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "PORT_B", Reason: reasonPort},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "RATE", Reason: "expected a number"},
		{Kind: ViolationWrongType, Section: SectionEnvVar, Key: "WORKERS", Reason: "expected one of [1 2]"},
	}
	got := Validate(s, v)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Validate mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestValidateURL(t *testing.T) {
	spec := ValueSpec{Type: TypeURL}
	for _, bad := range []string{"", "example.com", "https://", "not a url"} {
		if valueError(&spec, bad) == "" {
			t.Errorf("valueError(url, %q) accepted, want rejected", bad)
		}
	}
	for _, good := range []string{"https://example.com", "postgres://db:5432/x"} {
		if reason := valueError(&spec, good); reason != "" {
			t.Errorf("valueError(url, %q) = %q, want accepted", good, reason)
		}
	}
}
