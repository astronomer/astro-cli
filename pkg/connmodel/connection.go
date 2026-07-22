// Package connmodel holds the Airflow connection value types — the pure data
// shape of a connection and its credential-free projection, with no behavior
// and no dependencies.
//
// Lifted from Astro Desktop's connmodel package, which was
// written as a pure leaf for exactly this move. It stays standard-library
// only: conversions to other shapes (the Airflow REST client's connection,
// the AIRFLOW_CONN_* env form in pkg/airflowenv) live with their consumer,
// never here, so a new importer can never drag a heavy package in through
// the value type. Desktop's store bookkeeping fields (scope, project links)
// did not travel — they belong to its store, not the value.
package connmodel

// Connection holds the full configuration for an Airflow connection.
//
// Field names mirror astro-cli's airflow_settings.yaml connection format so
// registry schemas and Airflow REST clients can consume either struct
// without rename churn. Values are held plaintext in memory; storage
// encryption is the store's job (pkg/secrets in the CLI).
type Connection struct {
	ConnID       string         `json:"conn_id"`
	ConnType     string         `json:"conn_type"`
	ConnHost     string         `json:"conn_host,omitempty"`
	ConnSchema   string         `json:"conn_schema,omitempty"`
	ConnLogin    string         `json:"conn_login,omitempty"`
	ConnPassword string         `json:"conn_password,omitempty"`
	ConnPort     int            `json:"conn_port,omitempty"`
	ConnExtra    map[string]any `json:"conn_extra,omitempty"`
}

// ConnectionMeta is the credential-free projection used everywhere
// LLM-visible or otherwise low-trust: agent-facing listings, telemetry,
// debug logs.
type ConnectionMeta struct {
	ConnID   string `json:"conn_id"`
	ConnType string `json:"conn_type"`
}
