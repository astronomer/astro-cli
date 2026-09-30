// Package airflowenv encodes and decodes Airflow connections and variables as
// environment variables — the form Airflow resolves natively. A connection is
// AIRFLOW_CONN_<ID> with a single-line JSON value; a variable is
// AIRFLOW_VAR_<KEY> with its raw value. This is the delivery mechanism for
// local Airflow in the MVP: a connection or variable "is just an env var", so
// the runtime reads it with no extra wiring.
//
// Lifted from Astro Desktop's airflowenv package.
//
// There are two pairs for connections, and they obey different rules because
// they address different things.
//
// EncodeConnEnv / DecodeConnEnv are the ENV-VAR form. The id is uppercased in
// the name (Airflow's convention) and treated case-insensitively, so decode
// lowercases it back; and because the name must be a legal env-var identifier,
// an id that cannot be one is rejected by the encoder (see ValidConnID).
//
// EncodeConnValue / DecodeConnValue are the VALUE only, for a store that keys a
// connection itself. Neither rule applies: no id is rejected, and the id the
// caller supplies is returned verbatim rather than lowercased, because it came
// from that store's own key and is not this package's to reinterpret. A caller
// holding one id and reading through both doors therefore gets the id it gave
// from one and a lowercased id from the other — deliberate, and pinned by test.
package airflowenv

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	value, err := EncodeConnValue(c)
	if err != nil {
		return "", "", false
	}
	return EnvKeyForConnID(c.ConnID), value, true
}

