// Package localrt runs local Airflow. It is the runtime behind `astro local`
// and behind Astro Desktop, so it follows the shared sub-module rules
// (docs/v2-architecture.md): no printing, no exiting, and no in-repo imports
// except pkg/airflowrt, the primitives it orchestrates. Progress flows
// through Callbacks; results flow through typed errors.
//
// The types here are the contract the an earlier fix/43 implementations fill in.
// Signatures may still move while those land; nothing outside this repo and
// Astro Desktop may depend on them yet.
package localrt

import (
	"context"
	"errors"
	"io"
	"time"
)

// Mode selects how Airflow runs. The values are wire-coupled: they appear in
// the state record and in routes.json (pkg/proxy Route.Mode), so changing a
// string breaks tools already in the field.
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
	// StopWithSession ties Airflow's lifetime to the process that starts
	// it: true means Airflow is killed when that process exits; false (the
	// default) means Airflow keeps running and any tool can reconnect to
	// it later. Persisted in the state record so other tools can see which
	// way a running Airflow was started.
	StopWithSession bool
	// Env is the fully layered process environment for Airflow.
	Env map[string]string
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
type Status struct {
	ProjectPath     string
	Mode            Mode
	StopWithSession bool
	State           State
	PID             int
	Port            int
	Hostname        string
	StartedAt       time.Time
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

// ErrNotImplemented marks the contract stubs below.
var ErrNotImplemented = errors.New("not yet implemented")

// Start launches Airflow per the plan and returns a handle to it.
func Start(ctx context.Context, p Plan, cb Callbacks) (Airflow, error) {
	return nil, ErrNotImplemented
}

// Attach returns a handle to an already-running Airflow using only its
// state record on disk — no in-memory state from the process that started
// it.
func Attach(projectPath string) (Airflow, error) {
	return nil, ErrNotImplemented
}

// ReadStatus reads one project's status straight from its state record,
// without the reconnect work Attach does.
func ReadStatus(projectPath string) (Status, error) {
	return Status{}, ErrNotImplemented
}

// List returns every local Airflow known on this machine.
func List() ([]Status, error) {
	return nil, ErrNotImplemented
}
