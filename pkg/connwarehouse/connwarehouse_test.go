package connwarehouse

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

func TestMaterializeSnowflakePassword(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "snow_prod",
		ConnType:     "snowflake",
		ConnLogin:    "svc",
		ConnPassword: "s3cret",
		ConnSchema:   "PUBLIC",
		ConnExtra:    map[string]any{"account": "acme-east", "warehouse": "WH", "role": "ANALYST", "database": "ANALYTICS"},
	})
	if !ok {
		t.Fatal("expected snowflake to materialize")
	}
	if m.Name != "airflow_snow_prod" {
		t.Errorf("name = %q", m.Name)
	}
	if m.Config["account"] != "${AIRFLOW_SNOW_PROD_ACCOUNT}" || m.Config["user"] != "${AIRFLOW_SNOW_PROD_USER}" || m.Config["auth_type"] != "password" {
		t.Errorf("config = %+v", m.Config)
	}
	if m.Env["AIRFLOW_SNOW_PROD_ACCOUNT"] != "acme-east" || m.Env["AIRFLOW_SNOW_PROD_USER"] != "svc" {
		t.Errorf("env keys = %d", len(m.Env))
	}
	if m.Config["password"] != "${AIRFLOW_SNOW_PROD_PASSWORD}" {
		t.Errorf("password ref = %v", m.Config["password"])
	}
	if m.Env["AIRFLOW_SNOW_PROD_PASSWORD"] != "s3cret" {
		t.Errorf("env = %+v", m.Env)
	}
	if dbs, _ := m.Config["databases"].([]string); len(dbs) != 1 || dbs[0] != "ANALYTICS" {
		t.Errorf("databases = %v", m.Config["databases"])
	}
	assertNoPlaintextSecret(t, m, "s3cret")
	assertNoPlaintextSecret(t, m, "acme-east")
	assertNoPlaintextSecret(t, m, "svc")
}

func TestMaterializeSnowflakeAccountFromHost(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "s",
		ConnType:     "snowflake",
		ConnLogin:    "u",
		ConnPassword: "p",
		ConnHost:     "xy12345.us-east-1.snowflakecomputing.com",
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got := resolved(t, m, "account"); got != "xy12345.us-east-1" {
		t.Errorf("account from host = %v", got)
	}
}

