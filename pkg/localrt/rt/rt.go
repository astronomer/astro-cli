// Package rt is the local-runtime contract: the types both consumers and engines
// speak, and nothing else. Its dependency list is empty and must stay that way.
//
// It sits below pkg/localrt rather than in it for two reasons.
//
// The first is mechanical: localrt's Start and Attach import the engines, and the
// engines need Plan, Callbacks, and the rest, so types in the parent would be an
// import cycle.
//
// The second is why this package is public rather than internal. Importing
// pkg/localrt links the whole local runtime — both engines, the record store, the
// prune predicate, and through them pkg/proxy, pkg/container, pkg/uv, and
// pkg/fsatomic. Code that only needs to SPEAK the contract should not pay for the
// machinery that implements it: pkg/imagebuild wants four progress types, and
// Astro Desktop's supervisor shim wants a few markers. Both import a leaf instead
// and link no engine.
//
// pkg/localrt re-exports everything here as aliases, so callers that do want the
// runtime keep writing localrt.Plan and get the identical type.
package rt

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Mode selects how Airflow runs. The values are wire-coupled: they appear in
// the state record and in routes.json (pkg/proxy Route.Mode — pkg/proxy's
// RouteModeStandalone/RouteModeDocker carry the same wire values), so
// changing a string breaks tools already in the field.
type Mode string

const (
	// ModeStandalone runs Airflow from a uv-managed venv, no Docker daemon.
	ModeStandalone Mode = "standalone"
	// ModeDocker runs Airflow via compose. The only mode on Windows in the MVP.
	ModeDocker Mode = "docker"
)

