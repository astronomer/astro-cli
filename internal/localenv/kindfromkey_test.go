package localenv

import "testing"

// kindFromKey classifies an undeclared file entry from its env-var name, and
// that classification is what `astro local env list` labels each row with. It
// reads the key only, so no case here supplies a value: the point is that the
// answer cannot depend on one.
func TestKindFromKey(t *testing.T) {
	cases := []struct {
		key      string
		wantKind Kind
		wantName string
	}{
		{"AIRFLOW_CONN_WAREHOUSE", KindConn, "warehouse"},
		{"AIRFLOW_CONN_MY_CONN", KindConn, "my_conn"},
		{"AIRFLOW_CONN_MixedCase", KindConn, "mixedcase"},
		{"AIRFLOW_VAR_REGION", KindVar, "region"},
		{"AIRFLOW_VAR_MY_VAR", KindVar, "my_var"},
		{"DATABASE_URL", KindEnv, "DATABASE_URL"},
		// A bare prefix names no object, so it is an ordinary env var whose
		// name happens to look like one.
		{"AIRFLOW_CONN_", KindEnv, "AIRFLOW_CONN_"},
		{"AIRFLOW_VAR_", KindEnv, "AIRFLOW_VAR_"},
		{"", KindEnv, ""},
	}
	for _, tc := range cases {
		kind, name := kindFromKey(tc.key)
		if kind != tc.wantKind || name != tc.wantName {
			t.Errorf("kindFromKey(%q) = (%q, %q), want (%q, %q)",
				tc.key, kind, name, tc.wantKind, tc.wantName)
		}
	}
}
