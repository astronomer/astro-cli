// Package envresolve composes the env-schema pieces for the CLI: it types
// the manifest's [tool.astro.env] section (envschema), resolves values for
// the local environment from the shared vault (pkg/secrets) and the process
// env, validates the result, and reports what's missing as typed data. cmd
// renders; nothing here prints.
//
// # Vault key namespace
//
// The CLI and Astro Desktop share one vault (secrets.DefaultService,
// ~/.astro/secrets). Within it, environment values use two key shapes:
//
//	env:<scope>:<NAME>      a plain env var. Airflow Variables are stored
//	                        under their AIRFLOW_VAR_<KEY> env name — a
//	                        variable "is just an env var" (pkg/airflowenv).
//	conn:<scope>:<conn_id>  a connection; the value is exactly what
//	                        AIRFLOW_CONN_<ID> would hold (the airflowenv
//	                        JSON form). Conn ids are stored lowercased,
//	                        matching Airflow's case-insensitive treatment.
//
// <scope> is "" for a global entry or the symlink-resolved absolute project
// path for a per-checkout entry — desktop's worktree-path pattern, chosen
// there for CLI compatibility. Scoped entries beat global ones. NAME and
// conn_id can never contain a colon (they must be legal env-var names), so
// a key parses from both ends even when the scope path contains colons.
//
// # Resolution order
//
// Process env beats the vault, and the project scope beats the global tier:
// env > scoped vault > global vault. An exported variable is the most
// deliberate, most local statement — and it is the only source a headless
// machine without a keyring has.
package envresolve

import (
	"strings"
)

// Vault key kinds. See the package doc for the full namespace.
const (
	VaultKindEnv  = "env"
	VaultKindConn = "conn"
)

// EnvVaultKey returns the vault key for a plain env var (or an Airflow
// Variable under its AIRFLOW_VAR_* name) in the given scope ("" = global).
func EnvVaultKey(scope, name string) string {
	return VaultKindEnv + ":" + scope + ":" + name
}

// ConnVaultKey returns the vault key for a connection in the given scope
// ("" = global). The conn id is lowercased: Airflow treats connection ids
// case-insensitively, so one canonical spelling keeps a mixed-case declare
// and a lowercase set from producing two entries.
func ConnVaultKey(scope, connID string) string {
	return VaultKindConn + ":" + scope + ":" + strings.ToLower(connID)
}

// ParseVaultKey splits a vault key into kind, scope, and name. The kind
// ends at the first colon and the name starts after the last one; the scope
// is everything between, so a scope path containing colons still parses.
// ok is false for keys outside this namespace.
func ParseVaultKey(key string) (kind, scope, name string, ok bool) {
	kind, rest, found := strings.Cut(key, ":")
	if !found || (kind != VaultKindEnv && kind != VaultKindConn) {
		return "", "", "", false
	}
	i := strings.LastIndex(rest, ":")
	if i < 0 || rest[i+1:] == "" {
		return "", "", "", false
	}
	return kind, rest[:i], rest[i+1:], true
}
