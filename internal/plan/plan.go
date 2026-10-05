// Package plan turns the project on disk into a localrt.Plan. It is the CLI's
// composition seam: from a working directory it discovers the project, loads
// and validates its manifest, reads user state, resolves the local
// environment, and assembles the value struct the runtime starts Airflow
// from. cmd/local calls Build and PersistPort; nothing here prints, and
// nothing about cobra or rendering leaks in.
package plan

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/util"
)

// Options carries the per-invocation choices the command line supplies. The
// manifest, user state, and the env files supply everything else.
type Options struct {
	// Mode is the runtime the command asked for. "" leaves the choice to
	// the runtime, which defaults to standalone.
	Mode localrt.Mode
	// RequestedPort is the --port value, 0 when the flag was not given.
	RequestedPort int
	// StopWithSession ties Airflow's lifetime to the calling process.
	StopWithSession bool
	// WorkspaceProvider, when set, turns on the linked workspace's tier: the
	// manifest's workspace's Environment Manager objects reach Airflow below
	// the global vault and above declaration defaults, declared or not, and a name
	// declared `source = "workspace"` resolves from them. nil leaves the tier
	// out and a workspace source unresolved (a required one gates as missing)
	// — the offline default, no network.
	//
	// It is a constructor rather than a client because the workspace comes from
	// the manifest, which this package is what reads. Taking the Astro client
	// directly would make every plan build import a platform, which the layer
	// rules forbid below cmd/ (docs/architecture.md).
	WorkspaceProvider func(astro *manifest.Astro, reveal bool) envresolve.Provider
	// AllowMissing starts even when a required value has no source, instead
	// of returning *MissingEnvError: `--allow-missing`, and Astro Desktop's
	// Start anyway. The missing values come back on Built.StartedWithout for
	// the caller to warn about. Nothing is invented for them.
	AllowMissing bool
	// BuildSecretFlags are the --build-secret specs for a declared
	// Dockerfile's build. Build resolves them against BUILD_SECRET_INPUT and
	// the manifest's build-secrets (util.ResolveProjectBuildSecrets); whether
	// asking for them makes sense is cmd's to decide.
	BuildSecretFlags []string
	// PythonCatalog reads the runtime catalog for a standalone plan, to pick
	// the venv's Python the way a generated image picks its own (VenvPython).
	// It returns nil when the catalog cannot be read, and is called only for
	// an Airflow 3 manifest that states requires-python. nil, or a docker-mode
	// plan, keeps the rule that needs no catalog (airflowrt.PythonFallback).
	PythonCatalog func() *runtimeversions.Catalog
}

// Built is a resolved plan plus the discovered project, so cmd can persist the
// chosen port against the project after the runtime reports it.
type Built struct {
	Plan    localrt.Plan
	Project *project.Project
	// EnvWarnings are the value-level findings a start does NOT refuse over: a
	// value present but not the shape its declaration promised, or a connection
	// that resolved to a different conn_type. Missing values are not here —
	// those are *MissingEnvError and stop the run.
	//
	// Carried out rather than printed, because cmd/ prints and internal/ does
	// not, and the caller chooses between text and json. Nil when everything
	// conformed.
	EnvWarnings []envschema.Violation
	// StartedWithout is every required value that had no source when the start
	// was allowed past them (Options.AllowMissing). Empty otherwise: without
	// the option, a missing value is *MissingEnvError and there is no Built.
	StartedWithout []envresolve.Missing
	// ManifestWarnings are the manifest's own findings that do not stop a
	// load (manifest.Manifest.Warnings), carried out for the same reason as
	// EnvWarnings.
	ManifestWarnings []manifest.Problem
	// WorkspaceNote is one line saying the linked workspace could not be read
	// and the start went on without its values, or empty when it was read, the
	// manifest links none, or the tier was not asked for. Carried out for the
	// same reason as EnvWarnings.
	WorkspaceNote string
	// PythonNote is one line saying requires-python admits none of the
	// Pythons the runtime build ships, so a generated image of the project
	// would be refused while the venv goes ahead on uv's choice; empty
	// otherwise. Carried out for the same reason as EnvWarnings.
	PythonNote string
	// Pools are the manifest's [tool.astro.pools], which a start creates or
	// updates once Airflow answers. They are not part of the Plan: the
	// runtime starts Airflow, and the pools go in over its API afterwards.
	Pools map[string]manifest.Pool
}

