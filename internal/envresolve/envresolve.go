// Package envresolve composes the env-schema pieces for the CLI: it types
// the manifest's [tool.astro.env] section (envschema), resolves values for
// the local environment from an ordered chain of providers, validates the
// result, and reports what's missing and where each present value came from —
// all as typed data. cmd renders; nothing here prints.
//
// # Resolution order
//
// Resolution is an ordered chain (see Provider), lowest tiers last:
//
//	shell env > project .env > global ~/.astro/env > workspace EM > manifest default
//
// An exported variable is the most deliberate, most local statement, so it
// wins; the project file beats the global one. A name declared source =
// "workspace" also reads the workspace's Environment Manager below the files,
// and a name with a manifest default falls back to it at the very bottom.
// internal/localenv builds the file chain and owns the files; this package is
// the pure resolution and validation over whatever providers it is handed, so
// a later provider (another cloud-backed source, an exec hook) slots into the
// same walk without changing anything here.
//
// # Env-var keys
//
// Every declared name maps to the one env-var name Airflow reads, and
// providers key on that form (pkg/airflowenv):
//
//	a plain var        under its own NAME
//	an Airflow Variable under AIRFLOW_VAR_<KEY>
//	a connection       under AIRFLOW_CONN_<ID> (the airflowenv JSON form)
package envresolve