// Plan is everything needed to start Airflow for a project, as plain values.
// The code that builds a Plan (internal/plan in the CLI, the app layer in
// desktop) resolves manifest, config, and env layering, and fills in
// defaults, before this point. No manifest, config, cloud, or houston
// (v1 Software API) types cross this boundary.
type Plan struct {
	ProjectPath string
	Mode        Mode
	// AirflowVersion is the version the project's Airflow requirement pins,
	// "3.3" or "3.3.2": the runtime image's series in docker mode, and the
	// generation, and so the process layout, in both.
	AirflowVersion string
	// Runtime is the manifest's [tool.astro] runtime (manifest.Airflow().Runtime),
	// one Astro Runtime build, "3.3-8" or "13.11.0", or "" for none. Docker
	// mode builds a generated image FROM that build rather than the newest one
	// of AirflowVersion's series. Standalone ignores it: it installs the
	// requirement. The manifest never sets it beside a Dockerfile.
	Runtime string
	// PythonVersion is the interpreter standalone asks uv for ("3.13"), or ""
	// to let uv pick within requires-python. A caller sets it with
	// imagebuild.StandalonePython: the Python a generated image of the same
	// manifest runs when that can be decided, airflowrt.PythonFallback
	// otherwise. uv rebuilds an existing venv on another Python. Docker mode
	// ignores it.
	PythonVersion string
	// RequiresPython is the manifest's [project] requires-python, which picks
	// the Python a generated image runs. Standalone ignores it: uv reads it
	// from the manifest.
	RequiresPython string
	// Dependencies is the project's [project] dependencies (PEP 508 specs).
	// Standalone mode ignores it — uv syncs the venv straight from the
	// manifest — but docker mode installs these into the runtime image so
	// both modes run Airflow against the same set of packages.
	Dependencies []string
	// Packages is the project's [tool.astro] packages, the OS (apt) packages
	// it needs at the system level. Docker mode bakes them into the runtime
	// image through the ONBUILD packages.txt step; standalone mode has no
	// image and cannot honor them; StandaloneOmissions reports them.
	Packages []string
	// Dockerfile is a project-relative path to the project's own Dockerfile,
	// set when the manifest declared one ("tier 3" in the project design: the
	// escape hatch for multi-stage builds and anything else a manifest cannot
	// express). Docker mode then runs that file as the build, and
	// AirflowVersion, Dependencies, and Packages stop describing the image —
	// the file does. Empty means the image is generated from the manifest,
	// which is the common case.
	//
	// Standalone mode ignores it: there is no image, so a project pinned to a
	// Dockerfile has nothing standalone can honor. StandaloneOmissions reports
	// it, for the caller to tell its user.
	//
	// AirflowVersion is still required with this set. It does not pick the
	// image any more, but the runtime needs the generation to decide the
	// compose service set (Airflow 2 has no dag-processor) and the env it
	// writes, and reading that back out of a user's Dockerfile would be a
	// guess.
	Dockerfile string
	// BuildSecrets are docker build --secret specs ("id=mysecret,src=/path" or
	// "id=mysecret,env=VAR") for the image build. Only the spec is
	// carried: the container CLI reads each value from the file or the variable
	// it names. Never recorded, so a later start that rebuilds needs them again.
	// Standalone mode ignores them, and a generated image reads only netrc.
	BuildSecrets []string
	// StopWithSession ties Airflow's lifetime to the process that starts
	// it: true means Airflow is killed when that process exits; false (the
	// default) means Airflow keeps running and any tool can reconnect to
	// it later. Persisted in the state record so other tools can see which
	// way a running Airflow was started.
	StopWithSession bool
	// Env is the fully layered process environment for Airflow. Docker mode writes
	// these into its compose file; see SecretEnv for values that must not be
	// written anywhere.
	Env map[string]string
	// PassthroughEnv names env vars satisfied only by the calling shell's
	// environment. It never carries values, so engines that persist their
	// configuration (docker mode's compose file) can hand the names to the
	// runtime without writing the values to disk. Standalone mode ignores
	// it: the Airflow process inherits the shell environment directly.
	PassthroughEnv []string
	// SecretEnv is environment the consumer does not want written into the
	// runtime's own configuration. Same delivery as Env, different persistence:
	// docker mode declares these keys in its compose file with no value and
	// supplies the values to the compose process itself, so the file records that
	// the variable exists and never what it holds.
	//
	// Be precise about what that buys, because it is narrower than "never written
	// anywhere" and a consumer sizing a threat model off the wrong sentence will
	// get it wrong. The compose file is protected, and it is the artifact that
	// outlives the containers — it sits in the state directory until a --clean
	// stop removes it. The container is not: compose resolves the declarations at
	// creation and the daemon persists the result into the container's own config,
	// where `docker inspect` reads it back for as long as the container exists.
	// The property delivered is "not in our file, and gone when the containers
	// are gone".
	//
	// The distinction is not stylistic. Env is written into a compose file that
	// outlives the containers, and a consumer whose values come from a keyring —
	// Astro Desktop's connection and variable vault — would otherwise decrypt
	// secrets onto disk as a side effect of starting Airflow, in a file nothing
	// cleans up until a --clean stop.
	//
	// PassthroughEnv does not solve this: it carries no values at all, so it can
	// only describe variables the runtime's own environment already holds. A
	// consumer holding values it must not persist needs to hand them over, which
	// is what this field is for.
	//
	// A key in both Env and SecretEnv is treated as secret — the safer reading of
	// a caller that contradicts itself. Standalone mode makes no distinction: it
	// has no file, so both maps go to the process and neither is persisted.
	SecretEnv map[string]string
	// Hostname is the display hostname for this project, computed by the
	// caller (e.g. from pkg/proxy's derivation). localrt persists it into
	// the state record and reports it in Status; it is a label, never an
	// identity.
	Hostname string
	// StateDir is the per-project runtime state home,
	// ~/.cache/astro/projects/<path-hash>.
	StateDir string
	// AirflowHome is where AIRFLOW_HOME points; project-scoped.
	AirflowHome string
	// RequestedPort is a preference; the runtime may allocate another and
	// reports the real one in Status.
	RequestedPort int
}

// State is the lifecycle state of a local Airflow.
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateError    State = "error"
)