func TestMaterializeSnowflakeFoldsRegionIntoLocator(t *testing.T) {
	// A cloud Environment Manager connection whose account is a bare legacy
	// locator plus a separate region extra — the account must become
	// "<locator>.<region>" or the login request 404s.
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "s",
		ConnType:     "snowflake",
		ConnLogin:    "u",
		ConnPassword: "p",
		ConnExtra:    map[string]any{"account": "XY12345", "region": "us-east-1"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got := resolved(t, m, "account"); got != "XY12345.us-east-1" {
		t.Errorf("account = %v, want XY12345.us-east-1", got)
	}
}

func TestMaterializeSnowflakeRegionSkippedWhenQualified(t *testing.T) {
	// Account already carries its region (a dot) — don't double-append.
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "s",
		ConnType:     "snowflake",
		ConnLogin:    "u",
		ConnPassword: "p",
		ConnExtra:    map[string]any{"account": "XY12345.us-east-1", "region": "us-east-1"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got := resolved(t, m, "account"); got != "XY12345.us-east-1" {
		t.Errorf("account = %v, want XY12345.us-east-1 (region not re-appended)", got)
	}
}

func TestMaterializeSnowflakeRegionSkippedForOrgAccount(t *testing.T) {
	// An org-account identifier ("orgname-accountname") already routes on its
	// own; a stray region extra must not be folded in, or we'd corrupt the
	// account into "orgname-accountname.us-east-1".
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "s",
		ConnType:     "snowflake",
		ConnLogin:    "u",
		ConnPassword: "p",
		ConnExtra:    map[string]any{"account": "acme-analytics", "region": "us-east-1"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got := resolved(t, m, "account"); got != "acme-analytics" {
		t.Errorf("account = %v, want acme-analytics (region not folded into an org account)", got)
	}
}

func TestMaterializeSnowflakePrefixedExtraKeys(t *testing.T) {
	// Some connection sources emit Airflow's extra__snowflake__* prefixed keys;
	// account/region/warehouse/role must resolve from those too.
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "s",
		ConnType:     "snowflake",
		ConnLogin:    "u",
		ConnPassword: "p",
		ConnExtra: map[string]any{
			"extra__snowflake__account":   "XY12345",
			"extra__snowflake__region":    "us-east-1",
			"extra__snowflake__warehouse": "WH",
			"extra__snowflake__role":      "ANALYST",
		},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got := resolved(t, m, "account"); got != "XY12345.us-east-1" {
		t.Errorf("account = %v", got)
	}
	if m.Config["warehouse"] != "WH" || m.Config["role"] != "ANALYST" {
		t.Errorf("config = %+v", m.Config)
	}
}

func TestMaterializeSnowflakeKeyPair(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "kp",
		ConnType:     "snowflake",
		ConnLogin:    "u",
		ConnPassword: "passphrase",
		ConnExtra:    map[string]any{"account": "a", "private_key_content": "-----BEGIN KEY-----"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if m.Config["auth_type"] != "private_key" {
		t.Errorf("auth_type = %v", m.Config["auth_type"])
	}
	if m.Config["private_key"] != "${AIRFLOW_KP_PRIVATE_KEY}" {
		t.Errorf("private_key ref = %v", m.Config["private_key"])
	}
	// Password field carries the key passphrase for key-pair auth.
	if m.Config["private_key_passphrase"] != "${AIRFLOW_KP_PRIVATE_KEY_PASSPHRASE}" {
		t.Errorf("passphrase ref = %v", m.Config["private_key_passphrase"])
	}
	if m.Env["AIRFLOW_KP_PRIVATE_KEY"] == "" || m.Env["AIRFLOW_KP_PRIVATE_KEY_PASSPHRASE"] != "passphrase" {
		t.Errorf("env = %+v", m.Env)
	}
	assertNoPlaintextSecret(t, m, "-----BEGIN KEY-----")
}

func TestMaterializeSnowflakeMissingCreds(t *testing.T) {
	_, s, ok := Materialize(connmodel.Connection{
		ConnID:    "s",
		ConnType:  "snowflake",
		ConnLogin: "u",
		ConnExtra: map[string]any{"account": "a"},
	})
	if ok {
		t.Fatal("expected skip without password/key")
	}
	if !strings.Contains(s.Reason, "password or private key") {
		t.Errorf("reason = %q", s.Reason)
	}
}

func TestMaterializePostgres(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "pg",
		ConnType:     "postgresql",
		ConnHost:     "db.example.com",
		ConnPort:     6543,
		ConnLogin:    "rw",
		ConnPassword: "pw",
		ConnSchema:   "analytics",
		ConnExtra:    map[string]any{"sslmode": "require"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if m.Config["type"] != "postgres" || m.Config["port"] != 6543 || m.Config["sslmode"] != "require" {
		t.Errorf("config = %+v", m.Config)
	}
	if resolved(t, m, "host") != "db.example.com" || resolved(t, m, "user") != "rw" || resolved(t, m, "database") != "analytics" {
		t.Errorf("identifying fields do not resolve to the connection's values")
	}
	// The skill reads a databases list literally and defaults it to the
	// resolved database, so the entry must not carry one.
	if _, ok := m.Config["databases"]; ok {
		t.Errorf("databases = %v, want it left to the skill's default", m.Config["databases"])
	}
	if m.Config["password"] != "${AIRFLOW_PG_PASSWORD}" || m.Env["AIRFLOW_PG_PASSWORD"] != "pw" {
		t.Errorf("secret handling = %+v / %+v", m.Config["password"], m.Env)
	}
	assertNoPlaintextSecret(t, m, "pw")
	assertNoPlaintextSecret(t, m, "db.example.com")
	assertNoPlaintextSecret(t, m, "analytics")
}

func TestMaterializePostgresDefaultsPort(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "pg",
		ConnType:     "postgres",
		ConnHost:     "h",
		ConnLogin:    "u",
		ConnPassword: "p",
		ConnSchema:   "db",
	})
	if !ok || m.Config["port"] != 5432 {
		t.Errorf("expected default port 5432, got %v (ok=%v)", m.Config["port"], ok)
	}
}

func TestMaterializePostgresMissingDatabase(t *testing.T) {
	_, s, ok := Materialize(connmodel.Connection{
		ConnID:       "pg",
		ConnType:     "postgres",
		ConnHost:     "h",
		ConnLogin:    "u",
		ConnPassword: "p",
	})
	if ok {
		t.Fatal("expected skip without database")
	}
	if !strings.Contains(s.Reason, "database") {
		t.Errorf("reason = %q", s.Reason)
	}
}

func TestMaterializeBigQueryKeyPath(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:    "bq",
		ConnType:  "gcpbigquery",
		ConnExtra: map[string]any{"project": "my-proj", "key_path": "/secrets/key.json", "location": "US"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if m.Config["type"] != "bigquery" || m.Config["location"] != "US" {
		t.Errorf("config = %+v", m.Config)
	}
	if resolved(t, m, "project") != "my-proj" || resolved(t, m, "credentials_path") != "/secrets/key.json" {
		t.Errorf("project/credentials_path do not resolve to the connection's values")
	}
	if _, ok := m.Config["databases"]; ok {
		t.Errorf("databases = %v, want it left to the skill's default", m.Config["databases"])
	}
	assertNoPlaintextSecret(t, m, "my-proj")
	assertNoPlaintextSecret(t, m, "/secrets/key.json")
}

func TestMaterializeBigQueryInlineSkipped(t *testing.T) {
	_, s, ok := Materialize(connmodel.Connection{
		ConnID:    "bq",
		ConnType:  "google_cloud_platform",
		ConnExtra: map[string]any{"project": "p", "keyfile_dict": `{"type":"service_account"}`},
	})
	if ok {
		t.Fatal("expected inline keyfile_dict to be skipped for now")
	}
	if !strings.Contains(s.Reason, "key-file path") {
		t.Errorf("reason = %q", s.Reason)
	}
}

func TestMaterializeDatabricks(t *testing.T) {
	m, _, ok := Materialize(connmodel.Connection{
		ConnID:       "dbx",
		ConnType:     "databricks",
		ConnHost:     "https://dbc-abc.cloud.databricks.com",
		ConnPassword: "dapi-token",
		ConnSchema:   "default",
		ConnExtra:    map[string]any{"http_path": "/sql/1.0/warehouses/xyz", "catalog": "main"},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if m.Config["type"] != "sqlalchemy" || m.Config["url"] != "${AIRFLOW_DBX_URL}" {
		t.Errorf("config = %+v", m.Config)
	}
	if dbs, _ := m.Config["databases"].([]string); len(dbs) != 1 || dbs[0] != "main" {
		t.Errorf("databases = %v", m.Config["databases"])
	}
	url := m.Env["AIRFLOW_DBX_URL"]
	for _, want := range []string{"databricks://token:dapi-token@dbc-abc.cloud.databricks.com", "http_path=/sql/1.0/warehouses/xyz", "catalog=main", "schema=default"} {
		if !strings.Contains(url, want) {
			t.Errorf("url %q missing %q", url, want)
		}
	}
	// Token is in Env only; the yaml carries just the ${VAR} ref.
	assertNoPlaintextSecret(t, m, "dapi-token")
}

// A connection whose secret variable would be a setting Otto reads (id "api"
// makes AIRFLOW_API_URL) is skipped rather than allowed to set it.
func TestMaterializeSkipsALauncherSetting(t *testing.T) {
	_, s, ok := Materialize(connmodel.Connection{
		ConnID:       "api",
		ConnType:     "databricks",
		ConnHost:     "https://dbc-abc.cloud.databricks.com",
		ConnPassword: "dapi-token",
		ConnSchema:   "default",
		ConnExtra:    map[string]any{"http_path": "/sql/1.0/warehouses/xyz", "catalog": "main"},
	})
	if ok {
		t.Fatal("a connection whose secret is AIRFLOW_API_URL was materialized")
	}
	if strings.Contains(s.Reason, "dapi-token") {
		t.Fatal("the skip reason holds the token")
	}
	if !strings.Contains(s.Reason, "AIRFLOW_API_URL") {
		t.Errorf("reason = %q, want it to name AIRFLOW_API_URL", s.Reason)
	}
}

func TestMaterializeDatabricksMissingPieces(t *testing.T) {
	for _, tc := range []struct {
		name string
		conn connmodel.Connection
		want string
	}{
		{"no http_path", connmodel.Connection{
			ConnID:       "d",
			ConnType:     "databricks",
			ConnHost:     "h",
			ConnPassword: "t",
			ConnExtra:    map[string]any{"catalog": "c"},
		}, "http_path"},
		{"no token", connmodel.Connection{
			ConnID:    "d",
			ConnType:  "databricks",
			ConnHost:  "h",
			ConnExtra: map[string]any{"http_path": "/p", "catalog": "c"},
		}, "token"},
		{"no catalog", connmodel.Connection{
			ConnID:       "d",
			ConnType:     "databricks",
			ConnHost:     "h",
			ConnPassword: "t",
			ConnExtra:    map[string]any{"http_path": "/p"},
		}, "catalog"},
	} {
		if _, s, ok := Materialize(tc.conn); ok || !strings.Contains(s.Reason, tc.want) {
			t.Errorf("%s: ok=%v reason=%q want reason containing %q", tc.name, ok, s.Reason, tc.want)
		}
	}
}

func TestMaterializeUnsupportedType(t *testing.T) {
	_, s, ok := Materialize(connmodel.Connection{
		ConnID:       "x",
		ConnType:     "mysql",
		ConnLogin:    "u",
		ConnPassword: "p",
	})
	if ok {
		t.Fatal("expected skip for unsupported type")
	}
	if !strings.Contains(s.Reason, "not a supported warehouse") {
		t.Errorf("reason = %q", s.Reason)
	}
}

func TestMaterializeAllDedupsAndSplits(t *testing.T) {
	conns := []connmodel.Connection{
		{ConnID: "pg", ConnType: "postgres", ConnHost: "h", ConnLogin: "u", ConnPassword: "worktree-pw", ConnSchema: "db"},
		{ConnID: "pg", ConnType: "postgres", ConnHost: "h", ConnLogin: "u", ConnPassword: "global-pw", ConnSchema: "db"}, // dup, ignored
		{ConnID: "nocreds", ConnType: "snowflake", ConnLogin: "u", ConnExtra: map[string]any{"account": "a"}},            // skip
	}
	live, skipped := MaterializeAll(conns)
	if len(live) != 1 {
		t.Fatalf("expected 1 live warehouse, got %d", len(live))
	}
	// First occurrence wins (highest-precedence scope passed first).
	if live[0].Env["AIRFLOW_PG_PASSWORD"] != "worktree-pw" {
		t.Errorf("expected first occurrence to win, got %+v", live[0].Env)
	}
	if len(skipped) != 1 {
		t.Errorf("expected 1 skip, got %d (%+v)", len(skipped), skipped)
	}
}

func TestEnvVarSanitization(t *testing.T) {
	for in, want := range map[string]string{
		"snow_prod": "AIRFLOW_SNOW_PROD_PASSWORD",
		"snow-prod": "AIRFLOW_SNOW_PROD_PASSWORD",
		"My.Conn 1": "AIRFLOW_MY_CONN_1_PASSWORD",
		"_leading_": "AIRFLOW_LEADING_PASSWORD",
	} {
		if got := envVar(in, "PASSWORD"); got != want {
			t.Errorf("envVar(%q) = %q, want %q", in, got, want)
		}
	}
}

// assertNoPlaintextSecret guards the core invariant: a secret value must never
// appear in the warehouse.yml entry — only in the .env map.
// assertNoPlaintextSecret fails when value appears anywhere in the YAML entry,
// lists included. Named for secrets, and used for the identifying fields too.
func assertNoPlaintextSecret(t *testing.T, m Materialized, value string) {
	t.Helper()
	for k, v := range m.Config {
		switch x := v.(type) {
		case string:
			if strings.Contains(x, value) {
				t.Errorf("value leaked into config[%q] (%d bytes)", k, len(x))
			}
		case []string:
			for _, s := range x {
				if strings.Contains(s, value) {
					t.Errorf("value leaked into config[%q] list", k)
				}
			}
		}
	}
}

// resolved asserts config[key] is a "${VAR}" ref and returns what Env gives
// that variable, which is what the skill resolves the field to.
func resolved(t *testing.T, m Materialized, key string) string {
	t.Helper()
	v, _ := m.Config[key].(string)
	if !strings.HasPrefix(v, "${") || !strings.HasSuffix(v, "}") {
		t.Fatalf("config[%q] = %q, want a ${VAR} ref", key, v)
	}
	val, ok := m.Env[v[2:len(v)-1]]
	if !ok {
		t.Fatalf("config[%q] refs %s, which Env does not set", key, v)
	}
	return val
}
