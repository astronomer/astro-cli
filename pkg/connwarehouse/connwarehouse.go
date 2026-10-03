// Package connwarehouse maps the user's local Airflow connections into the
// config the bundled "analyzing-data" skill reads, so Otto can run exploratory
// queries against the same warehouses a project's DAGs use.
//
// The skill (astronomer/agents) reads ~/.astro/agents/warehouse.yml — a map of
// warehouse name -> connector config — and resolves any value written exactly
// as "${VAR}" from its process environment. This package turns a connections.
// Connection into that pair: a warehouse.yml entry whose secret fields are
// "${VAR}" references, plus the values those references resolve to. So are
// the fields that say who connects and where (account, user, host, database,
// project, key path) wherever the skill resolves them, which leaves the YAML
// naming only object names such as a warehouse, role or schema. The
// values go only into the environment Otto is started with (AppendEnv), which
// the skill's Python kernel inherits — never into the YAML, never into a file
// on disk, and never into Otto's prompt.
//
// Only connections whose credentials are actually present locally are
// materialized; everything else is reported as a Skip with a reason so the
// caller can tell Otto which warehouses are live vs. configured-but-not-queryable.
package connwarehouse

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

// Materialized is one Airflow connection rendered as a queryable warehouse.
type Materialized struct {
	Name     string            // warehouse.yml key, e.g. "airflow_snow_prod"
	ConnID   string            // source Airflow connection id
	ConnType string            // source Airflow conn_type (normalized)
	Config   map[string]any    // the warehouse.yml entry (secrets as "${VAR}")
	Env      map[string]string // the values the "${VAR}" refs resolve to, by variable name
}

// Skip is a connection that can't (yet) be made queryable, with a reason
// suitable for surfacing to the user / Otto.
type Skip struct {
	ConnID   string
	ConnType string
	Reason   string
}

// namePrefix marks the warehouse.yml entries this package owns, so a writer can
// distinguish managed entries from anything the user hand-authored.
const namePrefix = "airflow_"

// launcherEnv is the AIRFLOW_* settings Otto and af read from their
// environment. A connection whose secret would land in one of them (a
// Databricks connection with id "api" makes AIRFLOW_API_URL) is skipped: in
// Otto's environment it would point af at the warehouse when no Airflow URL is
// set, and AppendEnv keeps such a key whenever one is.
var launcherEnv = map[string]bool{
	"AIRFLOW_API_URL":  true,
	"AIRFLOW_USERNAME": true,
	"AIRFLOW_PASSWORD": true,
}

// Materialize maps a single Airflow connection to a warehouse entry. It returns
// (entry, ok). When ok is false the Skip explains why — an unsupported
// conn_type, or required credentials that aren't present locally.
func Materialize(c connmodel.Connection) (Materialized, Skip, bool) { //nolint:gocritic // hugeParam: by value, like the rest of the connmodel API
	connType := strings.ToLower(strings.TrimSpace(c.ConnType))
	skip := func(reason string) (Materialized, Skip, bool) {
		return Materialized{}, Skip{ConnID: c.ConnID, ConnType: connType, Reason: reason}, false
	}

	var (
		cfg map[string]any
		env map[string]string
		err error
	)
	switch connType {
	case "snowflake":
		cfg, env, err = snowflake(&c)
	case "postgres", "postgresql":
		cfg, env, err = postgres(&c)
	case "gcpbigquery", "google_cloud_platform":
		cfg, env, err = bigquery(&c)
	case "databricks":
		cfg, env, err = databricks(&c)
	default:
		return skip(fmt.Sprintf("conn_type %q is not a supported warehouse", connType))
	}
	if err != nil {
		return skip(err.Error())
	}
	for k := range env {
		if launcherEnv[k] {
			return skip(fmt.Sprintf("its secret's variable %s is a setting Otto reads; rename the connection to query it", k))
		}
	}

	return Materialized{
		Name:     namePrefix + c.ConnID,
		ConnID:   c.ConnID,
		ConnType: connType,
		Config:   cfg,
		Env:      env,
	}, Skip{}, true
}

// MaterializeAll maps a set of connections, returning the queryable warehouses
// and the skipped ones (deduped by conn id, first occurrence wins — callers
// should pass connections highest-precedence-scope first).
func MaterializeAll(conns []connmodel.Connection) ([]Materialized, []Skip) {
	var out []Materialized
	var skipped []Skip
	seen := make(map[string]bool, len(conns))
	for _, c := range conns {
		if c.ConnID == "" || seen[c.ConnID] {
			continue
		}
		seen[c.ConnID] = true
		if m, s, ok := Materialize(c); ok {
			out = append(out, m)
		} else {
			skipped = append(skipped, s)
		}
	}
	return out, skipped
}

