package envschema

import (
	"reflect"
	"testing"
)

func TestDeclarationName(t *testing.T) {
	for _, tc := range []struct {
		section Section
		key     string
		want    string
	}{
		{SectionEnvVar, "API_TOKEN", "API_TOKEN"},
		// Env var names are case-sensitive, and an AIRFLOW_VAR_-looking env var
		// is still an env var.
		{SectionEnvVar, "Mixed_Case", "Mixed_Case"},
		{SectionEnvVar, "AIRFLOW_VAR_REGION", "AIRFLOW_VAR_REGION"},
		{SectionAirflowVariable, "region", "region"},
		{SectionAirflowVariable, "REGION", "region"},
		{SectionAirflowVariable, "AIRFLOW_VAR_REGION", "region"},
		{SectionConnection, "db_main", "db_main"},
		{SectionConnection, "DB_Main", "db_main"},
		{SectionConnection, "AIRFLOW_CONN_DB_MAIN", "db_main"},
	} {
		got, err := DeclarationName(tc.section, tc.key)
		if err != nil {
			t.Errorf("DeclarationName(%s, %q): %v", tc.section, tc.key, err)
			continue
		}
		if got != tc.want {
			t.Errorf("DeclarationName(%s, %q) = %q, want %q", tc.section, tc.key, got, tc.want)
		}
	}
}

func TestDeclarationNameRefusesWhatTheParserRefuses(t *testing.T) {
	for _, tc := range []struct {
		section Section
		key     string
	}{
		{SectionEnvVar, "MY-VAR"},
		{SectionEnvVar, "1ST"},
		{SectionEnvVar, "connections"},
		{SectionEnvVar, "airflow_variables"},
		{SectionAirflowVariable, "my.var"},
		{SectionAirflowVariable, ""},
		{SectionConnection, "db-main"},
		{Section("secrets"), "X"},
	} {
		if got, err := DeclarationName(tc.section, tc.key); err == nil {
			t.Errorf("DeclarationName(%s, %q) = %q, want an error", tc.section, tc.key, got)
		}
	}
	// FoldName does not judge the name, so a bad one can still be addressed.
	if got, err := FoldName(SectionEnvVar, "MY-VAR"); err != nil || got != "MY-VAR" {
		t.Errorf("FoldName(env_var, MY-VAR) = %q, %v", got, err)
	}
}

// What DeclarationTable renders, ParseSchema reads back as the same
// declaration. The writer and the reader are two encodings of one grammar, and
// a difference between them is a declaration that changes on its way to disk.
func TestDeclarationTableRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name    string
		section Section
		spec    ValueSpec
	}{
		{"bare", SectionEnvVar, ValueSpec{}},
		{"every annotation", SectionEnvVar, ValueSpec{
			Type: TypeEnum, Enum: []string{"debug", "info"}, Default: "info", HasDefault: true,
			Optional: true, Description: "how loud",
		}},
		{"sensitive", SectionEnvVar, ValueSpec{Sensitive: true, HasSensitive: true, Description: "the token"}},
		{"workspace", SectionAirflowVariable, ValueSpec{Source: SourceWorkspace, Type: TypeInt}},
		{"empty default", SectionAirflowVariable, ValueSpec{Default: "", HasDefault: true}},
		{"connection", SectionConnection, ValueSpec{
			Sensitive: true, ConnType: "postgres", Source: SourceWorkspace, Optional: true, Description: "db",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := DeclarationTable(&tc.spec, tc.section)
			env := map[string]any{}
			switch tc.section {
			case SectionEnvVar:
				env["X"] = table
			case SectionAirflowVariable:
				env["airflow_variables"] = map[string]any{"x": table}
			case SectionConnection:
				env["connections"] = map[string]any{"x": table}
			}
			s, err := ParseSchema(env)
			if err != nil {
				t.Fatalf("ParseSchema(%v): %v", table, err)
			}
			var got ValueSpec
			switch tc.section {
			case SectionEnvVar:
				got = s.EnvVars["X"]
			case SectionAirflowVariable:
				got = s.AirflowVariables["x"]
			case SectionConnection:
				got = s.Connections["x"]
			}
			if !reflect.DeepEqual(got, tc.spec) {
				t.Errorf("round trip changed the declaration:\n got %+v\nwant %+v\ntable %v", got, tc.spec, table)
			}
		})
	}
}

// A connection is sensitive by its section, and `sensitive` there is refused
// whichever value it carries, so a caller building a connection spec from the
// zero value, or from one that says Sensitive, gets the same loadable table.
func TestDeclarationTableNeverWritesSensitiveForAConnection(t *testing.T) {
	for _, spec := range []ValueSpec{{}, {Sensitive: true, HasSensitive: true}} {
		table := DeclarationTable(&spec, SectionConnection)
		if _, ok := table["sensitive"]; ok {
			t.Errorf("DeclarationTable(%+v) wrote sensitive for a connection: %v", spec, table)
		}
		if _, err := ParseSchema(map[string]any{"connections": map[string]any{"x": table}}); err != nil {
			t.Errorf("DeclarationTable(%+v) = %v, which does not load: %v", spec, table, err)
		}
	}
}
