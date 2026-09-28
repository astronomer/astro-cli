package rt

import "context"

// ImageBuilder produces the container image docker mode runs.
//
// An interface rather than a direct dependency because pkg/imagebuild imports
// this contract — Callbacks, LogLine, and Stdio are how a build reports progress
// — so localrt importing it back would close a package cycle. The architecture
// doc's rule for exactly this case is that a sub-module takes what it needs as a
// constructor argument, and internal/pack already consumes imagebuild through an
// equivalent seam of its own.
//
// The CLI passes an adapter over *imagebuild.Builder. Astro Desktop passes the
// same, which is why imagebuild is a pkg/ sub-module rather than CLI-private.
type ImageBuilder interface {
	// RuntimeImage resolves the base image for an Airflow version. It takes a
	// context because the answer is not always local: Airflow 3 tags its
	// runtime images by the Airflow version, but an Airflow 2 image is tagged
	// by runtime version, so which runtime carries a given Airflow is a lookup
	// against Astronomer's version service.
	//
	// runtime is the plan's Runtime, the one build the manifest's
	// [tool.astro] runtime names, or "" for none. When set, the base is that
	// build and no lookup is made; airflowVersion is still passed, for the
	// checks that hold whichever build is chosen.
	RuntimeImage(ctx context.Context, airflowVersion, runtime string) (string, error)
	// Build layers the project's dependencies and OS packages over BaseImage and
	// returns the image to run. With nothing to install it returns BaseImage
	// unchanged rather than building an empty layer.
	Build(ctx context.Context, req BuildRequest, cb Callbacks) (string, error)
}

// BuildRequest is what the runtime asks an ImageBuilder for. It mirrors
// imagebuild.Request rather than being it: naming that type here would reinstate
// the import this seam exists to avoid. The adapter on the consumer's side is a
// field-for-field copy, which is the price of the boundary.
type BuildRequest struct {
	// WorkDir is where the build context and Dockerfile are written.
	WorkDir string
	// BaseImage is the resolved runtime image the build starts FROM. Empty when
	// Dockerfile is set — that file declares its own FROM.
	BaseImage string
	// Dockerfile and Context ask the builder to run the project's own Dockerfile
	// rather than generate one, for a project whose manifest declared it. Both
	// absolute, set together, and both empty for a generated build. Dependencies
	// and Packages are then that file's business rather than the runtime's.
	Dockerfile string
	Context    string
	// Secrets are docker build --secret specs. A generated build passes on
	// only the netrc one, which the runtime image mounts while it installs.
	Secrets []string
	// Tag is the image reference the build produces.
	Tag string
	// Dependencies are the manifest's Python dependencies (PEP 508).
	Dependencies []string
	// Packages are the manifest's OS (apt) package names.
	Packages []string
	// Bin is the container CLI to shell out to; Env reaches its daemon.
	Bin string
	Env []string
}

// ProxyDaemon is the reverse-proxy lifecycle the engines drive: started after a
// route lands so <name>.localhost resolves, reaped when the last route goes.
//
// An interface because the two consumers run different proxies — the CLI shells out
// to its daemon, the desktop runs one in-process — and because the CLI's
// implementation pulls in config/, which these packages must not import. Nil is
// valid and means no daemon at all (Windows, tests).
type ProxyDaemon interface {
	// EnsureRunning starts the daemon if it isn't already running and returns
	// the port it bound.
	EnsureRunning() (string, error)
	// StopIfEmpty stops the daemon when no routes remain, so the last project
	// to stop leaves no orphan behind.
	StopIfEmpty()
}
