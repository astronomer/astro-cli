package envresolve

import "strings"

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
// The chain that ships is project .env > shell env > project vault > global
// vault > global ~/.astro/env (internal/localenv assembles it; the vault tiers
// come from internal/vaultenv). The chain is the extension point, and the vault
// is the proof: it slotted into the same ordered walk without changing anything
// here.
type Provider interface {
	// Lookup reports the raw stored value for an Airflow env-var key, and
	// whether this provider holds one.
	Lookup(envKey string) (value string, ok bool)
	// Label names the source for reporting: "shell", "project", "global",
	// the vault tiers, or "workspace" (plus its unavailable variants).
	Label() string
}

// Diagnoser is an optional Provider capability: when a Lookup misses, it
// explains why in a sentence, for the missing-value message a required-but-
// absent bound name produces. The Environment Manager provider implements it
// (logged out, access lost, workspace gone, org secret policy); the file
// providers do not, so callers reach it through Diagnose.
type Diagnoser interface {
	Diagnose(envKey string) string
}

// Lister is an optional Provider capability: every env key the provider
// supplies, so a caller can pass along the names no declaration mentions. The
// Environment Manager provider implements it; the workspace tier reaches a
// project whole, declared or not, the way every local tier does.
type Lister interface {
	Keys() []string
}

// Keys is every env key p supplies, or nil when p cannot list them.
func Keys(p Provider) []string {
	if l, ok := p.(Lister); ok {
		return l.Keys()
	}
	return nil
}

// Skipper is an optional Provider capability: the keys the provider holds
// that it cannot supply because they are not env-var names.
type Skipper interface {
	SkippedKeys() []string
}

// SkippedKeys is the keys p holds and cannot supply, or nil when p cannot say.
func SkippedKeys(p Provider) []string {
	if s, ok := p.(Skipper); ok {
		return s.SkippedKeys()
	}
	return nil
}

// SkippedNote is the one line naming the keys workspace holds that are not
// env-var names, or empty when there are none. Keys only, never values.
func SkippedNote(p Provider, workspace string) string {
	keys := SkippedKeys(p)
	if len(keys) == 0 {
		return ""
	}
	return "workspace " + workspace + " holds " + strings.Join(keys, ", ") +
		", which cannot be environment variable names, so Airflow does not get them. Rename them in the workspace to use them."
}

// Outager is an optional Provider capability: why the provider as a whole
// could not be read, as a short reason and a longer cause with its fix. Both
// are empty when it was read.
type Outager interface {
	Outage() (short, cause string)
}

// Outage is why p as a whole could not be read, or empty strings when it was
// or when p cannot say.
func Outage(p Provider) (short, cause string) {
	if o, ok := p.(Outager); ok {
		return o.Outage()
	}
	return "", ""
}

// Diagnose asks a provider why envKey did not resolve from it, for the message
// a missed bound name shows. A provider with nothing to say (or none) falls
// back to the generic cause. Both the resolver and `get` compose from this, so
// the wording never drifts between them.
func Diagnose(p Provider, envKey string) string {
	if d, ok := p.(Diagnoser); ok {
		if c := d.Diagnose(envKey); c != "" {
			return c
		}
	}
	return "Environment Manager holds no value for it"
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