// Build discovers the project containing workingDir, loads and validates its
// manifest, reads user state, resolves the local environment, and assembles a
// localrt.Plan. A missing project (*project.NotFoundError), a manifest that
// does not validate (a declared Dockerfile whose FROM names another Airflow
// than the requirement among them, scaffold.CheckDockerfileAirflow), or a
// required env value with no source on this machine (*MissingEnvError) each
// surface as a typed error the caller renders.
func Build(workingDir string, opts Options) (*Built, error) {
	proj, err := project.Discover(workingDir)
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(filepath.Join(proj.Dir, project.Marker))
	if err != nil {
		return nil, project.LoadError(workingDir, proj.Dir, err)
	}
	// In both modes, and standalone most of all: standalone installs the
	// requirement, so a declared Dockerfile whose FROM names another Airflow is
	// exactly the silent split between the two modes this refuses.
	if err := scaffold.CheckDockerfileAirflow(proj.Dir, m); err != nil {
		return nil, err
	}
	us, err := userstate.Load(proj.Dir)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveEnv(m, proj, opts)
	if err != nil {
		return nil, err
	}
	stateDir, err := localrt.StateDir(proj.Dir)
	if err != nil {
		return nil, err
	}
	catalog := opts.PythonCatalog
	if opts.Mode == localrt.ModeDocker {
		// Docker mode picks the image's Python itself, from the same function,
		// and never reads PythonVersion: no catalog read for nothing.
		catalog = nil
	}
	python, pythonNote := VenvPython(m, catalog)

	p := localrt.Plan{
		ProjectPath:  proj.Dir,
		Mode:         opts.Mode,
		BuildSecrets: util.ResolveProjectBuildSecrets(opts.BuildSecretFlags, m.Astro.BuildSecretSpecs()),
		// The Python a generated image of this manifest runs, so the venv
		// and the image agree (VenvPython). Docker mode never reads it.
		PythonVersion:   python,
		StopWithSession: opts.StopWithSession,
		Env:             resolved.env,
		// The vault's values, kept out of Env on purpose: docker mode
		// writes Env into the compose file it leaves in the state
		// directory, and a decrypted credential on disk would undo the
		// reason the vault exists. SecretEnv is declared there without a
		// value and handed to the compose process instead; standalone
		// treats it as ordinary environment.
		SecretEnv:      resolved.secretEnv,
		PassthroughEnv: resolved.passthrough,
		Hostname:       proj.Hostname,
		StateDir:       stateDir,
		RequestedPort:  choosePort(opts.RequestedPort, us.Port),
	}
	p.SetManifestBuild(imagebuild.ManifestBuildOf(proj.Dir, m))

	return &Built{
		Project:          proj,
		PythonNote:       pythonNote,
		EnvWarnings:      resolved.warnings,
		StartedWithout:   resolved.startedWithout,
		WorkspaceNote:    resolved.workspaceNote,
		ManifestWarnings: m.Warnings,
		Pools:            m.Astro.Pools,
		Plan:             p,
	}, nil
}

