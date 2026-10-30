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
	// generated over the runtime base the pin, runtime build and
	// requires-python resolve to.
	//
	// It takes a context because the base is not always local: an Airflow 2
	// image is tagged by runtime version, so which runtime carries a given
	// Airflow is a lookup against Astronomer's runtime catalog. Deploy refuses
	// Airflow 2; docker mode runs it, and that is the one difference.
	//
	// The returned request carries BaseImage, or Dockerfile and Context, plus
	// ManifestDir, Dependencies, FindLinks and Packages. The engine fills in
	// the rest. An
	// adapter copying fields across must copy every one: a field it leaves
	// out reaches the build as its zero value, with nothing to say so.
	Request(ctx context.Context, m ManifestBuild) (BuildRequest, error)
	// Build returns the image to run. A generated build (no Dockerfile)
	// starts FROM BaseImage's -base tag, BaseImage + "-base", the same image
	// without the runtime's ONBUILD triggers, and installs the project's OS
	// packages and dependencies in steps of its own. With nothing to install
	// it builds nothing and returns that -base tag itself: never BaseImage,
	// and never req.Tag. So a consumer tells the fast path from a build by
	// comparing the result with req.Tag, which every build returns; comparing
	// it with BaseImage matches neither. A declared Dockerfile always builds
	// and returns req.Tag.
	Build(ctx context.Context, req BuildRequest, cb Callbacks) (string, error)
}

// ManifestBuild is the part of a project manifest that decides which image
// the project builds, and is imagebuild.ManifestBuild (an alias): declared
// here so ImageBuilder can take it without importing imagebuild.
//
// Fill it with imagebuild.ManifestBuildOf, which maps a *manifest.Manifest to
// every field, so a field added here is wired once rather than at each
// consumer; a Plan carries the same fields and moves them with
// Plan.ManifestBuild and Plan.SetManifestBuild. The fields stay plain, rather
// than a pkg/manifest type, so a caller can override one after filling it,
// and so this package keeps its empty dependency list.
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
	// LockedRuntime is the runtime build the project's uv.lock was seeded
	// taken from (manifest UV.RuntimeBuild), the build its uv.lock was made
	// for, or "" for none. With Runtime empty, a local or packaged generated
	// image starts FROM it instead of the newest build of the series, and a
	// deploy prefers it when the Deployment offers it. Unused when a
	// Dockerfile is declared, whose FROM names the build.
	LockedRuntime string
	// LockedPython is the Python the uv.lock was seeded for on LockedRuntime
	// (manifest UV.RuntimePython), or "" for none: an image of LockedRuntime
	// runs it, when requires-python admits it, so the image runs the Python
	// the lock was made for.
	LockedPython string
	// LockedAirflow is the Astronomer build of Airflow pyproject.toml records
	// beside LockedRuntime ("3.1.3+astro.2"), the Airflow its uv.lock was made
	// for, or "" for none: a build that falls back from LockedRuntime to one
	// carrying another Airflow release is refused, as the lock's versions were
	// not resolved for it.
	LockedAirflow string
	// RequiresPython is [project] requires-python. A generated image runs the
	// newest Python of its runtime build that it admits, preferring the
	// build's default (imagebuild.RuntimeImageForPython).
	RequiresPython string
	// Dockerfile is [tool.astro] dockerfile: slash-separated and relative to
	// ProjectDir, or empty when the project declares none.
	Dockerfile string
	// Dependencies are [project] dependencies (PEP 508), as
	// manifest.Requirements gives them.
	Dependencies []string
	// FindLinks are the per-package pages of the indexes [tool.uv.sources]
	// routes a dependency to (manifest.UV.IndexPages), which a generated
	// image's install searches besides PyPI.
	FindLinks []string
	// Packages are [tool.astro] packages, OS (apt) package names.
	Packages []string
}

// ManifestBuild is the image-deciding part of the plan, for the project at
// projectDir: the fields SetManifestBuild sets, read back. It leaves the
// plan's ProjectPath alone, since an engine passes the path it resolved.
func (p Plan) ManifestBuild(projectDir string) ManifestBuild {
	return ManifestBuild{
		ProjectDir:     projectDir,
		AirflowVersion: p.AirflowVersion,
		Runtime:        p.Runtime,
		LockedRuntime:  p.LockedRuntime,
		LockedPython:   p.LockedPython,
		LockedAirflow:  p.LockedAirflow,
		RequiresPython: p.RequiresPython,
		Dockerfile:     p.Dockerfile,
		Dependencies:   p.Dependencies,
		FindLinks:      p.FindLinks,
		Packages:       p.Packages,
	}
}

// SetManifestBuild copies mb's image-deciding fields into the plan, the
// inverse of ManifestBuild. mb.ProjectDir is not copied: ProjectPath is the
// plan's own, set by its builder.
func (p *Plan) SetManifestBuild(mb ManifestBuild) {
	p.AirflowVersion = mb.AirflowVersion
	p.Runtime = mb.Runtime
	p.LockedRuntime = mb.LockedRuntime
	p.LockedPython = mb.LockedPython
	p.LockedAirflow = mb.LockedAirflow
	p.RequiresPython = mb.RequiresPython
	p.Dockerfile = mb.Dockerfile
	p.Dependencies = mb.Dependencies
	p.FindLinks = mb.FindLinks
	p.Packages = mb.Packages
}

// BuildRequest is what the runtime asks an ImageBuilder for. It mirrors
// imagebuild.Request rather than being it: naming that type here would reinstate
// the import this seam exists to avoid. The adapter on the consumer's side is a
// field-for-field copy, which is the price of the boundary.
type BuildRequest struct {
	// WorkDir is where the build context and Dockerfile are written.
	WorkDir string
	// BaseImage is the resolved runtime image the build is of: the runtime's
	// own tag (runtime:3.1-5), which names the runtime and is what a caller
	// parses a version out of. The build starts FROM its -base tag, and the
	// fast path returns that tag (ImageBuilder.Build), not this one. Empty
	// when Dockerfile is set — that file declares its own FROM.
	BaseImage string
	// Dockerfile and Context ask the builder to run the project's own Dockerfile
	// rather than generate one, for a project whose manifest declared it. Both
	// absolute, set together, and both empty for a generated build. Dependencies
	// and Packages are then that file's business rather than the runtime's.
	Dockerfile string
	Context    string
	// Secrets are docker build --secret specs. A generated build passes on
	// only the netrc one, which its Python install mounts.
	Secrets []string
	// Tag is the image reference the build produces.
	Tag string
	// ManifestDir is the project whose pyproject.toml and uv.lock a generated
	// build installs from (imagebuild.Request.ManifestDir): the builder
	// resolves the lock into the lists the image installs before it builds.
	// When it does, Dependencies, FindLinks and Packages are ignored; when it
	// cannot, or ManifestDir is empty, it installs those three, unpinned. An
	// adapter that drops it builds the image without the lock's versions.
	ManifestDir string
	// Dependencies are the manifest's Python dependencies (PEP 508).
	Dependencies []string
	// FindLinks are pages the Python install searches besides PyPI
	// (ManifestBuild.FindLinks).
	FindLinks []string
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