// Status describes a local Airflow, recovered from the state record on disk
// — it must not require in-memory state from the process that started it.
// Proxy routes are derived from it (Route.ProjectDir = ProjectPath,
// Route.Port = itoa(Port)); routes.json itself stays a compatibility view.
// The json tags keep this in step with the rest of the core surface: lowercase
// keys, and the fields a stopped Airflow zeroes (pid, port, startedAt) drop out
// rather than reporting a false 0 or a zero-value timestamp.
type Status struct {
	ProjectPath     string `json:"projectPath"`
	Mode            Mode   `json:"mode,omitempty"`
	StopWithSession bool   `json:"stopWithSession,omitempty"`
	State           State  `json:"state"`
	PID             int    `json:"pid,omitempty"`
	Port            int    `json:"port,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	// AirflowMajor is the Airflow generation this runtime was started for
	// ("2" or "3"), carried from the record. It describes the running
	// process, not the manifest, which may have been edited since. Empty on
	// a record written before the field existed.
	AirflowMajor string    `json:"airflowMajor,omitempty"`
	StartedAt    time.Time `json:"startedAt,omitzero"`
}

// LogLine is one parsed line of component output.
type LogLine struct {
	Component string
	Time      time.Time
	Text      string
}

// Callbacks receives progress while the runtime works. Fields may be nil,
// and implementations must check before calling. This is the only channel
// for progress: the runtime never prints.
type Callbacks struct {
	OnState func(State, error)
	OnLine  func(LogLine)
}

// StopOptions controls Stop.
type StopOptions struct {
	// Force skips the graceful SIGTERM window.
	Force bool
	// Clean also removes derived state (the old `astro dev kill`).
	Clean bool
}

// LogOptions controls Logs. Exactly one of Writer or OnLine is set;
// implementations error otherwise.
type LogOptions struct {
	Follow     bool
	Components []string
	// Tail limits output to the last N lines; 0 means all.
	Tail int
	// Since drops lines older than this; zero means all.
	Since  time.Time
	Writer io.Writer
	OnLine func(LogLine)
}

// Stdio carries the stdin/stdout/stderr for Run and Shell.
type Stdio struct {
	In       io.Reader
	Out, Err io.Writer
}

// ProjectDirInImage is where a project's directories are mounted inside its
// image: AIRFLOW_HOME in the runtime image, so dags/, include/, plugins/ and
// tests/ appear beneath it.
//
// A RunInImage caller needs it because the command's arguments are paths the
// container will resolve, not host paths. Declared here so that caller does not
// hardcode a second copy of the mount layout.
const ProjectDirInImage = "/usr/local/airflow"

// ComposeOverrideFile is the project-relative compose file docker mode merges
// over the one it generates, as v1 did. Standalone mode cannot honor one;
// StandaloneOmissions reports it.
const ComposeOverrideFile = "docker-compose.override.yml"

// HasComposeOverride reports whether projectPath holds a ComposeOverrideFile.
func HasComposeOverride(projectPath string) bool {
	info, err := os.Stat(filepath.Join(projectPath, ComposeOverrideFile))
	return err == nil && info.Mode().IsRegular()
}

// ImageRun is one command to run in a project's own image with no Airflow
// running: the offline counterpart to Airflow.Run, which execs into a live
// container and so needs one.
//
// It exists because a stopped project still has everything needed to inspect
// itself. Checking a project's DAGs is the first caller: it imports them with
// the same interpreter and the same dependencies the real Airflow would, which
// is the only way to get an answer a running Airflow would agree with.
type ImageRun struct {
	// Argv is the program and its arguments.
	//
	// The image's ENTRYPOINT is bypassed. The runtime image's entrypoint waits
	// for the metadata database and prints to stdout while it waits, so an
	// offline command would hang forever and have its output corrupted.
	Argv []string
	// Env is extra environment for the command, layered over what the
	// project's own configuration already supplies.
	Env map[string]string
	// Stdio wires the command's streams. Out and Err stay separate, so a
	// caller parsing one is not reading the other's noise.
	Stdio Stdio
}

// ErrImageNotBuilt reports that a project has no image to run a command in,
// because it has never been started or was stopped with --clean. Distinct from
// a project that is merely not running, which still has one.
var ErrImageNotBuilt = errors.New("this project has no image yet")

// Airflow is a handle to a running local Airflow, obtained from Start or
// Attach — so every method on it is always valid.
type Airflow interface {
	Stop(ctx context.Context, opts StopOptions) error
	Status() (Status, error)
	Logs(ctx context.Context, opts LogOptions) error
	Run(ctx context.Context, argv []string, s Stdio) error
	Shell(ctx context.Context, s Stdio) error

	// Env is the environment Run and Shell use, in os/exec form: the
	// project venv ahead of PATH, AIRFLOW_HOME, the generation-specific
	// settings and the rest of what BuildEnv settles.
	//
	// Run and Shell cover an embedder that wants the engine to run
	// something. This covers one that has to run its own: a terminal hands
	// the user an interactive session the engine never sees, and it still
	// has to be the project's environment or `airflow` in that terminal is
	// a different Airflow from the one on screen.
	//
	// It is what Run uses, NOT everything the running process was started
	// with. Both are rebuilt from the state record, which does not carry the
	// plan's own Env and SecretEnv layers or a relocated state dir — so a
	// project that sets those through its plan rather than its .env gets a
	// command that differs from the scheduler in exactly those variables.
	// The engine's shellEnv documents which, and closing the gap means
	// extending the record.
	//
	// Docker mode has no such environment to hand out — the project's
	// Python lives in a container — and answers ErrNotImplemented.
	Env() ([]string, error)
}

// ErrNotImplemented marks a contract entry point that has no engine behind it
// yet. Declared here rather than in the façade so both can refer to one value.
var ErrNotImplemented = errors.New("not yet implemented")
