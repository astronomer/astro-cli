package envresolve

import (
	"errors"
	"fmt"
	"sort"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// Inputs is everything Resolve needs; the caller (the composition root)
// owns building the provider chain and choosing the environment.
type Inputs struct {
	Schema *envschema.Schema
	// Environment picks which binding each name resolves with. Empty means
	// envschema.EnvLocal — the only environment the MVP resolves;
	// anything else is ErrNotLocal.
	Environment string
	// Providers is the ordered resolution chain: the first that holds a
	// value wins. The shipped chain is shell env > project .env > global
	// ~/.astro/env (internal/localenv builds it).
	Providers []Provider
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
}

// ErrNotLocal reports a Resolve for an environment other than local. The
// MVP resolves only the local environment; deployment environments arrive
// with the deployment-backed source (stage 2).
var ErrNotLocal = errors.New("only the local environment can be resolved")

// DeploymentBindingError reports a name whose binding for the resolved
// environment names a deployment-backed source — declared in the manifest,
// not implemented yet (stage 2). The error is deliberate and loud: falling
// back to another source would silently resolve against the wrong
// environment.
type DeploymentBindingError struct {
	Section    envschema.Section
	Name       string
	Deployment string
}

func (e *DeploymentBindingError) Error() string {
	return fmt.Sprintf("%s %q is bound to deployment %q: deployment-backed values are not supported yet",
		e.Section, e.Name, e.Deployment)
}

// Resolve assembles values for every declared name from the provider chain
// (shell env > project .env > global ~/.astro/env), validates them, and
// reports what is missing and where each present value came from. It never
// writes and resolves only declared names — undeclared entries in any source
// pass through untouched, unjudged.
func Resolve(in Inputs) (*Result, error) {
	env := in.Environment
	if env == "" {
		env = envschema.EnvLocal
	}
	if env != envschema.EnvLocal {
		return nil, fmt.Errorf("%w: %q", ErrNotLocal, in.Environment)
	}
	res := &Result{}
	if in.Schema == nil {
		return res, nil
	}

	r := &resolver{in: in}
	res.Values.EnvVars = r.values(in.Schema.EnvVars, env, envschema.SectionEnvVar, func(name string) string { return name })
	res.Values.AirflowVariables = r.values(in.Schema.AirflowVariables, env, envschema.SectionAirflowVariable, airflowenv.EnvKeyForVarKey)
	res.Values.Connections = r.connTypes(in.Schema.Connections, env)

	if len(r.errs) > 0 {
		return nil, errors.Join(r.errs...)
	}

	sortResolved(r.resolved)
	res.Resolved = r.resolved
	res.Violations = append(envschema.Validate(in.Schema, res.Values), r.extraViolations...)
	sortViolations(res.Violations)
	res.Missing = missingReport(in.Schema, res.Violations)
	return res, nil
}

type resolver struct {
	in       Inputs
	resolved []ResolvedName
	errs     []error
	// extraViolations holds findings the validator can't see, e.g. a stored
	// connection whose value isn't valid connection JSON.
	extraViolations []envschema.Violation
}

// values resolves one ValueSpec section. envKey maps a declared name to its
// Airflow env-var name.
func (r *resolver) values(specs map[string]envschema.ValueSpec, env string, section envschema.Section, envKey func(string) string) map[string]string {
	if len(specs) == 0 {
		return nil
	}
	out := map[string]string{}
	for name, spec := range specs {
		if !r.checkBinding(spec.Bindings, env, section, name) {
			continue
		}
		key := envKey(name)
		v, source, ok := lookup(r.in.Providers, key)
		r.resolved = append(r.resolved, ResolvedName{Section: section, Name: name, EnvKey: key, Source: source, Found: ok})
		if ok {
			out[name] = v
		}
	}
	return out
}

// connTypes resolves the connections section down to conn id -> conn_type,
// the shape the validator inspects.
func (r *resolver) connTypes(specs map[string]envschema.ConnSpec, env string) map[string]string {
	if len(specs) == 0 {
		return nil
	}
	out := map[string]string{}
	for connID, spec := range specs {
		if !r.checkBinding(spec.Bindings, env, envschema.SectionConnection, connID) {
			continue
		}
		connKey := airflowenv.EnvKeyForConnID(connID)
		raw, source, ok := lookup(r.in.Providers, connKey)
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

// checkBinding reports whether the name resolves locally. A deployment
// binding records a typed error instead.
func (r *resolver) checkBinding(bindings map[string]envschema.Binding, env string, section envschema.Section, name string) bool {
	b := envschema.BindingFor(bindings, env)
	switch b.Source {
	case envschema.SourceVault:
		// SourceVault is the default binding: resolve from the provider chain.
		return true
	case envschema.SourceDeployment:
		r.errs = append(r.errs, &DeploymentBindingError{Section: section, Name: name, Deployment: b.Deployment})
	default:
		// ParseSchema rejects unknown sources; a hand-built schema could
		// still carry one, and skipping it silently would fake "missing".
		r.errs = append(r.errs, fmt.Errorf("%s %q: unknown binding source %q", section, name, b.Source))
	}
	return false
}

// missingReport joins the missing-value violations back with the schema.
func missingReport(s *envschema.Schema, violations []envschema.Violation) []Missing {
	var out []Missing
	for _, v := range violations {
		if v.Kind != envschema.ViolationMissing {
			continue
		}
		m := Missing{Section: v.Section, Name: v.Key}
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
