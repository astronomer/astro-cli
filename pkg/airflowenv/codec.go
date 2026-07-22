// Package airflowenv encodes and decodes Airflow connections and variables as
// environment variables — the form Airflow resolves natively. A connection is
// AIRFLOW_CONN_<ID> with a single-line JSON value; a variable is
// AIRFLOW_VAR_<KEY> with its raw value. This is the delivery mechanism for
// local Airflow in the MVP: a connection or variable "is just an env var", so
// the runtime reads it with no extra wiring.
//
// Lifted from Astro Desktop's airflowenv package. The conn id /
// var key in the env-var name is uppercased (Airflow's convention) and
// treated case-insensitively: decode lowercases it back. Because the name
// must be a legal env-var identifier, ids/keys are restricted to [A-Za-z0-9_]
// (see ValidConnID / ValidVarKey) — connections or variables whose id can't
// be expressed as an env var are rejected by the encoder.
package airflowenv

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/connmodel"
)

const (
	// ConnPrefix is Airflow's environment-variable prefix for connections.
	ConnPrefix = "AIRFLOW_CONN_"
	// VarPrefix is Airflow's environment-variable prefix for variables.
	VarPrefix = "AIRFLOW_VAR_"
)

// envKeyPattern matches a valid POSIX environment-variable name: a leading
// letter or underscore followed by letters, digits, or underscores.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidEnvKey reports whether key is a legal environment-variable name.
// (Desktop's envfile.ValidKey — the single validator every env-var writer
// shares, so no path ever emits a malformed name. The .env machinery
// itself stayed behind.)
func ValidEnvKey(key string) bool {
	return envKeyPattern.MatchString(key)
}

// connJSON is the JSON shape Airflow parses from an AIRFLOW_CONN_* value. It
// is deliberately distinct from the Airflow REST API v2 shape (which uses
// connection_id and a string-encoded extra): the env-var form keys
// host/login/password/schema/port/extra under conn_type, with extra as a
// nested object.
type connJSON struct {
	ConnType string         `json:"conn_type"`
	Host     string         `json:"host,omitempty"`
	Login    string         `json:"login,omitempty"`
	Password string         `json:"password,omitempty"`
	Schema   string         `json:"schema,omitempty"`
	Port     int            `json:"port,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// EnvKeyForConnID returns the AIRFLOW_CONN_* env-var key for a connection id.
func EnvKeyForConnID(connID string) string {
	return ConnPrefix + strings.ToUpper(connID)
}

// ValidConnID reports whether a connection id can be represented as an
// AIRFLOW_CONN_* env var (i.e. AIRFLOW_CONN_<ID> is a legal env-var name).
func ValidConnID(connID string) bool {
	return connID != "" && ValidEnvKey(EnvKeyForConnID(connID))
}

// EncodeConnEnv renders a connection as an (AIRFLOW_CONN_<ID>, single-line
// JSON) env-var pair. The value is always a single physical line
// (json.Marshal never emits newlines; any newline inside a field is escaped
// within the JSON string), satisfying Airflow's "single unbroken line"
// requirement. ok is false when the id can't be an env var.
func EncodeConnEnv(c connmodel.Connection) (key, value string, ok bool) { //nolint:gocritic // hugeParam: the value type passes by value, matching desktop's codec API
	if !ValidConnID(c.ConnID) {
		return "", "", false
	}
	payload := connJSON{
		ConnType: c.ConnType,
		Host:     c.ConnHost,
		Login:    c.ConnLogin,
		Password: c.ConnPassword,
		Schema:   c.ConnSchema,
		Port:     c.ConnPort,
		Extra:    c.ConnExtra,
	}
	data, err := json.Marshal(payload) //nolint:gosec // G117: the AIRFLOW_CONN_ JSON form intentionally serializes the password — that is exactly how Airflow parses an env-var connection.
	if err != nil {
		return "", "", false
	}
	return EnvKeyForConnID(c.ConnID), string(data), true
}

// IsConnEnvKey reports whether key is an AIRFLOW_CONN_* env-var key (with a
// non-empty id suffix).
func IsConnEnvKey(key string) bool {
	return strings.HasPrefix(key, ConnPrefix) && len(key) > len(ConnPrefix)
}

// DecodeConnEnv parses an AIRFLOW_CONN_* env-var pair back into a Connection.
// The conn id is the lowercased suffix (Airflow treats env connection ids
// case-insensitively). ok is false for non-connection keys or invalid JSON.
func DecodeConnEnv(key, value string) (connmodel.Connection, bool) {
	if !IsConnEnvKey(key) {
		return connmodel.Connection{}, false
	}
	var p connJSON
	if err := json.Unmarshal([]byte(value), &p); err != nil {
		return connmodel.Connection{}, false
	}
	return connmodel.Connection{
		ConnID:       strings.ToLower(strings.TrimPrefix(key, ConnPrefix)),
		ConnType:     p.ConnType,
		ConnHost:     p.Host,
		ConnLogin:    p.Login,
		ConnPassword: p.Password,
		ConnSchema:   p.Schema,
		ConnPort:     p.Port,
		ConnExtra:    p.Extra,
	}, true
}

// EnvKeyForVarKey returns the AIRFLOW_VAR_* env-var key for a variable key.
func EnvKeyForVarKey(varKey string) string {
	return VarPrefix + strings.ToUpper(varKey)
}

// ValidVarKey reports whether a variable key can be represented as an
// AIRFLOW_VAR_* env var.
func ValidVarKey(varKey string) bool {
	return varKey != "" && ValidEnvKey(EnvKeyForVarKey(varKey))
}

// EncodeVarEnv renders an Airflow variable as an (AIRFLOW_VAR_<KEY>, value)
// pair. The value is stored raw — Airflow variable values are opaque
// strings. ok is false when the key can't be an env var.
func EncodeVarEnv(varKey, value string) (key, val string, ok bool) {
	if !ValidVarKey(varKey) {
		return "", "", false
	}
	return EnvKeyForVarKey(varKey), value, true
}

// IsVarEnvKey reports whether key is an AIRFLOW_VAR_* env-var key (with a
// non-empty key suffix).
func IsVarEnvKey(key string) bool {
	return strings.HasPrefix(key, VarPrefix) && len(key) > len(VarPrefix)
}

// DecodeVarEnv parses an AIRFLOW_VAR_* pair into (lowercased key, value).
func DecodeVarEnv(key, value string) (varKey, val string, ok bool) {
	if !IsVarEnvKey(key) {
		return "", "", false
	}
	return strings.ToLower(strings.TrimPrefix(key, VarPrefix)), value, true
}
