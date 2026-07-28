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
	// value wins. The shipped chain is shell env > project .env > global
	// ~/.astro/env (internal/localenv builds it).
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
	Section     envschema.Section
	Name        string
	Description string
	// Sensitive marks a value the schema flags sensitive. Connections are
	// always sensitive.
	Sensitive bool
	// ConnType is the declared type, connections only.
	ConnType string
	// EnvKey is the Airflow env-var name that satisfies this value
	// (NAME, AIRFLOW_VAR_<KEY>, or AIRFLOW_CONN_<ID>).
	EnvKey string
	// SourceNote explains why a workspace-source value could not be fetched —
	// logged out, offline, no workspace set, access lost, org secret policy —
	// so the missing-value message names the cause and the fix. Empty for a
	// plain local miss.
	SourceNote string `json:"source_note,omitempty"`
}

// ResolvedName is one declared name with the source it resolved from — the
// value-free record `list` reports. Source is a provider label ("shell",
// "project", "global") or "" when nothing in the chain holds it.
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
	// Injected is envKey -> value for the names that resolved from the
	// WorkspaceProvider (Environment Manager). The composition root layers
	// these into the Airflow environment on top of the file sources: unlike
	// the file values, they are not on disk for the injection path to read.
	// It deliberately overlaps Values for the env-var case (Values is
	// name-keyed for the validator; Injected is envKey-keyed for injection and
	// also carries raw connection values Values reduces to conn_type). Empty
	// when no workspace-source name resolved from the cloud. Value-carrying —
	// never hand it to an LLM-visible surface.
	Injected map[string]string
}

// Resolve assembles values for every declared name from the provider chain
// (shell env > project .env > global ~/.astro/env, then the workspace source
// for names that declare it), validates them, and reports what is missing and
// where each present value came from. It never writes and resolves only
// declared names — undeclared entries in any source pass through untouched,
// unjudged.
func Resolve(in Inputs) (*Result, error) {
	res := &Result{}
	if in.Schema == nil {
		return res, nil
	}

	r := &resolver{in: in, injected: map[string]string{}, notes: map[nameRef]string{}}
	res.Values.EnvVars = r.values(in.Schema.EnvVars, envschema.SectionEnvVar, func(name string) string { return name })
	res.Values.AirflowVariables = r.values(in.Schema.AirflowVariables, envschema.SectionAirflowVariable, airflowenv.EnvKeyForVarKey)
	res.Values.Connections = r.connTypes(in.Schema.Connections)

	if len(r.errs) > 0 {
		return nil, errors.Join(r.errs...)
	}

	sortResolved(r.resolved)
	res.Resolved = r.resolved
	res.Violations = append(envschema.Validate(in.Schema, res.Values), r.extraViolations...)
	sortViolations(res.Violations)
	res.Missing = r.missingReport(in.Schema, res.Violations)
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
	// injected is envKey -> value for names that resolved from the workspace
	// provider, the values the composition root layers into Airflow (the file
	// sources are read from disk separately).
	injected map[string]string
	// notes holds the SourceNote for a workspace-source name that could not be
	// fetched, keyed by section+name, joined into Missing.
	notes map[nameRef]string
	errs  []error
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
		v, source, ok := r.resolveOne(spec.Source, section, name, key)
		r.resolved = append(r.resolved, ResolvedName{Section: section, Name: name, EnvKey: key, Source: source, Found: ok})
		if ok {
			out[name] = v
		}
	}
	return out
}

// connTypes resolves the connections section down to conn id -> conn_type,
// the shape the validator inspects.
func (r *resolver) connTypes(specs map[string]envschema.ConnSpec) map[string]string {
	if len(specs) == 0 {
		return nil
	}
	out := map[string]string{}
	for connID, spec := range specs {
		connKey := airflowenv.EnvKeyForConnID(connID)
		raw, source, ok := r.resolveOne(spec.Source, envschema.SectionConnection, connID, connKey)
		r.resolved = append(r.resolved, ResolvedName{Section: envschema.SectionConnection, Name: connID, EnvKey: connKey, Source: source, Found: ok})
		if !ok {
			continue
		}
		conn, ok := airflowenv.DecodeConnEnv(connKey, raw)
		if !ok {
			// A corrupt value is present, not missing: record it with an
			// empty conn_type (which the validator won't re-judge) so the
			// user is told to fix the value, not to provide one.
			r.extraViolations = append(r.extraViolations, envschema.Violation{
				Kind:    envschema.ViolationWrongType,
				Section: envschema.SectionConnection,
				Key:     connID,
				Reason:  "stored value is not valid connection JSON",
			})
			out[connID] = ""
			continue
		}
		out[connID] = conn.ConnType
	}
	return out
}

// resolveOne resolves one declared name for its source. The default (empty)
// source walks the local provider chain. A workspace source checks the local
// chain first (local always wins) and falls to the workspace's Environment
// Manager scope.
func (r *resolver) resolveOne(source envschema.Source, section envschema.Section, name, key string) (value, srcLabel string, found bool) {
	switch source {
	case "":
		return lookup(r.in.Providers, key)
	case envschema.SourceWorkspace:
		return r.resolveWorkspace(section, name, key)
	default:
		// ParseSchema rejects unknown sources; a hand-built schema could
		// still carry one, and skipping it silently would fake "missing".
		r.errs = append(r.errs, fmt.Errorf("%s %q: unknown source %q", section, name, source))
		return "", "", false
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

// missingReport joins the missing-value violations back with the schema.
func (r *resolver) missingReport(s *envschema.Schema, violations []envschema.Violation) []Missing {
	var out []Missing
	for _, v := range violations {
		if v.Kind != envschema.ViolationMissing {
			continue
		}
		m := Missing{Section: v.Section, Name: v.Key, SourceNote: r.notes[nameRef{v.Section, v.Key}]}
		switch v.Section {
		case envschema.SectionEnvVar:
			spec := s.EnvVars[v.Key]
			m.Description, m.Sensitive = spec.Description, spec.Sensitive
			m.EnvKey = v.Key
		case envschema.SectionAirflowVariable:
			spec := s.AirflowVariables[v.Key]
			m.Description, m.Sensitive = spec.Description, spec.Sensitive
			m.EnvKey = airflowenv.EnvKeyForVarKey(v.Key)
		case envschema.SectionConnection:
			spec := s.Connections[v.Key]
			m.Description, m.ConnType = spec.Description, spec.ConnType
			m.Sensitive = true // connections always carry credentials
			m.EnvKey = airflowenv.EnvKeyForConnID(v.Key)
		}
		out = append(out, m)
	}
	// Violations arrive sorted, so Missing inherits the order.
	return out
}

func sortViolations(vs []envschema.Violation) {
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].Section != vs[j].Section {
			return vs[i].Section < vs[j].Section
		}
		return vs[i].Key < vs[j].Key
	})
}

func sortResolved(rs []ResolvedName) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Section != rs[j].Section {
			return rs[i].Section < rs[j].Section
		}
		return rs[i].Name < rs[j].Name
	})
}