// VenvPython is the interpreter a venv built for the project asks uv for
// (imagebuild.StandalonePython: the Python the project's image runs, so the
// two modes agree), and a line to warn with when the project's requires-python
// admits none of its runtime build's Pythons, empty otherwise. That does not
// stop a venv, which goes ahead on uv's choice within requires-python: the
// note says an image would be refused, so a start finds out what a deploy
// would.
func VenvPython(m *manifest.Manifest, catalog func() *runtimeversions.Catalog) (python, note string) {
	python, err := imagebuild.StandalonePython(m.Airflow().Pin, m.Airflow().Runtime, m.Project.RequiresPython, catalog)
	if err != nil {
		note = err.Error() + ", so an image of this project would not build; the environment uses the Python uv picks for requires-python"
	}
	return python, note
}

// EnvReport is what a start's gate would have said about the declared
// environment, for a command that resolves it without starting: the required
// values with no source, and the value-level warnings a start prints.
type EnvReport struct {
	// Missing is every required value with no source, as *MissingEnvError
	// would list it. A name declared source = "workspace" is among them when
	// nothing local holds it, marked Missing.Workspace: resolution here is
	// offline, so the caller decides what an unasked Environment Manager
	// means.
	Missing  []envresolve.Missing
	Warnings []envschema.Violation
}

// EnvironReport returns the environment a start of the project in dir runs its DAGs
// under, in os.Environ form: this process's own, then the
// ASTRONOMER_ENVIRONMENT both engines set, then Plan.Env, then Plan.SecretEnv,
// the order standalone applies them in. It is for commands that
// import the project's DAGs without starting Airflow, so a DAG that reads its
// .env at import time sees what it would under a start.
//
// Offline, unlike a start: no Environment Manager lookup. A required value with
// no source is left out of the environment rather than refused, and comes back
// in the report instead, beside the warnings a start prints: what the start
// gate would have said, from the same one resolution.
func EnvironReport(dir string, m *manifest.Manifest) ([]string, EnvReport, error) {
	resolved, err := resolveEnv(m, &project.Project{Dir: dir}, Options{AllowMissing: true})
	if err != nil {
		return nil, EnvReport{}, err
	}
	rep := EnvReport{Missing: resolved.startedWithout, Warnings: resolved.warnings}
	env := append(os.Environ(), "ASTRONOMER_ENVIRONMENT=local")
	for _, layer := range []map[string]string{resolved.env, resolved.secretEnv} {
		for _, k := range slices.Sorted(maps.Keys(layer)) {
			env = append(env, k+"="+layer[k])
		}
	}
	return env, rep, nil
}

// UndeclaredLocal is the env-var names a start of the project in dir passes to
// Airflow from this machine without the manifest declaring them
// (localenv.Sources.Undeclared). It reads the files and the vault's listing and
// decrypts nothing, so it opens no keyring.
func UndeclaredLocal(dir string, m *manifest.Manifest) ([]string, error) {
	schema, err := envschema.ParseSchema(m.Astro.Env)
	if err != nil {
		return nil, err
	}
	src, err := localenv.LoadSources(os.Environ(), dir)
	if err != nil {
		return nil, err
	}
	return src.Undeclared(schema, vaultenv.Load(dir).Tiers()), nil
}

// UndeclaredNote is the sentence check and package print for UndeclaredLocal's
// names and the manifest's linked workspace, or empty when there are neither.
//
// The workspace's objects reach a start declared or not too, but check and
// package stay offline and never read them, so the note names the workspace
// rather than its objects, and says they were not checked.
func UndeclaredNote(names []string, workspace string) string {
	var parts []string
	if len(names) > 0 {
		parts = append(parts, fmt.Sprintf("this project gets %s locally without declaring %s, so %s will not follow it to a Deployment "+
			"or a teammate's clone. Declare what it needs to make it a requirement: astro local env <kind> declare NAME "+
			"(run astro local env list to see where each comes from).",
			strings.Join(names, ", "), pronoun(len(names), "it", "them"), pronoun(len(names), "it", "they")))
	}
	if workspace != "" {
		parts = append(parts, fmt.Sprintf("a start also passes Airflow what workspace %s holds, declared or not, "+
			"below every local source. Those values are not checked here, since this runs offline: run astro local env list "+
			"to see them, and declare what the project needs to make it a requirement.", workspace))
	}
	return strings.Join(parts, " ")
}

