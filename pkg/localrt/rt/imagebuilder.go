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
	// Request picks the build for a plan's manifest fields. Both consumers
	// answer it with imagebuild.ForLocalManifest, the rule deploy and
	// `astro package astro` follow through imagebuild.ForManifest, so a
	// project starts from the image it deploys: a declared Dockerfile is the
	// build, with the project as its context, and otherwise the image is
	// generated over the runtime base the pin and runtime build resolve to.
	//
	// It takes a context because the base is not always local: an Airflow 2
	// image is tagged by runtime version, so which runtime carries a given
	// Airflow is a lookup against Astronomer's runtime catalog. Deploy refuses
	// Airflow 2; docker mode runs it, and that is the one difference.
	//
	// The returned request carries BaseImage, or Dockerfile and Context, plus
	// Dependencies and Packages. The engine fills in the rest.
	Request(ctx context.Context, m ManifestBuild) (BuildRequest, error)
	// Build layers the project's dependencies and OS packages over BaseImage and
	// returns the image to run. With nothing to install it returns BaseImage
	// unchanged rather than building an empty layer.
	Build(ctx context.Context, req BuildRequest, cb Callbacks) (string, error)
}

// ManifestBuild is the part of a v2 project manifest that decides which image
// the project builds, and is imagebuild.ManifestBuild (an alias): declared
// here so ImageBuilder can take it without importing imagebuild. Plain fields
// rather than a pkg/manifest type, so a caller holding its own manifest read
// (Astro Desktop's, for one) fills it directly.
type ManifestBuild struct {
	// ProjectDir is the project's root, absolute. A declared Dockerfile resolves
	// against it and builds with it as the context.
	ProjectDir string
	// AirflowVersion is the version the manifest's Airflow requirement pins
	// (manifest.Airflow().Pin), which a generated build's runtime base
	// resolves from. Unused when a Dockerfile is declared.
	AirflowVersion string
	// Runtime is [tool.astro] runtime (manifest.Airflow().Runtime), the one
	// runtime build a generated image starts FROM instead of the newest build
	// of AirflowVersion's series, or "" for none. The manifest never sets it
	// beside a Dockerfile.
	Runtime string
	// Dockerfile is [tool.astro] dockerfile: slash-separated and relative to
	// ProjectDir, or empty when the project declares none.
	Dockerfile string
	// Dependencies are [project] dependencies (PEP 508), as
	// manifest.Requirements gives them.
	Dependencies []string
	// Packages are [tool.astro] packages, OS (apt) package names.
	Packages []string
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
