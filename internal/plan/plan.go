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

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// Options carries the per-invocation choices the command line supplies. The
// manifest, user state, and vault supply everything else.
type Options struct {
	// Mode is the runtime the command asked for. "" leaves the choice to
	// the runtime, which defaults to standalone.
	Mode localrt.Mode
	// RequestedPort is the --port value, 0 when the flag was not given.
	RequestedPort int
	// StopWithSession ties Airflow's lifetime to the calling process.
	StopWithSession bool
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
	env, err := resolveEnv(m, proj)
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
			// PythonVersion is left empty on purpose: uv resolves the
			// interpreter from the manifest's requires-python, so a specifier
			// like ">=3.10" never reaches uv's --python, which wants a
			// concrete version.
			StopWithSession: opts.StopWithSession,
			Env:             env,
			Hostname:        proj.Hostname,
			StateDir:        stateDir,
			RequestedPort:   choosePort(opts.RequestedPort, us.Port),
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
// against the process env and the shared vault, and returns the assembled
// Airflow environment. A required value with no source surfaces as
// *MissingEnvError — the clone-and-run gate.
func resolveEnv(m *manifest.Manifest, proj *project.Project) (map[string]string, error) {
	schema, err := envresolve.ParseSchema(m.Astro.Env)
	if err != nil {
		return nil, err
	}
	// The vault is opened only when the schema declares names, so a project
	// with no env schema never touches the OS keyring (which can prompt).
	var store secrets.Store
	if len(schema.EnvVars)+len(schema.AirflowVariables)+len(schema.Connections) > 0 {
		store = openVault()
	}
	// Scope is the symlink-resolved absolute project path: scoped vault
	// entries beat global ones (internal/envresolve). ProjectID hashes the
	// same path, but the vault keys on the path itself.
	scope, err := filepath.EvalSymlinks(proj.Dir)
	if err != nil {
		return nil, err
	}
	res, err := envresolve.Resolve(envresolve.Inputs{
		Schema:  schema,
		Scope:   scope,
		Store:   store,
		Environ: os.Environ(),
	})
	if err != nil {
		return nil, err
	}
	// The start gate is missing-required only: a value with no source blocks
	// the run (the clone-and-run message). Value-level problems on values
	// that are present — a wrong type, a corrupt connection JSON — are left
	// in res.Violations for `astro local check`, the command whose
	// whole job is validating without starting. Gating start on them too
	// would split that responsibility across two commands.
	if len(res.Missing) > 0 {
		return nil, &MissingEnvError{
			Project:          proj.Dir,
			Missing:          res.Missing,
			VaultUnavailable: res.VaultUnavailable,
		}
	}
	return res.Env, nil
}

// openVault opens the shared local vault (secrets.DefaultService,
// <astro home>/secrets), the same store desktop uses so a secret saved in
// either is readable in both. A construction failure degrades to env-only
// resolution: the store is nil and Resolve reports values may exist that this
// machine cannot read.
func openVault() secrets.Store {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home, _ = os.UserHomeDir() //nolint:errcheck // a lookup failure degrades to env-only resolution, per the doc above
	}
	store, err := secrets.NewKeyringStore(secrets.Config{
		Service: secrets.DefaultService,
		Dir:     filepath.Join(home, ".astro", "secrets"),
	})
	if err != nil {
		return nil
	}
	return store
}
