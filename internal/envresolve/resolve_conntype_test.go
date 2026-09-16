package envresolve

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// A stored connection that parses as JSON but names no conn_type used to
// resolve to an empty kind and be accepted against any declaration. It is not a
// usable connection — Airflow resolves a provider from conn_type — so it is now
// reported the same way a corrupt value is: present, wrong type, with the value
// named as the thing to fix rather than the declaration.
//
// The distinction from TestResolveCorruptConnection is that this value is
// well-formed JSON, so the reason has to name what is missing instead of
// implying a syntax error.
func TestResolveConnectionWithNoConnType(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {},
		},
	}
	project := mapProvider{label: "project", vals: map[string]string{
		"AIRFLOW_CONN_WAREHOUSE": `{"host":"h","login":"u","password":"p"}`,
	}}
	res, err := Resolve(Inputs{Schema: schema, Providers: []Provider{project}})
	if err != nil {
		t.Fatal(err)
	}

	// Present, so not missing: the user has to fix a value, not supply one.
	if len(res.Missing) != 0 {
		t.Fatalf("unexpected missing: %+v", res.Missing)
	}

	var reason string
	for _, v := range res.Violations {
		if v.Section == envschema.SectionConnection && v.Kind == envschema.ViolationWrongType {
			reason = v.Reason
		}
	}
	if reason == "" {
		t.Fatalf("a connection with no conn_type was accepted; violations: %+v", res.Violations)
	}
	if !strings.Contains(reason, "conn_type") {
		t.Errorf("violation reason does not name conn_type, so it cannot be acted on: %q", reason)
	}
}

// A connection that does carry a conn_type still resolves to it, so the guard
// did not turn every connection into a violation.
func TestResolveConnectionWithAConnTypeStillResolves(t *testing.T) {
	schema := &envschema.Schema{
		Connections: map[string]envschema.ValueSpec{
			"warehouse": {},
		},
	}
	project := mapProvider{label: "project", vals: map[string]string{
		"AIRFLOW_CONN_WAREHOUSE": `{"conn_type":"postgres","host":"h"}`,
	}}
	res, err := Resolve(Inputs{Schema: schema, Providers: []Provider{project}})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Violations {
		if v.Section == envschema.SectionConnection {
			t.Errorf("unexpected connection violation: %+v", v)
		}
	}
	if len(res.Missing) != 0 {
		t.Fatalf("unexpected missing: %+v", res.Missing)
	}
}