// --- per-connector mappers ---
//
// Each returns the warehouse.yml entry and the values its "${VAR}" refs name,
// or an error when a required credential/field is missing (→ the connection is
// skipped, not emitted half-built). A field is a ref when it is a secret or
// identifies the account, and the skill's from_dict passes it through
// substitute_env_vars; everything else is inline, because the skill would read
// a ref there literally.

func snowflake(c *connmodel.Connection) (cfg map[string]any, env map[string]string, err error) {
	account := firstNonEmpty(extraStr(c, "account"), extraStr(c, "extra__snowflake__account"), accountFromHost(c.ConnHost))
	if account == "" {
		return nil, nil, fmt.Errorf("snowflake %q: no account (set extra.account or a *.snowflakecomputing.com host)", c.ConnID)
	}
	// A legacy account *locator* (e.g. "XY12345") only routes once its region is
	// folded in: "XY12345.us-east-1" -> XY12345.us-east-1.snowflakecomputing.com.
	// Without the region the connector builds "<locator>.snowflakecomputing.com"
	// and the login request 404s. Mirror Airflow's SnowflakeHook: append the
	// region only for a bare legacy locator. Skip when the account already routes
	// on its own — either it's region-qualified (contains a dot) or it's an
	// org-account identifier ("orgname-accountname", contains a hyphen). A legacy
	// locator is always plain alphanumeric, so a hyphen unambiguously marks an
	// org account, which takes no region.
	if region := firstNonEmpty(extraStr(c, "region"), extraStr(c, "extra__snowflake__region")); region != "" && !strings.ContainsAny(account, ".-") {
		account += "." + region
	}
	if c.ConnLogin == "" {
		return nil, nil, fmt.Errorf("snowflake %q: no login/user", c.ConnID)
	}

	env = map[string]string{}
	cfg = map[string]any{
		"type":    "snowflake",
		"account": refTo(env, c.ConnID, "ACCOUNT", account),
		"user":    refTo(env, c.ConnID, "USER", c.ConnLogin),
	}
	putNonEmpty(cfg, "warehouse", firstNonEmpty(extraStr(c, "warehouse"), extraStr(c, "extra__snowflake__warehouse")))
	putNonEmpty(cfg, "role", firstNonEmpty(extraStr(c, "role"), extraStr(c, "extra__snowflake__role")))
	putNonEmpty(cfg, "schema", c.ConnSchema)
	if db := firstNonEmpty(extraStr(c, "database"), extraStr(c, "extra__snowflake__database")); db != "" {
		cfg["databases"] = []string{db}
	}

	// Key-pair auth takes precedence when key material is present; otherwise
	// fall back to password. Either way the secret goes to Env, not the yaml.
	if key := firstNonEmpty(extraStr(c, "private_key_content"), extraStr(c, "private_key")); key != "" {
		cfg["auth_type"] = "private_key"
		v := envVar(c.ConnID, "PRIVATE_KEY")
		cfg["private_key"] = ref(v)
		env[v] = key
		if pp := c.ConnPassword; pp != "" { // Airflow stores the key passphrase in password
			pv := envVar(c.ConnID, "PRIVATE_KEY_PASSPHRASE")
			cfg["private_key_passphrase"] = ref(pv)
			env[pv] = pp
		}
		return cfg, env, nil
	}
	if c.ConnPassword == "" {
		return nil, nil, fmt.Errorf("snowflake %q: no password or private key available locally", c.ConnID)
	}
	cfg["auth_type"] = "password"
	pv := envVar(c.ConnID, "PASSWORD")
	cfg["password"] = ref(pv)
	env[pv] = c.ConnPassword
	return cfg, env, nil
}

func postgres(c *connmodel.Connection) (cfg map[string]any, env map[string]string, err error) {
	if c.ConnHost == "" {
		return nil, nil, fmt.Errorf("postgres %q: no host", c.ConnID)
	}
	if c.ConnLogin == "" {
		return nil, nil, fmt.Errorf("postgres %q: no login/user", c.ConnID)
	}
	// Airflow's Postgres connection carries the database name in the schema field.
	database := c.ConnSchema
	if database == "" {
		return nil, nil, fmt.Errorf("postgres %q: no database (Airflow stores it in the schema field)", c.ConnID)
	}
	if c.ConnPassword == "" {
		return nil, nil, fmt.Errorf("postgres %q: no password available locally", c.ConnID)
	}

	port := c.ConnPort
	if port == 0 {
		port = 5432
	}
	// No "databases": the skill defaults it to the resolved database, and a
	// list is read literally, so writing one would put the name back inline.
	env = map[string]string{}
	cfg = map[string]any{
		"type":     "postgres",
		"host":     refTo(env, c.ConnID, "HOST", c.ConnHost),
		"port":     port,
		"user":     refTo(env, c.ConnID, "USER", c.ConnLogin),
		"database": refTo(env, c.ConnID, "DATABASE", database),
		"password": refTo(env, c.ConnID, "PASSWORD", c.ConnPassword),
	}
	putNonEmpty(cfg, "sslmode", extraStr(c, "sslmode"))
	return cfg, env, nil
}