func pronoun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// choosePort applies the v2 requested-port precedence:
//
//	--port flag > user state > manifest default > runtime allocator.
//
// It returns the preference handed to the runtime as Plan.RequestedPort; 0
// means "no preference", which the runtime turns into its own default port
// and then, if that is taken, the proxy allocator. The manifest carries no
// port field yet, so that tier is a deliberate gap, not an omission — add it
// between user state and 0 when one exists. The runtime reports the port it
// actually bound; PersistPort writes that back so the next start prefers it.
func choosePort(flagPort, userPort int) int {
	if flagPort > 0 {
		return flagPort
	}
	return userPort // 0 when user state has no preference
}

// PersistPort records the port the runtime actually bound, so the next start
// for this project prefers it. It is a no-op when nothing changed or when the
// runtime reported no port.
func PersistPort(projectPath string, chosen int) error {
	if chosen <= 0 {
		return nil
	}
	us, err := userstate.Load(projectPath)
	if err != nil {
		return err
	}
	if us.Port == chosen {
		return nil
	}
	us.Port = chosen
	return userstate.Save(projectPath, us)
}

// resolveEnv types the manifest's [tool.astro.env] section, resolves it
// against the provider chain (project .env > shell env > project vault > global
// vault > linked workspace > declaration default), and returns the environment injected into Airflow at start plus the
// declared names only the shell satisfies (Plan.PassthroughEnv). A required
// value with no source surfaces as *MissingEnvError — the clone-and-run gate.
//
// Injection is not the resolved map: everything that reaches the project goes
// in, declared or not. The project .env goes in wholesale
// (localenv.Sources.Injection), and the vault's project secrets and every
// global linked to this checkout come back separately, for Plan.SecretEnv
// (vaultenv.SecretInjection). A declaration is a requirement, not a gate: it
// refuses a start when nothing supplies the name, and types the value.
//
// A name more than one source holds goes to the highest in the chain above,
// with one exception kept from before: the project .env, applied wholesale,
// also beats the shell for a name the schema does not declare.
// resolvedEnv is what resolveEnv hands Build: the environment in its two
// halves, the names docker passes through from the shell, the value-level
// warnings, and any required value the start was allowed past.
type resolvedEnv struct {
	env, secretEnv map[string]string
	passthrough    []string
	warnings       []envschema.Violation
	startedWithout []envresolve.Missing
	workspaceNote  string
}

