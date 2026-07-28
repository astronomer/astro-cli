package localenv

import (
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
)

// Source labels — the values the `source` field carries in list/get output.
const (
	SourceShell   = "shell"
	SourceProject = "project"
	SourceGlobal  = "global"
	SourceAbsent  = "absent"
)

// mapProvider is a labeled bag of env-var keys implementing
// envresolve.Provider. The shell, project, and global sources are all one.
type mapProvider struct {
	label string
	vals  map[string]string
}

func (p mapProvider) Lookup(key string) (string, bool) { v, ok := p.vals[key]; return v, ok }
func (p mapProvider) Label() string                    { return p.label }

// Sources holds the three resolution sources for a single invocation, read
// once. It builds the ordered provider chain and the runtime injection map
// from the same parsed data, so list, the missing-value gate, and injection
// never disagree about what a file holds.
type Sources struct {
	shell      map[string]string
	project    map[string]string // nil when running outside a project
	global     map[string]string
	hasProject bool
}

// LoadSources reads the shell environment, the project's .env (when
// projectDir is non-empty), and the global ~/.astro/env. environ is the
// process environment in os.Environ() form; the caller passes it so tests can
// pin it. A missing file is empty, not an error.
func LoadSources(environ []string, projectDir string) (Sources, error) {
	s := Sources{shell: environMap(environ), global: map[string]string{}}
	if projectDir != "" {
		m, err := readMap(ProjectEnvPath(projectDir))
		if err != nil {
			return Sources{}, err
		}
		s.project = m
		s.hasProject = true
	}
	gpath, err := GlobalEnvPath()
	if err != nil {
		return Sources{}, err
	}
	gm, err := readMap(gpath)
	if err != nil {
		return Sources{}, err
	}
	s.global = gm
	return s, nil
}

// Providers is the ordered resolution chain: shell env > project .env >
// global ~/.astro/env. The project provider is omitted when there is no
// project.
func (s Sources) Providers() []envresolve.Provider {
	ps := []envresolve.Provider{mapProvider{label: SourceShell, vals: s.shell}}
	if s.hasProject {
		ps = append(ps, mapProvider{label: SourceProject, vals: s.project})
	}
	ps = append(ps, mapProvider{label: SourceGlobal, vals: s.global})
	return ps
}

// Injection is the map layered into the Airflow environment at start: the
// project .env WHOLESALE (every entry, docker-compose semantics) over the
// global file's schema-declared entries only. The shell environment is not
// here — the process already carries it. Both engines apply this identically
// through Plan.Env.
func (s Sources) Injection(schema *envschema.Schema) map[string]string {
	inj := map[string]string{}
	// Global: only the keys the schema declares. A stray global entry never
	// leaks into a project that did not ask for it.
	for _, key := range declaredEnvKeys(schema) {
		if v, ok := s.global[key]; ok {
			inj[key] = v
		}
	}
	// Project: every entry, declared or not.
	for k, v := range s.project {
		inj[k] = v
	}
	return inj
}

// declaredEnvKeys is every Airflow env-var name the schema declares.
func declaredEnvKeys(schema *envschema.Schema) []string {
	if schema == nil {
		return nil
	}
	var keys []string
	for name := range schema.EnvVars {
		keys = append(keys, name)
	}
	for key := range schema.AirflowVariables {
		keys = append(keys, airflowenv.EnvKeyForVarKey(key))
	}
	for id := range schema.Connections {
		keys = append(keys, airflowenv.EnvKeyForConnID(id))
	}
	return keys
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