func bigquery(c *connmodel.Connection) (cfg map[string]any, env map[string]string, err error) {
	project := firstNonEmpty(
		extraStr(c, "project"),
		extraStr(c, "project_id"),
		extraStr(c, "extra__google_cloud_platform__project"),
	)
	if project == "" {
		return nil, nil, fmt.Errorf("bigquery %q: no project in extra", c.ConnID)
	}
	keyPath := firstNonEmpty(
		extraStr(c, "key_path"),
		extraStr(c, "keyfile_path"),
		extraStr(c, "extra__google_cloud_platform__key_path"),
	)
	if keyPath == "" {
		// Inline keyfile_dict would need to be written to a 0600 file on disk;
		// not handled yet. Application-default credentials (no key) also land here.
		return nil, nil, fmt.Errorf("bigquery %q: only a credentials key-file path is supported yet (inline keyfile_dict / ADC pending)", c.ConnID)
	}
	// The key file is referenced by path and read by the kernel. No
	// "databases", for the same reason as postgres: it defaults to the
	// resolved project.
	env = map[string]string{}
	cfg = map[string]any{
		"type":             "bigquery",
		"project":          refTo(env, c.ConnID, "PROJECT", project),
		"credentials_path": refTo(env, c.ConnID, "CREDENTIALS_PATH", keyPath),
	}
	putNonEmpty(cfg, "location", firstNonEmpty(extraStr(c, "location"), extraStr(c, "extra__google_cloud_platform__location")))
	return cfg, env, nil
}

func databricks(c *connmodel.Connection) (cfg map[string]any, env map[string]string, err error) {
	host := strings.TrimPrefix(strings.TrimPrefix(c.ConnHost, "https://"), "http://")
	if host == "" {
		return nil, nil, fmt.Errorf("databricks %q: no host", c.ConnID)
	}
	httpPath := firstNonEmpty(extraStr(c, "http_path"), extraStr(c, "extra__databricks__http_path"))
	if httpPath == "" {
		return nil, nil, fmt.Errorf("databricks %q: no http_path in extra", c.ConnID)
	}
	token := firstNonEmpty(c.ConnPassword, extraStr(c, "token"))
	if token == "" {
		return nil, nil, fmt.Errorf("databricks %q: no token available locally", c.ConnID)
	}
	catalog := firstNonEmpty(extraStr(c, "catalog"), c.ConnSchema)
	if catalog == "" {
		// SQLAlchemyConnector.validate requires a non-empty databases list.
		return nil, nil, fmt.Errorf("databricks %q: no catalog (set extra.catalog or the schema field)", c.ConnID)
	}

	// The whole URL (token included) goes to Env as one "${VAR}", because the
	// skill only substitutes a value that is exactly "${VAR}" — it won't expand
	// a var embedded inside a larger string.
	q := fmt.Sprintf("http_path=%s&catalog=%s", httpPath, catalog)
	if c.ConnSchema != "" {
		q += "&schema=" + c.ConnSchema
	}
	url := fmt.Sprintf("databricks://token:%s@%s?%s", token, host, q)

	uv := envVar(c.ConnID, "URL")
	cfg = map[string]any{
		"type":      "sqlalchemy",
		"url":       ref(uv),
		"databases": []string{catalog},
	}
	return cfg, map[string]string{uv: url}, nil
}

// --- helpers ---

var envSanitize = regexp.MustCompile(`[^A-Z0-9]+`)

// envVar builds the environment variable name for a connection's field, e.g.
// envVar("snow-prod", "PASSWORD") -> "AIRFLOW_SNOW_PROD_PASSWORD".
func envVar(connID, field string) string {
	id := envSanitize.ReplaceAllString(strings.ToUpper(connID), "_")
	id = strings.Trim(id, "_")
	return "AIRFLOW_" + id + "_" + field
}

func ref(envVar string) string { return "${" + envVar + "}" }

// refTo records value in env under the connection's variable for field and
// returns the "${VAR}" ref the YAML carries in its place.
func refTo(env map[string]string, connID, field, value string) string {
	v := envVar(connID, field)
	env[v] = value
	return ref(v)
}

// extraStr reads a string-ish value from ConnExtra (values may be string, or a
// JSON number/bool decoded into float64/bool). Returns "" when absent/empty.
func extraStr(c *connmodel.Connection, key string) string {
	v, ok := c.ConnExtra[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", t), "0"), ".")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// accountFromHost extracts a Snowflake account from a "<account>.snowflake
// computing.com" host, returning "" if the host isn't in that form.
func accountFromHost(host string) string {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	if suffix := ".snowflakecomputing.com"; strings.HasSuffix(host, suffix) {
		return strings.TrimSuffix(host, suffix)
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func putNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}
