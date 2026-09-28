// Package localrt runs local Airflow. It is the runtime behind `astro local`
// and behind Astro Desktop, so it follows the shared sub-module rules
// (docs/v2-architecture.md): no printing, no exiting, and no in-repo imports
// except pkg/airflowrt, the primitives it orchestrates. Progress flows
// through Callbacks; results flow through typed errors.
//
// The engines that implement this live in internal/ beneath this package, which
// makes them unreachable from outside — deliberately. Both consumers enter through
// New and the methods on Runtime, and nothing else is API.
//
// The contract types are declared in the rt leaf below and aliased here. The
// aliases mean callers never see the difference — localrt.Plan IS rt.Plan, not a
// copy of it, so a value built by one is the value the other consumes.
//
// Importing this package links the engines, which is right if you want to RUN
// Airflow and wasteful if you only need to speak the contract. Two leaves exist
// for that: pkg/localrt/rt for the types, and pkg/localrt/supervise for the
// supervisor and its argv markers. Neither links an engine.
package localrt

import (
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localprune"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// The contract, re-exported. See internal/rt for the documentation on each.
type (
	Mode        = rt.Mode
	Plan        = rt.Plan
	State       = rt.State
	Status      = rt.Status
	LogLine     = rt.LogLine
	Callbacks   = rt.Callbacks
	StopOptions = rt.StopOptions
	LogOptions  = rt.LogOptions
	Stdio       = rt.Stdio
	Airflow     = rt.Airflow
	LineWriter  = rt.LineWriter
	ImageRun    = rt.ImageRun
	Omission    = rt.Omission

	OmissionKind = rt.OmissionKind

	// ProxyDaemon and the image-builder seam are supplied by the consumer; see
	// Config.
	ProxyDaemon  = rt.ProxyDaemon
	ImageBuilder = rt.ImageBuilder
	BuildRequest = rt.BuildRequest
)

// ProjectDirInImage is where a project's directories are mounted inside its
// image, for a RunInImage caller building container-side arguments.
const ProjectDirInImage = rt.ProjectDirInImage

// ComposeOverrideFile is the project file docker mode merges over its own
// compose file.
const ComposeOverrideFile = rt.ComposeOverrideFile

const (
	ModeStandalone = rt.ModeStandalone
	ModeDocker     = rt.ModeDocker

	StateStopped  = rt.StateStopped
	StateStarting = rt.StateStarting
	StateRunning  = rt.StateRunning
	StateStopping = rt.StateStopping
	StateError    = rt.StateError
)

// The kinds of Omission Plan.StandaloneOmissions reports.
const (
	OmissionDockerfile      = rt.OmissionDockerfile
	OmissionPackages        = rt.OmissionPackages
	OmissionComposeOverride = rt.OmissionComposeOverride
)

// ErrNotImplemented marks a contract entry point with no engine behind it. Still
// exported: `astro dev` reports it, and it is what a consumer checks while the
// remaining modes land.
var ErrNotImplemented = rt.ErrNotImplemented

// ErrImageNotBuilt reports that a project has no image to run a command in.
var ErrImageNotBuilt = rt.ErrImageNotBuilt

// CanonicalPath resolves a project path to the spelling every tool agrees on.
func CanonicalPath(path string) (string, error) { return rt.CanonicalPath(path) }

// ProjectID is the stable per-project identifier derived from its path.
func ProjectID(projectPath string) (string, error) { return rt.ProjectID(projectPath) }

// CacheRoot is ~/.cache/astro, the home for per-project runtime state.
func CacheRoot() (string, error) { return rt.CacheRoot() }

// StateDir is a project's runtime state home under CacheRoot.
func StateDir(projectPath string) (string, error) { return rt.StateDir(projectPath) }

// RouteAlive is the record-aware prune predicate, for a consumer that keeps its
// own proxy.Store rather than going through Runtime.
//
// pkg/proxy prunes routes.json on every write, and its default predicate asks
// only whether the route's own PID is alive. That is wrong for a standalone
// runtime in a way that shows up as a route disappearing from under a working
// Airflow: `airflow standalone` spawns its components into the process group and
// the master often exits before they do, so the recorded pid can be gone while
// the runtime is serving. This resolves the route to its state record and asks
// that record's mode for the answer, which for standalone is the whole group.
//
// Runtime wires this into its own store already. It is exported because the
// second consumer builds a store of its own — the app owns its proxy lifecycle —
// and two tools writing one routes.json with different liveness rules means each
// prune pass evicts routes the other considers alive.
func RouteAlive(r proxy.Route) bool { return localprune.RouteAlive(r) }
