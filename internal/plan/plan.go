// Package plan turns the project on disk into a localrt.Plan. It is the CLI's
// composition seam: from a working directory it discovers the project, loads
// and validates its manifest, reads user state, resolves the local
// environment, and assembles the value struct the runtime starts Airflow
// from. cmd/local calls Build and PersistPort; nothing here prints, and
// nothing about cobra or rendering leaks in.
package plan

import (
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
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
	// WorkspaceProvider, when set, turns on Environment Manager resolution for
	// names declared `source = "workspace"`. nil leaves a workspace source
	// unresolved (a required one gates as missing) — the offline default, no
	// network.
	//
	// It is a constructor rather than a client because the workspace comes from
	// the manifest, which this package is what reads. Taking the Astro client
	// directly would make every plan build import a platform, which the layer
	// rules forbid below cmd/ (docs/v2-architecture.md).
	WorkspaceProvider func(workspace, domain string, reveal bool) envresolve.Provider
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
		return nil, err
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

	return &Built{
		Project:          proj,
		EnvWarnings:      resolved.warnings,
		StartedWithout:   resolved.startedWithout,
		ManifestWarnings: m.Warnings,
		Pools:            m.Astro.Pools,
		Plan: localrt.Plan{
			ProjectPath:    proj.Dir,
			Mode:           opts.Mode,
			AirflowVersion: m.Airflow().Pin,
			// The one runtime build Docker mode builds FROM, when the manifest
			// names one; standalone installs the requirement and ignores it.
			Runtime: m.Airflow().Runtime,
			// Docker mode installs these into the runtime image for parity
			// with standalone, which gets them from the uv venv sync.
			Dependencies: m.Project.Dependencies,
			// OS packages: docker mode bakes them into the image; standalone
			// mode has no image and is warned about them (cmd/local start).
			Packages: m.Astro.Packages,
			// The project's own Dockerfile, when it declared one. Docker mode
			// then runs that file as the build and AirflowVersion, Dependencies
			// and Packages stop describing the image; standalone ignores it.
			//
			// This was missing rather than deliberately omitted, and the gap was
			// only visible from outside: a converted project that KEPT a
			// Dockerfile (one doing more than naming a base image) built from
			// that file in the desktop, which inferred the tier from the file
			// being present, and got a generated image here, which read no such
			// thing. Same project, two tools, two different images, and the one
			// that dropped the user's RUN steps was this one.
			Dockerfile:   m.Astro.Dockerfile,
			BuildSecrets: util.ResolveProjectBuildSecrets(opts.BuildSecretFlags, m.Astro.BuildSecretSpecs()),
			// Empty when the manifest states requires-python: the venv is
			// built inside the project, so uv reads it from the manifest
			// itself and passing it would only restate what uv is about to
			// read. Without one, uv would pick the newest CPython it knows of,
			// which an Airflow 2 pin cannot run under, so the fallback names
			// a version. Astro Desktop applies the same function, so the two
			// tools build the same interpreter for the same manifest.
			PythonVersion:   airflowrt.PythonFallback(m.Project.RequiresPython, m.Airflow().Pin),
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
		},
	}, nil
}

// Environ is the environment a start of the project in dir runs its DAGs
// under, in os.Environ form: this process's own, then the
// ASTRONOMER_ENVIRONMENT both engines set, then Plan.Env, then Plan.SecretEnv,
// the order standalone applies them in. It is for commands that
// import the project's DAGs without starting Airflow, so a DAG that reads its
// .env at import time sees what it would under a start.
//
// Offline, unlike a start: no Environment Manager lookup. A required value with
// no source is left out rather than refused, so the DAG that needs it reports
// the problem itself.
func Environ(dir string, m *manifest.Manifest) ([]string, error) {
	resolved, err := resolveEnv(m, &project.Project{Dir: dir}, Options{AllowMissing: true})
	if err != nil {
		return nil, err
	}
	env := append(os.Environ(), "ASTRONOMER_ENVIRONMENT=local")
	for _, layer := range []map[string]string{resolved.env, resolved.secretEnv} {
		for _, k := range slices.Sorted(maps.Keys(layer)) {
			env = append(env, k+"="+layer[k])
		}
	}
	return env, nil
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
// against the provider chain (shell env > project .env > project vault > global
// vault > global ~/.astro/env),
// and returns the environment injected into Airflow at start plus the
// declared names only the shell satisfies (Plan.PassthroughEnv). A required
// value with no source surfaces as *MissingEnvError — the clone-and-run gate.
//
// Injection is not the resolved map: the project .env goes in wholesale
// (every entry, docker-compose semantics) and the global file contributes
// only its schema-declared entries (localenv.Sources.Injection). Both engines
// apply the result identically through Plan.Env.
//
// The vault's values come back separately, for Plan.SecretEnv, and follow the
// same shape one tier down: the project's own secrets wholesale, the
// machine-wide ones only where the schema declares them
// (vaultenv.SecretInjection).
// resolvedEnv is what resolveEnv hands Build: the environment in its two
// halves, the names docker passes through from the shell, the value-level
// warnings, and any required value the start was allowed past.
type resolvedEnv struct {
	env, secretEnv map[string]string
	passthrough    []string
	warnings       []envschema.Violation
	startedWithout []envresolve.Missing
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
		in.WorkspaceProvider = opts.WorkspaceProvider(m.Astro.Workspace, m.Astro.WorkspaceDomain(), true)
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
	// They are reported instead, as Built.EnvWarnings. `astro local check`
	// does not validate the environment yet, so this is the only
	// thing that surfaces them.
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
	inj := src.Injection(schema)
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
	// Membership in `inj` was the wrong test, in both directions. `inj` carries
	// the global file's schema-declared entries, and those sit BELOW both vault
	// tiers, so keying on it let ~/.astro/env delete a secret that had won. And a
	// name the shell environment satisfied is not in `inj` at all, so keying on it
	// left the secret in place and let it override an explicit
	// `FOO=bar astro local start`. Both inverted the documented order.
	//
	// The resolver already made this decision for every DECLARED name, so ask it
	// rather than re-deriving. A name the schema does not declare was never
	// resolved — the project tier injects wholesale — so those are checked
	// against the sources that outrank the vault.
	secretInj := vault.SecretInjection(schema)
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
			continue
		}
		delete(inj, k)
	}
	// After the vault reconciliation, which would otherwise delete these: a
	// workspace value won its name, so nothing else may carry that name, and
	// no file held it — resolveWorkspace only answers when the local chain
	// did not.
	for k, v := range wsInj {
		secretInj[k] = v
		delete(inj, k)
	}
	return resolvedEnv{
		env:            inj,
		secretEnv:      secretInj,
		passthrough:    passthroughKeys(res.Resolved, inj),
		warnings:       valueWarnings(res.Violations),
		startedWithout: startedWithout,
	}, nil
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