// EncodeConnValue renders just the value half: the single-line JSON, with no
// env-var name and no requirement that the id could be one.
//
// It is separate from EncodeConnEnv because a vault stores a connection under a
// key of its own (conn:<scope>:<id>) and needs this exact JSON as the value —
// that encoding is the contract, since a reader builds an AIRFLOW_CONN_*
// straight from what it holds. Going through EncodeConnEnv would impose the
// env-var name rule on storage, so a connection whose id is legal in Airflow
// and in that store but not as an env-var identifier ("my-db") could not be
// saved at all. The two concerns stay separate, with one definition of the shape.
// The JSON intentionally carries the password: that is exactly how Airflow
// parses an env-var connection, and a form that omitted it would not be one.
func EncodeConnValue(c connmodel.Connection) (string, error) { //nolint:gocritic // hugeParam: matches EncodeConnEnv's value-type API
	data, err := json.Marshal(connJSON{
		ConnType: c.ConnType,
		Host:     c.ConnHost,
		Login:    c.ConnLogin,
		Password: c.ConnPassword,
		Schema:   c.ConnSchema,
		Port:     c.ConnPort,
		Extra:    c.ConnExtra,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsConnEnvKey reports whether key is an AIRFLOW_CONN_* env-var key (with a
// non-empty id suffix).
func IsConnEnvKey(key string) bool {
	return strings.HasPrefix(key, ConnPrefix) && len(key) > len(ConnPrefix)
}

// ConnIDForEnvKey is the connection id an AIRFLOW_CONN_* key addresses, read
// from the key alone. The id is the lowercased suffix, because Airflow treats
// env connection ids case-insensitively. Meaningless for a key IsConnEnvKey
// rejects.
//
// Exported because a caller has to be able to name WHICH connection an entry is
// without reading its value — to tell an unusable entry from an absent one, or
// to classify a key it holds no value for. The alternative is routing through
// DecodeConnEnv with a placeholder value, which couples key classification to
// whatever the value half happens to accept.
func ConnIDForEnvKey(key string) string {
	return strings.ToLower(strings.TrimPrefix(key, ConnPrefix))
}

// DecodeConnEnv parses an AIRFLOW_CONN_* env-var pair back into a Connection.
// The conn id is the lowercased suffix (Airflow treats env connection ids
// case-insensitively). ok is false for a non-connection key, for JSON that does
// not parse, and for JSON carrying no conn_type.
//
// The conn_type requirement is what makes a decoded value a connection rather
// than a bag of fields: Airflow resolves a provider from conn_type, so without
// one there is nothing to connect with. Returning such a value anyway only
// moves the failure somewhere that cannot explain it — into the provider import,
// or into a satisfied-looking declaration. A caller that needs to name the
// connection without judging its value wants ConnIDForEnvKey.
func DecodeConnEnv(key, value string) (connmodel.Connection, bool) {
	if !IsConnEnvKey(key) {
		return connmodel.Connection{}, false
	}
	c, err := DecodeConnValue(ConnIDForEnvKey(key), value)
	if err != nil {
		return connmodel.Connection{}, false
	}
	return c, true
}

// DecodeConnValue parses just the value half, for a caller that already knows
// the connection id — a vault reading back what EncodeConnValue stored under its
// own key. It is the one place the conn_type requirement is enforced.
//
// Not quite the mirror of EncodeConnValue: that encoder accepts a connection
// with no conn_type and this refuses the result, so a caller that writes
// without checking can store a row nothing can read. The env pair guards its
// own write boundary in NormalizeConn; a store using this pair has to do the
// same. Pinned by TestTheValuePairIsNotSymmetric.
//
// It returns an error rather than a bool because its callers can say which
// stored record is unusable and why, where DecodeConnEnv's callers are walking
// an environment and only need to know whether an entry is a connection. Every
// error names the record for that reason.
func DecodeConnValue(connID, value string) (connmodel.Connection, error) {
	if connID == "" {
		return connmodel.Connection{}, fmt.Errorf("connection id is required")
	}
	var p connJSON
	if err := decodeExact([]byte(value), &p); err != nil {
		// Deliberately not %w. The value is decrypted connection JSON, and a
		// json.SyntaxError quotes the byte it stopped on — which for a
		// corrupted row can be a byte of the password. The caller gets the
		// record and the kind of fault, never a piece of the payload.
		return connmodel.Connection{}, fmt.Errorf("connection %q: value is not valid JSON", connID)
	}
	if p.ConnType == "" {
		return connmodel.Connection{}, fmt.Errorf("connection %q has no conn_type", connID)
	}
	return connmodel.Connection{
		ConnID:       connID,
		ConnType:     p.ConnType,
		ConnHost:     p.Host,
		ConnLogin:    p.Login,
		ConnPassword: p.Password,
		ConnSchema:   p.Schema,
		ConnPort:     p.Port,
		ConnExtra:    p.Extra,
	}, nil
}

// decodeExact unmarshals with UseNumber, so a JSON number survives being
// decoded and re-encoded.
//
// Without it every number in `extra` becomes a float64, and re-marshaling one
// larger than 2^53 writes a different number: a Snowflake account id of
// 1234567890123456789 came back as 1234567890123456800, eleven off, with
// nothing reporting it. Connection extras carry exactly that kind of value —
// account, project and warehouse ids — and a connection is decoded and
// re-encoded on every read-modify-write of the store.
//
// json.Number marshals as the digits it was parsed from, so the round trip is
// lossless.
func decodeExact(data []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(into)
}

// DecodeExtra parses a connection's `extra` as given on a command line: a JSON
// object, with its numbers preserved exactly.
//
// It lives here so the two trees that accept --extra share one parser and one
// message. The distinction the message draws matters: `[1,2]` is valid JSON
// and telling the user it is not sends them looking for a syntax error that is
// not there.
func DecodeExtra(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var extra map[string]any
	if err := decodeExact([]byte(trimmed), &extra); err != nil {
		if !strings.HasPrefix(trimmed, "{") {
			return nil, fmt.Errorf("extra must be a JSON object, like {\"sslmode\":\"require\"}")
		}
		return nil, fmt.Errorf("extra is not valid JSON: %w", err)
	}
	return extra, nil
}

// EnvKeyForVarKey returns the AIRFLOW_VAR_* env-var key for a variable key.
func EnvKeyForVarKey(varKey string) string {
	return VarPrefix + strings.ToUpper(varKey)
}

// ValidVarKey reports whether a variable key can be stored and resolved as an
// Airflow Variable: the key must itself be a legal env-var name (ValidEnvKey),
// leading digit refused, and so AIRFLOW_VAR_<KEY> is one too.
//
// The key alone is held to the env-var rule, not only the prefixed form, so
// that every tool storing a Variable agrees: Astro Desktop keeps a project's
// values in a .env keyed by the POSIX rule (envfile.ValidKey), and a key only
// the CLI accepted would be one the desktop cannot hold or show.
func ValidVarKey(varKey string) bool {
	return ValidEnvKey(varKey)
}

// VarKeyRule is ValidVarKey's rule as a person reads it, for a refusal.
const VarKeyRule = "letters, digits, _; no leading digit"

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

// EnvKeyForStoredVarKey is the AIRFLOW_VAR_* key for an Airflow variable as
// Environment Manager keys it. The platform accepts either form as an object
// key — the variable's own key ("region") or one already in env form
// ("AIRFLOW_VAR_REGION") — so both readers of it, the CLI and Astro Desktop,
// map through this one function and cannot disagree on which it was.
func EnvKeyForStoredVarKey(objectKey string) string {
	if IsVarEnvKey(objectKey) {
		return objectKey
	}
	return EnvKeyForVarKey(objectKey)
}

// ConnIDForStoredConnKey is the connection id for a CONNECTION object as
// Environment Manager keys it, for the same reason as EnvKeyForStoredVarKey:
// the key is the id ("db_main") or already in env form ("AIRFLOW_CONN_DB_MAIN").
func ConnIDForStoredConnKey(objectKey string) string {
	if IsConnEnvKey(objectKey) {
		return ConnIDForEnvKey(objectKey)
	}
	return objectKey
}

// DecodeVarEnv parses an AIRFLOW_VAR_* pair into (lowercased key, value).
func DecodeVarEnv(key, value string) (varKey, val string, ok bool) {
	if !IsVarEnvKey(key) {
		return "", "", false
	}
	return strings.ToLower(strings.TrimPrefix(key, VarPrefix)), value, true
}
