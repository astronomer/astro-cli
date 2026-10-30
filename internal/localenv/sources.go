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
	SourceAbsent  = "absent"
)

// mapProvider is a labeled bag of env-var keys implementing
// envresolve.Provider. The shell and project sources are both one.
type mapProvider struct {
	label string
	vals  map[string]string
}

func (p mapProvider) Lookup(key string) (string, bool) { v, ok := p.vals[key]; return v, ok }
func (p mapProvider) Label() string                    { return p.label }

// Sources holds the plain resolution sources for a single invocation, read
// once. It builds the ordered provider chain and the runtime injection map
// from the same parsed data, so list, the missing-value gate, and injection
// never disagree about what a file holds.
type Sources struct {
	shell      map[string]string
	project    map[string]string // nil when running outside a project
	hasProject bool
}

// LoadSources reads the shell environment and the project's .env (when
// projectDir is non-empty). environ is the process environment in
// os.Environ() form; the caller passes it so tests can pin it. A missing file
// is empty, not an error.
func LoadSources(environ []string, projectDir string) (Sources, error) {
	s := Sources{shell: environMap(environ)}
	if projectDir != "" {
		m, err := readMap(ProjectEnvPath(projectDir))
		if err != nil {
			return Sources{}, err
		}
		s.project = m
		s.hasProject = true
	}
	return s, nil
}

// Providers is the ordered resolution chain:
//
//	project .env > shell env > project vault > global vault
//
// The linked workspace and declaration defaults rank below these; the resolver
// adds them. The project provider is omitted when there is no project.
//
// vault is the encrypted tier's providers, in their own order
// (internal/vaultenv builds them). They are passed in rather than built here
// because this package writes plain files and touches no keyring, and because
// the caller is the composition root — the same reason the Environment Manager
// provider arrives as a seam. Pass nil for a chain without the vault.
//
// The slot is the decision. A vault value sits BELOW the project's .env, so a
// hand-written plaintext entry still wins, exactly as it does on the desktop.
//
// The project .env sits above the shell because that is what a start delivers:
// Injection applies the whole file through Plan.Env, which both engines set
// over the inherited environment. Ranking it anywhere else would have list,
// get and check name a source whose value Airflow never sees. The shell still
// beats every tier below the file.
func (s Sources) Providers(vault []envresolve.Provider) []envresolve.Provider {
	var ps []envresolve.Provider
	if s.hasProject {
		ps = append(ps, mapProvider{label: SourceProject, vals: s.project})
	}
	ps = append(ps, mapProvider{label: SourceShell, vals: s.shell})
	return append(ps, vault...)
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

// Injection is the map layered into the Airflow environment at start: the
// project .env WHOLESALE (every entry, docker-compose semantics), declared or
// not: a declaration makes a name a requirement, it does not decide whether a
// value is passed. The shell environment is not here, since the process
// already carries it. Both engines apply this identically through Plan.Env.
func (s Sources) Injection() map[string]string {
	inj := map[string]string{}
	for k, v := range s.project {
		inj[k] = v
	}
	return inj
}

// UndeclaredValue is one value a start of the project passes to Airflow from
// this machine without the schema declaring it. Source, DeclareHint and
// RemoveHint are the ones `astro local env list` shows for the same value, so
// a warning built from them names the exact commands the listing does. No
// field is a value.
type UndeclaredValue struct {
	// EnvKey is the env var Airflow gets.
	EnvKey string
	Kind   Kind
	Name   string
	// Source is the list's SOURCE label: "project" for the project's .env, a
	// vault tier's label ("vault", "vault (global)").
	Source      string
	DeclareHint string
	RemoveHint  string
}

// Undeclared is what a start of this project passes to Airflow from local
// sources the schema does not declare: the project .env and the vault tiers'
// entries that reach this checkout. They work locally and nowhere else, since
// a Deployment and a teammate's clone get only what the project declares, so
// check and package name them. Airflow settings (AIRFLOW__*) are left out:
// they configure Airflow rather than name a value the project's code expects,
// the same reason list does not call them orphans.
//
// A key more than one source holds is reported once, from the source a start
// uses: the .env over the vault, and each vault tier over the ones after it
// (tiers come in resolution order). Sorted by env key.
func (s Sources) Undeclared(schema *envschema.Schema, tiers []VaultTier) []UndeclaredValue {
	declared := map[string]bool{}
	for _, k := range envschema.DeclaredEnvKeys(schema) {
		declared[k] = true
	}
	seen := map[string]UndeclaredValue{}
	add := func(key string, item ListItem) {
		if declared[key] || isAirflowSetting(key) {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = UndeclaredValue{
			EnvKey: key, Kind: item.Kind, Name: item.Name, Source: item.Source,
			DeclareHint: item.DeclareHint, RemoveHint: item.RemoveHint,
		}
	}
	for key := range s.project {
		add(key, orphanItem(key, "", s.hasProject))
	}
	for _, tier := range tiers {
		for _, e := range tier.Entries {
			// An unlinked global does not reach this checkout, and an invalid
			// entry never reaches Airflow at all; neither is injected, so
			// neither is something the project gets.
			if !e.Unlinked && e.Invalid == "" {
				add(e.EnvKey, vaultOrphanItem(e.Kind, e.Name, tier, s.hasProject))
			}
		}
	}
	out := make([]UndeclaredValue, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EnvKey < out[j].EnvKey })
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
