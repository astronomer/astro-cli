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
	ProjectPath    string
	Mode           Mode
	AirflowVersion string
	PythonVersion  string
	// Dependencies is the project's [project] dependencies (PEP 508 specs).
	// Standalone mode ignores it — uv syncs the venv straight from the
	// manifest — but docker mode installs these into the runtime image so
	// both modes run Airflow against the same set of packages.
	Dependencies []string
	// Packages is the project's [tool.astro] packages, the OS (apt) packages
	// it needs at the system level. Docker mode bakes them into the runtime
	// image through the ONBUILD packages.txt step; standalone mode has no
	// image and cannot honor them, so it is warned about them at start.
	Packages []string
	// StopWithSession ties Airflow's lifetime to the process that starts
	// it: true means Airflow is killed when that process exits; false (the
	// default) means Airflow keeps running and any tool can reconnect to
	// it later. Persisted in the state record so other tools can see which
	// way a running Airflow was started.
	StopWithSession bool
	// Env is the fully layered process environment for Airflow.
	Env map[string]string
	// PassthroughEnv names env vars satisfied only by the calling shell's
	// environment. It never carries values, so engines that persist their
	// configuration (docker mode's compose file) can hand the names to the
	// runtime without writing the values to disk. Standalone mode ignores
	// it: the Airflow process inherits the shell environment directly.
	PassthroughEnv []string
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
// The json tags keep this in step with the rest of the v2 surface: lowercase
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

// Airflow is a handle to a running local Airflow, obtained from Start or
// Attach — so every method on it is always valid.
type Airflow interface {
	Stop(ctx context.Context, opts StopOptions) error
	Status() (Status, error)
	Logs(ctx context.Context, opts LogOptions) error
	Run(ctx context.Context, argv []string, s Stdio) error
	Shell(ctx context.Context, s Stdio) error
}

// ErrNotImplemented marks a contract entry point that has no engine behind it
// yet. Declared here rather than in the façade so both can refer to one value.
var ErrNotImplemented = errors.New("not yet implemented")
