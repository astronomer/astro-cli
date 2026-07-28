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
			"API_URL":  {},
			"FREEFORM": {},
			"BLANK":    {}, // present as empty string counts as satisfied
		},
		AirflowVariables: map[string]ValueSpec{"batch_size": {}},
		Connections:      map[string]ValueSpec{"warehouse": {}},
	}
	v := Values{
		EnvVars: map[string]string{
			"API_URL":  "https://example.com/x",
			"FREEFORM": "anything",
			"BLANK":    "",
		},
		AirflowVariables: map[string]string{"batch_size": "10"},
		Connections:      map[string]string{"warehouse": "postgres"},
	}
	if got := Validate(s, v); len(got) != 0 {
		t.Errorf("Validate = %v, want none", got)
	}
}

// TestValidateGolden covers the one violation kind, missing, against the full
// expected slice, including the sorted output order across sections. Validate
// judges presence in Values, not the spec — the resolver is what turns a
// default into a present value.
func TestValidateGolden(t *testing.T) {
	s := &Schema{
		EnvVars: map[string]ValueSpec{
			"API_URL": {}, // missing
			"SET":     {}, // present
		},
		AirflowVariables: map[string]ValueSpec{
			"batch_size": {}, // missing
		},
		Connections: map[string]ValueSpec{
			"warehouse": {}, // missing
			"api":       {}, // present
		},
	}
	v := Values{
		EnvVars:     map[string]string{"SET": "x"},
		Connections: map[string]string{"api": "postgres"},
	}
	want := []Violation{
		{Kind: ViolationMissing, Section: SectionAirflowVariable, Key: "batch_size", Reason: reasonRequired},
		{Kind: ViolationMissing, Section: SectionConnection, Key: "warehouse", Reason: reasonRequired},
		{Kind: ViolationMissing, Section: SectionEnvVar, Key: "API_URL", Reason: reasonRequired},
	}
	got := Validate(s, v)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Validate mismatch\n got: %+v\nwant: %+v", got, want)
	}
}
