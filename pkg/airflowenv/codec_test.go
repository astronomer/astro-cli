package airflowenv

import (
	"reflect"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

func TestConnRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   connmodel.Connection
		want connmodel.Connection // after round-trip (conn id lowercased)
	}{
		{
			name: "full",
			in: connmodel.Connection{
				ConnID: "warehouse", ConnType: "postgres", ConnHost: "db.example.com",
				ConnSchema: "public", ConnLogin: "svc", ConnPassword: "p@ss w0rd", ConnPort: 5432,
				ConnExtra: map[string]any{"sslmode": "require", "region": "us-east-1"},
			},
			want: connmodel.Connection{
				ConnID: "warehouse", ConnType: "postgres", ConnHost: "db.example.com",
				ConnSchema: "public", ConnLogin: "svc", ConnPassword: "p@ss w0rd", ConnPort: 5432,
				ConnExtra: map[string]any{"sslmode": "require", "region": "us-east-1"},
			},
		},
		{
			name: "minimal",
			in:   connmodel.Connection{ConnID: "http_default", ConnType: "http"},
			want: connmodel.Connection{ConnID: "http_default", ConnType: "http"},
		},
		{
			name: "mixed_case_id_canonicalizes_lower",
			in:   connmodel.Connection{ConnID: "My_Conn", ConnType: "http"},
			want: connmodel.Connection{ConnID: "my_conn", ConnType: "http"},
		},
		{
			name: "password_with_quotes_and_backslashes",
			in:   connmodel.Connection{ConnID: "tricky", ConnType: "generic", ConnPassword: `a"b\c\nd`},
			want: connmodel.Connection{ConnID: "tricky", ConnType: "generic", ConnPassword: `a"b\c\nd`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, val, ok := EncodeConnEnv(tc.in)
			if !ok {
				t.Fatalf("EncodeConnEnv returned ok=false for %+v", tc.in)
			}
			if !strings.HasPrefix(key, ConnPrefix) {
				t.Errorf("key %q missing prefix", key)
			}
			if strings.Contains(val, "\n") {
				t.Errorf("encoded value must be a single line, got %q", val)
			}
			got, ok := DecodeConnEnv(key, val)
			if !ok {
				t.Fatalf("DecodeConnEnv returned ok=false for key=%q val=%q", key, val)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("round-trip mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}

func TestEncodeConnEnv_OmitsEmptyFields(t *testing.T) {
	_, val, ok := EncodeConnEnv(connmodel.Connection{ConnID: "c", ConnType: "http"})
	if !ok {
		t.Fatal("encode failed")
	}
	for _, k := range []string{"host", "login", "password", "schema", "port", "extra"} {
		if strings.Contains(val, `"`+k+`"`) {
			t.Errorf("empty field %q should be omitted, got %q", k, val)
		}
	}
	if !strings.Contains(val, `"conn_type":"http"`) {
		t.Errorf("conn_type must always be present, got %q", val)
	}
}

func TestValidConnID(t *testing.T) {
	// A leading digit is fine: the AIRFLOW_CONN_ prefix supplies the required
	// leading letter, so the full env-var name stays legal.
	valid := []string{"a", "my_conn", "Conn1", "_x", "ABC_123", "1conn"}
	invalid := []string{"", "my.conn", "my-conn", "has space", "a/b"}
	for _, id := range valid {
		if !ValidConnID(id) {
			t.Errorf("ValidConnID(%q) = false, want true", id)
		}
		if _, _, ok := EncodeConnEnv(connmodel.Connection{ConnID: id, ConnType: "http"}); !ok {
			t.Errorf("EncodeConnEnv rejected valid id %q", id)
		}
	}
	for _, id := range invalid {
		if ValidConnID(id) {
			t.Errorf("ValidConnID(%q) = true, want false", id)
		}
		if _, _, ok := EncodeConnEnv(connmodel.Connection{ConnID: id, ConnType: "http"}); ok {
			t.Errorf("EncodeConnEnv accepted invalid id %q", id)
		}
	}
}

func TestVarRoundTrip(t *testing.T) {
	cases := []struct {
		in       string
		value    string
		wantKey  string
		wantBack string
	}{
		{"batch_size", "100", "AIRFLOW_VAR_BATCH_SIZE", "batch_size"},
		{"Region", "us-east-1", "AIRFLOW_VAR_REGION", "region"},
		{"cfg", `{"a":1}`, "AIRFLOW_VAR_CFG", "cfg"},
	}
	for _, tc := range cases {
		key, val, ok := EncodeVarEnv(tc.in, tc.value)
		if !ok {
			t.Fatalf("EncodeVarEnv(%q) ok=false", tc.in)
		}
		if key != tc.wantKey {
			t.Errorf("key = %q, want %q", key, tc.wantKey)
		}
		if val != tc.value {
			t.Errorf("value = %q, want %q", val, tc.value)
		}
		gotKey, gotVal, ok := DecodeVarEnv(key, val)
		if !ok || gotKey != tc.wantBack || gotVal != tc.value {
			t.Errorf("DecodeVarEnv = (%q,%q,%v), want (%q,%q,true)", gotKey, gotVal, ok, tc.wantBack, tc.value)
		}
	}
	// A leading digit is refused although AIRFLOW_VAR_1X is a legal name: the
	// key is held to the env-var rule on its own, which Astro Desktop's .env
	// store applies too.
	if ValidVarKey("a.b") || ValidVarKey("") || ValidVarKey("1x") || ValidVarKey("9") {
		t.Error("invalid var keys accepted")
	}
}

func TestDetectionAndRejection(t *testing.T) {
	if IsConnEnvKey("FOO") || IsConnEnvKey(ConnPrefix) {
		t.Error("IsConnEnvKey should reject plain key and bare prefix")
	}
	if !IsConnEnvKey("AIRFLOW_CONN_X") {
		t.Error("IsConnEnvKey should accept AIRFLOW_CONN_X")
	}
	if IsVarEnvKey("FOO") || IsVarEnvKey(VarPrefix) {
		t.Error("IsVarEnvKey should reject plain key and bare prefix")
	}
	if _, ok := DecodeConnEnv("FOO", "{}"); ok {
		t.Error("DecodeConnEnv should reject non-connection key")
	}
	if _, ok := DecodeConnEnv("AIRFLOW_CONN_X", "not json"); ok {
		t.Error("DecodeConnEnv should reject invalid JSON")
	}
	if _, _, ok := DecodeVarEnv("FOO", "v"); ok {
		t.Error("DecodeVarEnv should reject non-variable key")
	}
}

// ConnIDForEnvKey reads the id from the key alone, and the value-independence is
// the property its callers rely on: classifying a key you hold no value for, or
// naming a connection whose value will not decode, must not depend on what the
// value decoder happens to accept.
func TestConnIDForEnvKeyReadsTheKeyAlone(t *testing.T) {
	cases := map[string]string{
		"AIRFLOW_CONN_WAREHOUSE": "warehouse",
		"AIRFLOW_CONN_MY_CONN":   "my_conn",
		"AIRFLOW_CONN_MixedCase": "mixedcase",
		"AIRFLOW_CONN_1CONN":     "1conn",
	}
	for key, want := range cases {
		if got := ConnIDForEnvKey(key); got != want {
			t.Errorf("ConnIDForEnvKey(%q) = %q, want %q", key, got, want)
		}
	}

	// It reports the same id DecodeConnEnv does, so the two cannot drift.
	for key, want := range cases {
		c, ok := DecodeConnEnv(key, `{"conn_type":"postgres"}`)
		if !ok {
			t.Fatalf("DecodeConnEnv(%q) returned ok=false", key)
		}
		if c.ConnID != want {
			t.Errorf("DecodeConnEnv(%q) id = %q, want %q", key, c.ConnID, want)
		}
	}

	// And it still answers for values that carry nothing usable, which is
	// precisely where a caller cannot route through DecodeConnEnv.
	for _, value := range []string{"", "{}", "not json", `{"host":"h"}`} {
		if _, ok := DecodeConnEnv("AIRFLOW_CONN_WAREHOUSE", value); ok && value == "not json" {
			t.Errorf("DecodeConnEnv accepted %q, so this case no longer proves anything", value)
		}
		if got := ConnIDForEnvKey("AIRFLOW_CONN_WAREHOUSE"); got != "warehouse" {
			t.Errorf("ConnIDForEnvKey did not answer alongside value %q: got %q", value, got)
		}
	}
}

// Environment Manager takes either form as an object key, so both must land on
// the one env key a local declaration looks up.
func TestStoredKeysMapToOneEnvKey(t *testing.T) {
	vars := map[string]string{
		"region":             "AIRFLOW_VAR_REGION",
		"AIRFLOW_VAR_REGION": "AIRFLOW_VAR_REGION",
		"api_token":          "AIRFLOW_VAR_API_TOKEN",
	}
	for objectKey, want := range vars {
		if got := EnvKeyForStoredVarKey(objectKey); got != want {
			t.Errorf("EnvKeyForStoredVarKey(%q) = %q, want %q", objectKey, got, want)
		}
	}

	conns := map[string]string{
		"db_main":              "db_main",
		"AIRFLOW_CONN_DB_MAIN": "db_main",
	}
	for objectKey, want := range conns {
		if got := ConnIDForStoredConnKey(objectKey); got != want {
			t.Errorf("ConnIDForStoredConnKey(%q) = %q, want %q", objectKey, got, want)
		}
	}
}
