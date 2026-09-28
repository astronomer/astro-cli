package envresolve

import (
	"errors"
	"fmt"
	"sort"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// Inputs is everything Resolve needs; the caller (the composition root)
// owns building the provider chain.
type Inputs struct {
	Schema *envschema.Schema
	// Providers is the ordered resolution chain: the first that holds a
	// value wins. The shipped chain is shell env > project .env > project
	// vault > global vault > global ~/.astro/env (internal/localenv assembles
	// it).
	Providers []Provider
	// WorkspaceProvider resolves a name declared `source = "workspace"`
	// against the workspace's Environment Manager objects. The local files
	// still win — source is a default, not a lock — so it answers only when no
	// file did. nil leaves a workspace source unresolved (a required one gates
	// as missing): a source is never silently resolved from another place.
	WorkspaceProvider Provider
}

// Missing is one required-but-absent value plus what cmd needs to render the
// exact command that provides it — "clone-and-run says exactly which env
// values are missing and how to set them".
type Missing struct {
	Section envschema.Section `json:"section"`
	Name    string            `json:"name"`
	// EnvKey is the Airflow env-var name that satisfies this value
	// (NAME, AIRFLOW_VAR_<KEY>, or AIRFLOW_CONN_<ID>).
	EnvKey string `json:"env_key"`
	// SourceNote explains why a workspace-source value could not be fetched —
	// logged out, offline, no workspace set, access lost, org secret policy —
	// so the missing-value message names the cause and the fix. Empty for a
	// plain local miss.
	SourceNote string `json:"source_note,omitempty"`
	// Workspace marks a name declared source = "workspace" that nothing
	// local held, so a caller can tell a value the Environment Manager should
	// have supplied from a plain local miss.
	Workspace bool `json:"-"`
}

// SourceDefault is the source label a name carries when the manifest default
// is what resolved — the bottom of the chain, below every file and the cloud.
const SourceDefault = "default"

// ResolvedName is one declared name with the source it resolved from — the
// value-free record `list` reports. Source is a provider label ("shell",
// "project", "global"), "default", or "" when nothing in the chain holds it.
type ResolvedName struct {
	Section envschema.Section
	Name    string
	EnvKey  string
	Source  string
	Found   bool
}

// Result is what local resolution produced.
type Result struct {
	// Values is the assembled input the validator judged. It holds real
	// values — never hand it to an LLM-visible surface.
	Values envschema.Values
	// Resolved records the source of every declared name, value-free — the
	// backing for `list`. Sorted by section then name.
	Resolved   []ResolvedName
	Violations []envschema.Violation
	// Missing joins the missing-value violations with the schema: what to
	// provide. Sorted by section then name.
	Missing []Missing
	// Injected is envKey -> value for the names not on disk for the injection
	// path to read: a value resolved from the WorkspaceProvider (Environment
	// Manager) or a manifest default. The composition root layers these into the
	// Airflow environment on top of the file sources. It deliberately overlaps
	// Values for the env-var case (Values is name-keyed for the validator;
	// Injected is envKey-keyed for injection and also carries raw connection
	// values Values reduces to conn_type). Empty when every declared name
	// resolved from a file or from nowhere. Value-carrying — never hand it to an
	// LLM-visible surface.
	Injected map[string]string
}

// Resolve assembles values for every declared name from the provider chain
// (shell env > project .env > the two vault tiers > global ~/.astro/env, then the workspace source
// for names that declare it), validates them, and reports what is missing and
// where each present value came from. It never writes and resolves only
// declared names — undeclared entries in any source pass through untouched,
// unjudged.
func Resolve(in Inputs) (*Result, error) {
	res := &Result{}
	if in.Schema == nil {
		return res, nil
	}

	r := &resolver{in: in, injected: map[string]string{}, notes: map[nameRef]string{}, workspace: map[nameRef]bool{}}
	res.Values.EnvVars = r.values(in.Schema.EnvVars, envschema.SectionEnvVar, func(name string) string { return name })
	res.Values.AirflowVariables = r.values(in.Schema.AirflowVariables, envschema.SectionAirflowVariable, airflowenv.EnvKeyForVarKey)
	res.Values.Connections = r.connTypes(in.Schema.Connections)

	if len(r.errs) > 0 {
		return nil, errors.Join(r.errs...)
	}

	sortResolved(r.resolved)
	res.Resolved = r.resolved
	// Three sources, three questions. Validate: is anything missing.
	// CheckValues: is what resolved the shape it was declared to be.
	// extraViolations: what only this resolver can find, a connection whose
	// JSON will not decode.
	//
	// Concatenated rather than folded together because callers gate on
	// different subsets — `astro local start` refuses on missing and reports
	// the rest — which is why Missing is derived separately below.
	res.Violations = append(envschema.Validate(in.Schema, res.Values), envschema.CheckValues(in.Schema, res.Values)...)
	res.Violations = append(res.Violations, r.extraViolations...)
	envschema.SortViolations(res.Violations)
	res.Missing = r.missingReport(res.Violations)
	res.Injected = r.injected
	return res, nil
}

// nameRef keys a declared name by its section, so an env var and a connection
// of the same name never collide.
type nameRef struct {
	Section envschema.Section
	Name    string
}

type resolver struct {
	in       Inputs
	resolved []ResolvedName
	// injected is envKey -> value for names resolved from the workspace provider
	// or a manifest default — the values not on disk that the composition root
	// layers into Airflow (the file sources are read from disk separately).
	injected map[string]string
	// notes holds the SourceNote for a workspace-source name that could not be
	// fetched, keyed by section+name, joined into Missing.
	notes map[nameRef]string
	// workspace holds every workspace-source name that nothing local held.
	workspace map[nameRef]bool
	errs      []error
	// extraViolations holds findings the validator can't see, e.g. a stored
	// connection whose value isn't valid connection JSON.
	extraViolations []envschema.Violation
}

// values resolves one ValueSpec section. envKey maps a declared name to its
// Airflow env-var name.
func (r *resolver) values(specs map[string]envschema.ValueSpec, section envschema.Section, envKey func(string) string) map[string]string {
	if len(specs) == 0 {
		return nil
	}
	out := map[string]string{}
	for name, spec := range specs {
		key := envKey(name)
		v, source, ok := r.resolveOne(spec, section, name, key)
		r.resolved = append(r.resolved, ResolvedName{Section: section, Name: name, EnvKey: key, Source: source, Found: ok})
		if ok {
			out[name] = v
		}
	}
	return out
}

// connTypes resolves the connections section down to conn id -> a present
// marker, the shape the validator inspects for presence. A resolved value that
// is not a usable connection is a wrong-type violation, not a missing value.
func (r *resolver) connTypes(specs map[string]envschema.ValueSpec) map[string]string {
	if len(specs) == 0 {
		return nil
	}
	out := map[string]string{}
	for connID, spec := range specs {
		connKey := airflowenv.EnvKeyForConnID(connID)
		raw, source, ok := r.resolveOne(spec, envschema.SectionConnection, connID, connKey)
		r.resolved = append(r.resolved, ResolvedName{Section: envschema.SectionConnection, Name: connID, EnvKey: connKey, Source: source, Found: ok})
		if !ok {
			continue
		}
		conn, ok := airflowenv.DecodeConnEnv(connKey, raw)
		if !ok {
			// A corrupt value is present, not missing: record it so the user is
			// told to fix the value, not to provide one. It stays present in the
			// values map so the validator does not also flag it missing.
			r.extraViolations = append(r.extraViolations, envschema.Violation{
				Kind:    envschema.ViolationWrongType,
				Section: envschema.SectionConnection,
				Key:     connID,
				Reason:  "stored value is not a usable connection: expected JSON carrying a conn_type",
			})
			out[connID] = ""
			continue
		}
		out[connID] = conn.ConnType
	}
	return out
}

// resolveOne resolves one declared name. The default (empty) source walks the
// local provider chain and falls back to the manifest default when set; the
// default sits at the very bottom, below every file. A workspace source checks
// the local chain first (local always wins) and falls to the workspace's
// Environment Manager scope.
func (r *resolver) resolveOne(spec envschema.ValueSpec, section envschema.Section, name, key string) (value, srcLabel string, found bool) {
	switch spec.Source {
	case "":
		if v, src, ok := lookup(r.in.Providers, key); ok {
			return v, src, true
		}
		if spec.HasDefault {
			// The default is not on disk; layer it into Airflow the same way a
			// workspace value is layered, so start injects it too.
			r.injected[key] = spec.Default
			return spec.Default, SourceDefault, true
		}
		// Nothing held it, and a provider may know why. Without this the run
		// reports a bare "not set on this machine" for a value that IS set and
		// merely unreadable — a vaulted secret behind a keyring that will not
		// open, which is the one cause the user can act on.
		r.noteChainMiss(section, name, key)
		return "", "", false
	case envschema.SourceWorkspace:
		return r.resolveWorkspace(section, name, key)
	default:
		// ParseSchema rejects unknown sources; a hand-built schema could
		// still carry one, and skipping it silently would fake "missing".
		r.errs = append(r.errs, fmt.Errorf("%s %q: unknown source %q", section, name, spec.Source))
		return "", "", false
	}
}

// noteChainMiss records the first provider explanation for a name nothing in the
// chain held. A provider with nothing to say returns an empty cause and is
// skipped, so an ordinary never-set value gets no note.
func (r *resolver) noteChainMiss(section envschema.Section, name, key string) {
	for _, p := range r.in.Providers {
		d, ok := p.(Diagnoser)
		if !ok {
			continue
		}
		if cause := d.Diagnose(key); cause != "" {
			r.notes[nameRef{section, name}] = cause
			return
		}
	}
}

// resolveWorkspace resolves a name declared source = "workspace". The local
// files win (source is a default, not a lock), so a set value keeps the network
// off this path; only when nothing local answers does the workspace's
// Environment Manager scope answer.
func (r *resolver) resolveWorkspace(section envschema.Section, name, key string) (value, srcLabel string, found bool) {
	if v, src, ok := lookup(r.in.Providers, key); ok {
		return v, src, true
	}
	r.workspace[nameRef{section, name}] = true
	wp := r.in.WorkspaceProvider
	if wp == nil {
		// No workspace resolution wired for this run. Report the name
		// unresolved with a plain note rather than resolving it elsewhere; a
		// required one then gates as missing.
		r.notes[nameRef{section, name}] = "declared source = \"workspace\", but workspace resolution is not available here"
		return "", string(envschema.SourceWorkspace), false
	}
	if v, ok := wp.Lookup(key); ok {
		r.injected[key] = v
		return v, wp.Label(), true
	}
	// Nothing resolved it. The provider's label carries any "unavailable"
	// reason for `list`; record the longer cause for the missing-value message.
	r.notes[nameRef{section, name}] = "source \"workspace\": " + Diagnose(wp, key)
	return "", wp.Label(), false
}

// missingReport turns the missing-value violations into the report cmd renders,
// each with its Airflow env-var key and any workspace-source note.
func (r *resolver) missingReport(violations []envschema.Violation) []Missing {
	var out []Missing
	for _, v := range violations {
		if v.Kind != envschema.ViolationMissing {
			continue
		}
		ref := nameRef{v.Section, v.Key}
		m := Missing{Section: v.Section, Name: v.Key, SourceNote: r.notes[ref], Workspace: r.workspace[ref]}
		switch v.Section {
		case envschema.SectionEnvVar:
			m.EnvKey = v.Key
		case envschema.SectionAirflowVariable:
			m.EnvKey = airflowenv.EnvKeyForVarKey(v.Key)
		case envschema.SectionConnection:
			m.EnvKey = airflowenv.EnvKeyForConnID(v.Key)
		}
		out = append(out, m)
	}
	// Violations arrive sorted, so Missing inherits the order.
	return out
}

func sortResolved(rs []ResolvedName) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Section != rs[j].Section {
			return rs[i].Section < rs[j].Section
		}
		return rs[i].Name < rs[j].Name
	})
}
