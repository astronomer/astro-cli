package envresolve

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Inputs is everything Resolve needs; the caller (the composition root)
// owns opening the store and choosing the scope.
type Inputs struct {
	Schema *envschema.Schema
	// Environment picks which binding each name resolves with. Empty means
	// envschema.EnvLocal — the only environment the MVP resolves;
	// anything else is ErrNotLocal.
	Environment string
	// Scope is the symlink-resolved absolute project path; scoped vault
	// entries beat global ones. See the package doc.
	Scope string
	// Store is the shared vault. nil when the keyring is unavailable
	// (headless Linux, secrets.ErrKeyringUnavailable): resolution then
	// uses the process env only and the result says so.
	Store secrets.Store
	// Environ is the process environment in os.Environ() form.
	Environ []string
}

// Missing is one required-but-absent value plus every way to provide it —
// typed data for cmd to render ("clone-and-run says exactly which env
// values are missing and how to provide them").
type Missing struct {
	Section     envschema.Section
	Name        string
	Description string
	// Sensitive routes the value to the vault rather than a plain file.
	// Connections are always sensitive.
	Sensitive bool
	// ConnType is the declared type, connections only.
	ConnType string
	// EnvKey is the process-env variable that satisfies this name
	// (NAME, AIRFLOW_VAR_<KEY>, or AIRFLOW_CONN_<ID>).
	EnvKey string
	// VaultKey is where a project-scoped vault write would land.
	VaultKey string
}

// Result is what local resolution produced.
type Result struct {
	// Values is the assembled input the validator judged. It holds real
	// values — never hand it to an LLM-visible surface; that is what
	// Listing is for.
	Values envschema.Values
	// Env is the assembled Airflow process environment: each declared env
	// var under its own NAME, each Airflow Variable under AIRFLOW_VAR_<KEY>,
	// each connection under AIRFLOW_CONN_<ID>. Only names that resolved to a
	// value appear; a missing required name is absent here and reported in
	// Missing instead. This is the map a plan builder layers onto the
	// runtime env — real values, so it carries the same handling rule as
	// Values.
	Env        map[string]string
	Violations []envschema.Violation
	// Missing joins the missing-value violations with the schema: what to
	// provide and where. Sorted by section then name.
	Missing []Missing
	// VaultUnavailable is set when Inputs.Store was nil — the report
	// should say values may exist that this machine cannot read.
	VaultUnavailable bool
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

// Resolve assembles values for every declared name from the process env and
// the shared vault (env > scoped vault > global vault), validates them, and
// reports what's missing. It never writes anywhere and resolves only
// declared names — undeclared process env or vault entries pass through
// untouched, unjudged.
func Resolve(in Inputs) (*Result, error) {
	env := in.Environment
	if env == "" {
		env = envschema.EnvLocal
	}
	if env != envschema.EnvLocal {
		return nil, fmt.Errorf("%w: %q", ErrNotLocal, in.Environment)
	}
	res := &Result{VaultUnavailable: in.Store == nil}
	if in.Schema == nil {
		return res, nil
	}

	environ := environMap(in.Environ)
	r := &resolver{in: in, environ: environ, env: map[string]string{}}

	res.Values.EnvVars = r.values(in.Schema.EnvVars, env, envschema.SectionEnvVar, func(name string) string { return name })
	res.Values.AirflowVariables = r.values(in.Schema.AirflowVariables, env, envschema.SectionAirflowVariable, airflowenv.EnvKeyForVarKey)
	res.Values.Connections = r.connTypes(in.Schema.Connections, env)

	if len(r.errs) > 0 {
		return nil, errors.Join(r.errs...)
	}
	if len(r.env) > 0 {
		res.Env = r.env
	}

	res.Violations = append(envschema.Validate(in.Schema, res.Values), r.extraViolations...)
	sortViolations(res.Violations)
	res.Missing = missingReport(in.Schema, in.Scope, res.Violations)
	return res, nil
}

type resolver struct {
	in      Inputs
	environ map[string]string
	// env accumulates the Airflow process environment as names resolve,
	// keyed by the env-var name Airflow reads (NAME, AIRFLOW_VAR_<KEY>,
	// AIRFLOW_CONN_<ID>). It becomes Result.Env.
	env  map[string]string
	errs []error
	// extraViolations holds findings the validator can't see, e.g. a vault
	// connection whose stored value isn't valid connection JSON.
	extraViolations []envschema.Violation
}

// values resolves one ValueSpec section. envKey maps a declared name to its
// process-env variable; the vault key always uses the same env-var name.
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
		if v, ok := r.environ[key]; ok {
			out[name] = v
			r.env[key] = v
			continue
		}
		if v, ok := r.vaultGet(EnvVaultKey(r.in.Scope, key), EnvVaultKey("", key)); ok {
			out[name] = v
			r.env[key] = v
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
		raw, ok := r.environ[connKey]
		if !ok {
			raw, ok = r.vaultGet(ConnVaultKey(r.in.Scope, connID), ConnVaultKey("", connID))
		}
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
		// The value is valid connection JSON: pass it to Airflow verbatim
		// under AIRFLOW_CONN_<ID>.
		r.env[connKey] = raw
	}
	return out
}

// checkBinding reports whether the name resolves locally. A deployment
// binding records a typed error instead.
func (r *resolver) checkBinding(bindings map[string]envschema.Binding, env string, section envschema.Section, name string) bool {
	b := envschema.BindingFor(bindings, env)
	switch b.Source {
	case envschema.SourceVault:
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

// vaultGet reads the scoped key, then the global one. Absence is not an
// error; anything else (a failing keyring mid-run) is.
func (r *resolver) vaultGet(scopedKey, globalKey string) (string, bool) {
	if r.in.Store == nil {
		return "", false
	}
	for _, key := range []string{scopedKey, globalKey} {
		v, err := r.in.Store.Get(key)
		if err == nil {
			return v, true
		}
		if !errors.Is(err, secrets.ErrNotFound) {
			r.errs = append(r.errs, fmt.Errorf("read %s: %w", key, err))
			return "", false
		}
	}
	return "", false
}

// missingReport joins the missing-value violations back with the schema.
func missingReport(s *envschema.Schema, scope string, violations []envschema.Violation) []Missing {
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
			m.VaultKey = EnvVaultKey(scope, v.Key)
		case envschema.SectionAirflowVariable:
			spec := s.AirflowVariables[v.Key]
			m.Description, m.Sensitive = spec.Description, spec.Sensitive
			m.EnvKey = airflowenv.EnvKeyForVarKey(v.Key)
			m.VaultKey = EnvVaultKey(scope, m.EnvKey)
		case envschema.SectionConnection:
			spec := s.Connections[v.Key]
			m.Description, m.ConnType = spec.Description, spec.ConnType
			m.Sensitive = true // connections always carry credentials
			m.EnvKey = airflowenv.EnvKeyForConnID(v.Key)
			m.VaultKey = ConnVaultKey(scope, v.Key)
		}
		out = append(out, m)
	}
	// Violations arrive sorted, so Missing inherits the order.
	return out
}

func sortViolations(vs []envschema.Violation) {
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].Section != vs[j].Section {
			return vs[i].Section < vs[j].Section
		}
		return vs[i].Key < vs[j].Key
	})
}

// environMap converts os.Environ() form ("KEY=value") to a map.
func environMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}
