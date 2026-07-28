package envresolve

// Provider is one source in the resolution chain. It answers a single
// question — "do you hold a value under this Airflow env-var key?" — and
// labels itself so the resolver can report where a value came from.
//
// The env-var key is the name Airflow reads: a plain var under its own NAME,
// an Airflow Variable under AIRFLOW_VAR_<KEY>, a connection under
// AIRFLOW_CONN_<ID> (pkg/airflowenv). Every provider keys on that one form,
// so the same key resolves against the shell, a project file, or a global
// file with no per-source translation.
//
// The chain that ships is shell env > project .env > global ~/.astro/env
// (internal/localenv builds it). The chain is the extension point: a
// cloud-backed or exec-hook provider slots into the same ordered walk
// without changing anything here.
type Provider interface {
	// Lookup reports the raw stored value for an Airflow env-var key, and
	// whether this provider holds one.
	Lookup(envKey string) (value string, ok bool)
	// Label names the source for reporting: "shell", "project", "global".
	Label() string
}

// lookup walks the chain and returns the first provider that holds envKey,
// with its label. ok is false when no provider holds it.
func lookup(providers []Provider, envKey string) (value, source string, ok bool) {
	for _, p := range providers {
		if v, has := p.Lookup(envKey); has {
			return v, p.Label(), true
		}
	}
	return "", "", false
}
