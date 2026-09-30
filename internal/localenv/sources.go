package localenv

import (
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
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

// Providers is the ordered resolution chain:
//
//	shell env > project .env > project vault > global vault > global ~/.astro/env
//
// The project provider is omitted when there is no project.
//
// vault is the encrypted tier's providers, in their own order
// (internal/vaultenv builds them). They are passed in rather than built here
// because this package writes plain files and touches no keyring, and because
// the caller is the composition root — the same reason the Environment Manager
// provider arrives as a seam. Pass nil for a chain without the vault.
//
// The slot is the decision. A vault value sits BELOW the project's .env, so a
// hand-written plaintext entry still wins, exactly as it does on the desktop;
// and ABOVE the global file, so this machine's shared secret beats a global
// plaintext default. Shell env stays on top, which is where a CLI user expects
// `FOO=bar astro local start` to land.
func (s Sources) Providers(vault []envresolve.Provider) []envresolve.Provider {
	ps := []envresolve.Provider{mapProvider{label: SourceShell, vals: s.shell}}
	if s.hasProject {
		ps = append(ps, mapProvider{label: SourceProject, vals: s.project})
	}
	ps = append(ps, vault...)
	return append(ps, mapProvider{label: SourceGlobal, vals: s.global})
}

// AboveVault reports whether a source that OUTRANKS the vault tiers holds key —
// the shell environment, or the project's .env.
//
// It exists for the injection merge. The vault's project tier injects wholesale,
// like the project .env, so it carries names the schema does not declare and the
// resolver therefore never saw; those names still have to obey the chain, and
// this is the half of the chain that beats them.
func (s Sources) AboveVault(key string) bool {
	if _, ok := s.shell[key]; ok {
		return true
	}
	if s.hasProject {
		if _, ok := s.project[key]; ok {
			return true
		}
	}
	return false
}

// InShell reports whether the shell environment sets key.
func (s Sources) InShell(key string) bool {
	_, ok := s.shell[key]
	return ok
}

// ShellOverGlobalFile is the ~/.astro/env keys Injection leaves out because the
// shell sets them too, sorted. The caller passes them through to docker, which
// inherits no host shell.
func (s Sources) ShellOverGlobalFile() []string {
	var out []string
	for key := range s.global {
		if s.InShell(key) {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// Injection is the map layered into the Airflow environment at start: the
// project .env WHOLESALE (every entry, docker-compose semantics) over the
// global file's entries, also wholesale. ~/.astro/env reaches every project,
// so all of it reaches this one, declared or not: a declaration makes a name a
// requirement, it does not decide whether a value is passed. The shell
// environment is not here, since the process already carries it, and a global
// entry the shell also sets is left out: the shell outranks the global file,
// and a Plan.Env entry would override it. Both engines apply this identically
// through Plan.Env.
func (s Sources) Injection() map[string]string {
	inj := map[string]string{}
	for key, v := range s.global {
		if s.InShell(key) {
			continue
		}
		inj[key] = v
	}
	// Project: every entry, declared or not.
	for k, v := range s.project {
		inj[k] = v
	}
	return inj
}

// Undeclared is the env-var names a start of this project passes to Airflow
// from local sources the schema does not declare: the project .env, the
// global file, and the vault tiers' entries that reach this checkout. They
// work locally and nowhere else, since a Deployment and a teammate's clone get
// only what the project declares, so check and package name them. Airflow
// settings (AIRFLOW__*) are left out: they configure Airflow rather than name a
// value the project's code expects, the same reason list does not call them
// orphans. Names only, sorted; nothing here is a value.
func (s Sources) Undeclared(schema *envschema.Schema, tiers []VaultTier) []string {
	declared := map[string]bool{}
	for _, k := range envschema.DeclaredEnvKeys(schema) {
		declared[k] = true
	}
	seen := map[string]bool{}
	add := func(key string) {
		if !declared[key] && !isAirflowSetting(key) {
			seen[key] = true
		}
	}
	for key := range s.project {
		add(key)
	}
	for key := range s.global {
		add(key)
	}
	for _, tier := range tiers {
		for _, e := range tier.Entries {
			// An unlinked global does not reach this checkout, and an invalid
			// entry never reaches Airflow at all; neither is injected, so
			// neither is something the project gets.
			if !e.Unlinked && e.Invalid == "" {
				add(e.EnvKey)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
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