func resolveEnv(m *manifest.Manifest, proj *project.Project, opts Options) (resolvedEnv, error) {
	var startedWithout []envresolve.Missing
	schema, err := envschema.ParseSchema(m.Astro.Env)
	if err != nil {
		return resolvedEnv{}, err
	}
	src, err := localenv.LoadSources(os.Environ(), proj.Dir)
	if err != nil {
		return resolvedEnv{}, err
	}
	// The vault shared with Astro Desktop. Opened here rather than inside
	// localenv because this is the composition root and that package holds the
	// plaintext files; opening it costs no keyring access until a name actually
	// resolves from it.
	vault := vaultenv.Load(proj.Dir)
	in := envresolve.Inputs{Schema: schema, Providers: src.Providers(vault.Providers())}
	if opts.WorkspaceProvider != nil {
		// reveal = true: start needs the real values to run Airflow. Both modes:
		// a value resolved here travels as SecretEnv (below), which docker mode
		// hands to the compose process rather than writing into the compose
		// file, so it stays off disk either way — the posture Astro Desktop
		// already runs its docker starts under.
		in.WorkspaceProvider = opts.WorkspaceProvider(&m.Astro, true)
	}
	res, err := envresolve.Resolve(in)
	if err != nil {
		return resolvedEnv{}, err
	}
	// The start GATE is missing-required only: a value with no source blocks
	// the run (the clone-and-run message). Value-level problems on values that
	// ARE present — a wrong type, a connection of the wrong kind, a corrupt
	// connection JSON — do not stop a start, because the value may well work
	// and a project that cannot start over its own annotation is a tool arguing
	// with its user.
	//
	// They are reported instead, as Built.EnvWarnings, and `astro local check`
	// reports the same ones through EnvironReport.
	if len(res.Missing) > 0 {
		if !opts.AllowMissing {
			return resolvedEnv{}, &MissingEnvError{
				Project: proj.Dir,
				Missing: res.Missing,
			}
		}
		startedWithout = res.Missing
	}
	// The file sources inject from disk; the manifest defaults and the
	// Environment Manager values are not on disk, so layer them in here. They
	// only fill keys no local file held (local always wins), so this never
	// overrides a file value. The Environment Manager ones are split off into
	// wsInj, to travel as SecretEnv.
	winner := make(map[string]string, len(res.Resolved))
	for _, r := range res.Resolved {
		if r.Found {
			winner[r.EnvKey] = r.Source
		}
	}
	inj := src.Injection()
	wsInj := map[string]string{}
	for k, v := range res.Injected {
		if winner[k] == string(envschema.SourceWorkspace) {
			wsInj[k] = v
			continue
		}
		inj[k] = v
	}
	// The vault's own injection, separate the whole way down so it can be
	// carried as SecretEnv. The two maps reach the engine as one environment, so
	// a name in both would be decided by whichever the engine applied last
	// rather than by the chain — which means everything the chain already decided
	// has to be applied here by hand.
	//
	// Membership in `inj` is the wrong test: a name the shell environment
	// satisfied is not in `inj` at all, so keying on it would leave the secret
	// in place and let it override an explicit `FOO=bar astro local start`,
	// inverting the documented order.
	//
	// The resolver already made this decision for every DECLARED name, so ask it
	// rather than re-deriving. A name the schema does not declare was never
	// resolved — both vault tiers inject everything that reaches the checkout —
	// so those are checked against the sources that outrank the vault.
	secretInj := vault.SecretInjection()
	// shellWon is the undeclared names a lower source held that the shell beat.
	// Standalone inherits the shell, but a docker container does not, so these
	// have to join the passthrough list or docker gets no value at all.
	var shellWon []string
	for k := range secretInj {
		if source, declared := winner[k]; declared {
			if vaultenv.IsVaultSource(source) {
				// The vault won, so the file's value must not travel beside it.
				// Leaving it in Env is not merely redundant: docker writes Env
				// into the compose file it leaves in the state directory, so a
				// plaintext credential that LOST would still land on disk.
				delete(inj, k)
			} else {
				delete(secretInj, k)
			}
			continue
		}
		if src.AboveVault(k) {
			delete(secretInj, k)
			if src.InShell(k) {
				shellWon = append(shellWon, k)
			}
			continue
		}
		delete(inj, k)
	}
	// After the vault reconciliation, which would otherwise delete these: a
	// workspace value won its name, so nothing else may carry that name, and
	// no file held it — the resolver asks the workspace only when the local
	// chain did not answer.
	for k, v := range wsInj {
		secretInj[k] = v
		delete(inj, k)
	}
	var wsNote string
	if in.WorkspaceProvider != nil && m.Astro.Workspace != "" {
		shellWon = append(shellWon, workspaceUndeclared(in.WorkspaceProvider, schema, src, inj, secretInj)...)
		wsNote = workspaceNote(in.WorkspaceProvider, m.Astro.Workspace)
	}
	return resolvedEnv{
		env:            inj,
		secretEnv:      secretInj,
		passthrough:    withShellWon(passthroughKeys(res.Resolved, inj), shellWon, inj, secretInj),
		warnings:       valueWarnings(res.Violations),
		startedWithout: startedWithout,
		workspaceNote:  wsNote,
	}, nil
}

