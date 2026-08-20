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
// The contract types are declared in internal/rt and aliased below. That is an
// import-cycle workaround, not a second layer: Start has to import the engines,
// and the engines need these types, so they cannot live in this package. The
// aliases mean callers never see the difference — localrt.Plan IS rt.Plan, not a
// copy of it, so a value built by one is the value the other consumes.
package localrt

import "github.com/astronomer/astro-cli/pkg/localrt/internal/rt"

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

	// ProxyDaemon and the image-builder seam are supplied by the consumer; see
	// Config.
	ProxyDaemon  = rt.ProxyDaemon
	ImageBuilder = rt.ImageBuilder
	BuildRequest = rt.BuildRequest
)

const (
	ModeStandalone = rt.ModeStandalone
	ModeDocker     = rt.ModeDocker

	StateStopped  = rt.StateStopped
	StateStarting = rt.StateStarting
	StateRunning  = rt.StateRunning
	StateStopping = rt.StateStopping
	StateError    = rt.StateError
)

// ErrNotImplemented marks a contract entry point with no engine behind it. Still
// exported: `astro dev` reports it, and it is what a consumer checks while the
// remaining modes land.
var ErrNotImplemented = rt.ErrNotImplemented

// OnState reports a state transition if the caller asked for one.
func OnState(cb Callbacks, s State, err error) { rt.OnState(cb, s, err) }

// CanonicalPath resolves a project path to the spelling every tool agrees on.
func CanonicalPath(path string) (string, error) { return rt.CanonicalPath(path) }

// ProjectID is the stable per-project identifier derived from its path.
func ProjectID(projectPath string) (string, error) { return rt.ProjectID(projectPath) }

// CacheRoot is ~/.cache/astro, the home for per-project runtime state.
func CacheRoot() (string, error) { return rt.CacheRoot() }

// StateDir is a project's runtime state home under CacheRoot.
func StateDir(projectPath string) (string, error) { return rt.StateDir(projectPath) }
