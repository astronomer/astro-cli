// Package plan turns the project on disk into a localrt.Plan. It is the CLI's
// composition seam: from a working directory it discovers the project, loads
// and validates its manifest, reads user state, resolves the local
// environment, and assembles the value struct the runtime starts Airflow
// from. cmd/local calls Build and PersistPort; nothing here prints, and
// nothing about cobra or rendering leaks in.
package plan

import (
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
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
	WorkspaceProvider func(workspace string, reveal bool) envresolve.Provider
}

// Built is a resolved plan plus the discovered project, so cmd can persist the
// chosen port against the project after the runtime reports it.
type Built struct {
	Plan    localrt.Plan
	Project *project.Project
}

// Build discovers the project containing workingDir, loads and validates its
// manifest, reads user state, resolves the local environment, and assembles a
// localrt.Plan. A missing project (*project.NotFoundError), a manifest that
// does not validate, or a required env value with no source on this machine
// (*MissingEnvError) each surface as a typed error the caller renders.
func Build(workingDir string, opts Options) (*Built, error) {
	proj, err := project.Discover(workingDir)
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(filepath.Join(proj.Dir, project.Marker))
	if err != nil {
		return nil, err
	}
	us, err := userstate.Load(proj.Dir)
	if err != nil {
		return nil, err
	}
	env, secretEnv, passEnv, err := resolveEnv(m, proj, opts)
	if err != nil {
		return nil, err
	}
	stateDir, err := localrt.StateDir(proj.Dir)
	if err != nil {
		return nil, err
	}

	return &Built{
		Project: proj,
		Plan: localrt.Plan{
			ProjectPath:    proj.Dir,
			Mode:           opts.Mode,
			AirflowVersion: m.Astro.AirflowVersion,
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
			Dockerfile: m.Astro.Dockerfile,
			// PythonVersion is left empty on purpose: uv resolves the
			// interpreter from the manifest's requires-python, so a specifier
			// like ">=3.10" never reaches uv's --python, which wants a
			// concrete version.
			StopWithSession: opts.StopWithSession,
			Env:             env,
			// The vault's values, kept out of Env on purpose: docker mode
			// writes Env into the compose file it leaves in the state
			// directory, and a decrypted credential on disk would undo the
			// reason the vault exists. SecretEnv is declared there without a
			// value and handed to the compose process instead; standalone
			// treats it as ordinary environment.
			SecretEnv:      secretEnv,
			PassthroughEnv: passEnv,
			Hostname:       proj.Hostname,
			StateDir:       stateDir,
			RequestedPort:  choosePort(opts.RequestedPort, us.Port),
		},
	}, nil
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
func resolveEnv(m *manifest.Manifest, proj *project.Project, opts Options) (env, secretEnv map[string]string, passthrough []string, err error) {
	schema, err := envresolve.ParseSchema(m.Astro.Env)
	if err != nil {
		return nil, nil, nil, err
	}
	src, err := localenv.LoadSources(os.Environ(), proj.Dir)
	if err != nil {
		return nil, nil, nil, err
	}
	// The vault shared with Astro Desktop. Opened here rather than inside
	// localenv because this is the composition root and that package holds the
	// plaintext files; opening it costs no keyring access until a name actually
	// resolves from it.
	vault := vaultenv.Load(proj.Dir)
	in := envresolve.Inputs{Schema: schema, Providers: src.Providers(vault.Providers())}
	if opts.WorkspaceProvider != nil {
		if opts.Mode == localrt.ModeDocker {
			// Docker start writes Plan.Env into the on-disk compose file, so a
			// resolved Environment Manager value would land on disk — the one
			// thing the read-through posture rules out. Withhold it in docker
			// mode (stage 1); standalone injects it in memory only.
			in.WorkspaceProvider = emenv.Unavailable("Environment Manager values are injected in standalone mode only; run without --docker, or set it locally")
		} else {
			// reveal = true: start needs the real values to run Airflow.
			in.WorkspaceProvider = opts.WorkspaceProvider(m.Astro.Workspace, true)
		}
	}
	res, err := envresolve.Resolve(in)
	if err != nil {
		return nil, nil, nil, err
	}
	// The start gate is missing-required only: a value with no source blocks
	// the run (the clone-and-run message). Value-level problems on values
	// that are present — a wrong type, a corrupt connection JSON — are left
	// in res.Violations for `astro local check`, the command whose
	// whole job is validating without starting. Gating start on them too
	// would split that responsibility across two commands.
	if len(res.Missing) > 0 {
		return nil, nil, nil, &MissingEnvError{
			Project: proj.Dir,
			Missing: res.Missing,
		}
	}
	// The file sources inject from disk; the Environment Manager values are not
	// on disk, so layer them in here. They only fill keys no local file held
	// (local always wins), so this never overrides a file value.
	inj := src.Injection(schema)
	for k, v := range res.Injected {
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
	winner := make(map[string]string, len(res.Resolved))
	for _, r := range res.Resolved {
		if r.Found {
			winner[r.EnvKey] = r.Source
		}
	}
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
	return inj, secretInj, passthroughKeys(res.Resolved, inj), nil
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