// workspaceUndeclared adds to secretInj the linked workspace's values for the
// names the schema does not declare and nothing local supplies: the workspace
// tier reaches a project whole, as every local tier does, below all of them.
// inj and secretInj are what the local tiers inject, already reconciled; a
// name either carries is theirs. It returns the names the shell beat the
// workspace for, which docker has to pass through. An empty value is skipped
// rather than injected as a blank, and a secret the org withheld never
// resolves, the rules Astro Desktop injects by.
//
// Travels as SecretEnv, like a declared workspace value, so docker hands it to
// the compose process rather than writing it into the compose file.
func workspaceUndeclared(wp envresolve.Provider, schema *envschema.Schema, src localenv.Sources, inj, secretInj map[string]string) []string {
	declared := map[string]bool{}
	for _, k := range envschema.DeclaredEnvKeys(schema) {
		declared[k] = true
	}
	var shellWon []string
	for _, k := range envresolve.Keys(wp) {
		if declared[k] {
			continue // the resolver already placed it in the chain
		}
		if _, ok := inj[k]; ok {
			continue
		}
		if _, ok := secretInj[k]; ok {
			continue
		}
		if src.InShell(k) {
			shellWon = append(shellWon, k)
			continue
		}
		if v, ok := wp.Lookup(k); ok && v != "" {
			secretInj[k] = v
		}
	}
	return shellWon
}

// workspaceNote is the line a start prints when the linked workspace could not
// be read, or, when it was, the line naming any keys it holds that cannot be
// env-var names; empty otherwise.
func workspaceNote(wp envresolve.Provider, workspace string) string {
	short, cause := envresolve.Outage(wp)
	if short == "" {
		return envresolve.SkippedNote(wp, workspace)
	}
	return fmt.Sprintf("workspace %s not read (%s): starting without its values. %s", workspace, short, cause)
}

// valueWarnings is the violations a start reports without refusing: everything
// except the missing ones, which reached the caller as *MissingEnvError.
//
// The exclusion does not fire today. missingReport maps every ViolationMissing
// to a Missing entry one-for-one, so a non-empty res.Missing means the gate
// above already returned, and reaching here implies there are none to filter.
// It is kept, and unit-tested directly, because the invariant lives in a
// different function than the assumption: a start that proceeded despite
// missing values would otherwise report an error and a duplicate warning for
// the same name.
func valueWarnings(all []envschema.Violation) []envschema.Violation {
	var out []envschema.Violation
	for _, v := range all {
		if v.Kind != envschema.ViolationMissing {
			out = append(out, v)
		}
	}
	return out
}

// withShellWon adds the undeclared names the shell beat a lower source for to
// the passthrough list, once each and sorted after the declared ones. A name
// something else still carries (Env or SecretEnv) is left out: that value is
// what both engines apply.
func withShellWon(keys, shellWon []string, env, secretEnv map[string]string) []string {
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		seen[k] = true
	}
	extra := make([]string, 0, len(shellWon))
	for _, k := range shellWon {
		_, inEnv := env[k]
		_, inSecret := secretEnv[k]
		if seen[k] || inEnv || inSecret {
			continue
		}
		seen[k] = true
		extra = append(extra, k)
	}
	slices.Sort(extra)
	return append(keys, extra...)
}

// passthroughKeys is the Airflow env-var names for declared values the shell
// environment alone satisfies — in no file and not injected. Standalone
// Airflow inherits them from the process, but docker containers inherit no
// host shell env, so the plan carries the names — never the
// values, which must stay off disk — for docker mode to pass through.
func passthroughKeys(resolved []envresolve.ResolvedName, inj map[string]string) []string {
	var keys []string
	for _, r := range resolved {
		if !r.Found || r.Source != localenv.SourceShell {
			continue
		}
		if _, onDisk := inj[r.EnvKey]; onDisk {
			continue
		}
		keys = append(keys, r.EnvKey)
	}
	return keys
}
