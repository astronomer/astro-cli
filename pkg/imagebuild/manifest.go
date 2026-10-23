package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// ManifestBuild is the part of a project manifest that decides which image
// the project builds. It is declared in rt, so the local runtime's image seam
// (rt.ImageBuilder.Request) takes the same type without importing this
// package; see rt.ManifestBuild for its fields.
type ManifestBuild = rt.ManifestBuild

// ManifestBuildOf reads the fields that decide a project's image out of its
// manifest, for the project rooted at dir (absolute). It is the one place that
// mapping lives: the CLI's local start, deploy and `astro package astro`, and
// Astro Desktop, all fill a ManifestBuild through it, so a field added to
// ManifestBuild is wired once here and every consumer builds the same image.
// A nil m yields just ProjectDir.
//
// It lives here rather than beside ManifestBuild in rt because rt keeps an
// empty dependency list, and reading a manifest needs pkg/manifest.
func ManifestBuildOf(dir string, m *manifest.Manifest) ManifestBuild {
	if m == nil {
		return ManifestBuild{ProjectDir: dir}
	}
	airflow := m.Airflow()
	deps := m.Requirements()
	return ManifestBuild{
		ProjectDir:     dir,
		AirflowVersion: airflow.Pin,
		Runtime:        airflow.Runtime,
		LockedRuntime:  lockedRuntime(m),
		RequiresPython: m.Project.RequiresPython,
		Dockerfile:     m.Astro.Dockerfile,
		Dependencies:   deps,
		// Only the pages of what a generated image installs: the runtime
		// already is Airflow (runtimeDeps), so the Airflow sources every
		// `astro init` project carries would only make each build reach
		// Astronomer's index, which a firewalled build host cannot.
		FindLinks: m.UV.IndexPagesFor(runtimeDeps(deps)),
		Packages:  m.Astro.Packages,
	}
}

// lockedRuntime is the build m's runtime pins were taken from, the one its
// uv.lock was made for, while it still carries the Airflow the pin asks for: its
// tag agrees with the pin, and so does the Astronomer build of Airflow pinned
// beside it, which names the exact release (an Airflow 2 build's tag shows
// only the generation). A pin moved since the pins were written leaves them to
// the next refresh, and the base to the pin. A [tool.astro] runtime or a
// declared Dockerfile still decides over it; the resolution, not this,
// applies that.
func lockedRuntime(m *manifest.Manifest) string {
	build, pin := m.UV.RuntimeBuild, m.Airflow().Pin
	if build == "" {
		return ""
	}
	if t, ok := manifest.ParseRuntimeTag(build); !ok || !t.Build || !t.Agrees(pin) {
		return ""
	}
	if !manifest.AstroBuildCovered(pin, m.UV.ConstraintDependencies) {
		return ""
	}
	return build
}

// lockedBase is base with the build the project's lock was made for
// (ManifestBuild.LockedRuntime) named as its runtime when the manifest names
// none, so a local or packaged image starts FROM the build the lock matches
// rather than the newest build of the series. A deploy goes through
// deploybuild.OfferedBase instead, which takes it only when the Deployment
// offers it.
//
// A requires-python that admits none of the Pythons the lock's build ships
// (edited since the pins were written) falls back to the rule without it,
// the newest build of the series, rather than failing the build;
// LockedRuntimeNotices says so.
func lockedBase(base BaseImageFunc) BaseImageFunc {
	return func(ctx context.Context, m ManifestBuild) (string, error) {
		if m.Runtime != "" || m.LockedRuntime == "" {
			return base(ctx, m)
		}
		locked := m
		locked.Runtime = m.LockedRuntime
		image, err := base(ctx, locked)
		if notShipped := (*runtimeversions.PythonNotShippedError)(nil); errors.As(err, &notShipped) {
			return base(ctx, m)
		}
		return image, err
	}
}

// LockedRuntimeNotices are what a local or packaged build of m has to say
// about the build its lock was made for (LockedRuntime), read against
// catalog: that requires-python admits none of that build's Pythons, so the
// image builds on the newest build of the series instead; else that the
// build is yanked, or else that a newer build of the pin is available, either
// of which the project moves off only by running astro init, since moving the
// build moves the lock. Nothing when the manifest
// names its own build, when there is no lock build, or with no catalog.
func LockedRuntimeNotices(m ManifestBuild, catalog *runtimeversions.Catalog) []string {
	if m.Runtime != "" || m.Dockerfile != "" || m.LockedRuntime == "" || catalog == nil {
		return nil
	}
	_, _, err := runtimeversions.ProjectPython(m.AirflowVersion, m.LockedRuntime, m.RequiresPython,
		func() *runtimeversions.Catalog { return catalog })
	if notShipped := (*runtimeversions.PythonNotShippedError)(nil); errors.As(err, &notShipped) {
		return []string{fmt.Sprintf(
			"requires-python %s admits none of the Pythons Astro Runtime %s ships (%s), the build this project's uv.lock was made for, so the image builds on the newest build of Airflow %s instead. Run astro init to move the lock to a build it admits",
			notShipped.RequiresPython, m.LockedRuntime, strings.Join(notShipped.Pythons, ", "), m.AirflowVersion)}
	}
	if r, ok := catalog.Runtime(m.LockedRuntime); ok && r.Yanked {
		why := ""
		if reason := strings.TrimSpace(r.YankedReason); reason != "" {
			why = " (" + reason + ")"
		}
		move := "Run astro init to move to another build"
		if newest, ok := catalog.NewestPublishedRuntimeFor(m.AirflowVersion); ok {
			move = "Run astro init to move to " + newest
		}
		return []string{fmt.Sprintf("Astro Runtime %s, which this project builds on, is yanked%s. %s", m.LockedRuntime, why, move)}
	}
	if newer, ok := catalog.NewerBuildFor(m.AirflowVersion, m.LockedRuntime); ok {
		return []string{fmt.Sprintf("Astro Runtime %s is available; this project builds on %s. Run astro init to move to it", newer, m.LockedRuntime)}
	}
	return nil
}

// BaseImageFunc resolves the runtime base a generated build starts FROM, from
// the manifest's Airflow pin, its [tool.astro] runtime build ("" for none) and
// its requires-python.
type BaseImageFunc func(ctx context.Context, m ManifestBuild) (string, error)

// ForManifest is the one rule for which image a project's manifest builds, so
// every caller building from a manifest builds the same image from it.
//
// A declared Dockerfile is the build: the returned Request carries the file,
// resolved against the project, and the project as its context, and no runtime
// base is resolved, since the file names its own FROM and asking for a base it
// would not use only adds a way to fail. Otherwise the image is generated over
// the runtime base the Airflow pin, runtime build and requires-python resolve
// to (RuntimeImageForPython), installing Dependencies and Packages; a pin it refuses is
// the error returned. The runtime build is [tool.astro] runtime, else the build
// the project's lock was made for (LockedRuntime), else none, which is the
// newest build of the series.
// Dependencies and Packages are carried in both cases, since Build ignores
// them in Dockerfile mode rather than rejecting them.
//
// `astro package astro` calls this, for Airflow 2 (from 2.7) and Airflow 3;
// an Airflow 2 base with no [tool.astro] runtime is looked up in catalog, so
// a nil catalog refuses it. A deploy knows better which builds its target
// takes, and calls ForManifestWith over a base that falls back to
// CatalogBase. Local Docker mode calls ForLocalManifest, the same rule over a
// base it reads the catalog for itself.
//
// The caller fills the rest: WorkDir, Tag, Platform ("linux/amd64" for a
// deploy), Bin, Env, and Secrets. A generated build reads only the
// manifest.RuntimeSecretID secret.
func ForManifest(m ManifestBuild, catalog func() *runtimeversions.Catalog) (Request, error) {
	return ForManifestWith(context.Background(), m, lockedBase(CatalogBase(catalog)))
}

// CatalogBase is ForManifest's base: RuntimeImageForPython over catalog, for
// a caller of ForManifestWith that wants the same rule behind its own.
func CatalogBase(catalog func() *runtimeversions.Catalog) BaseImageFunc {
	return func(_ context.Context, m ManifestBuild) (string, error) {
		return RuntimeImageForPython(m.AirflowVersion, m.Runtime, m.RequiresPython, catalog)
	}
}

// ForManifestWith is ForManifest with the generated build's runtime base
// resolved by base, for a caller that knows better than the public catalog
// which runtime builds its target accepts. Everything else is ForManifest's
// rule: a declared Dockerfile is the build and base is not called, and a
// generated image ships, so it carries the project's files.
func ForManifestWith(ctx context.Context, m ManifestBuild, base BaseImageFunc) (Request, error) {
	return forManifestWith(ctx, m, shipped, base)
}

// Whether the image a manifest builds is shipped, which decides whether a
// generated one carries the project's files (Request.ProjectFiles).
const (
	shipped = true
	mounted = false
)

// ForLocalManifest is ForManifest for local Docker mode. The one difference is
// the base a generated build starts FROM: LocalRuntimeImageWith, which resolves
// a pin exactly as ForManifest does, reading the runtime catalog o describes,
// and, for Airflow 2, waiting as long as o says and reporting why a catalog
// could not be read. So a project starts from the base it deploys.
func ForLocalManifest(ctx context.Context, m ManifestBuild, o runtimeversions.Options) (Request, error) {
	// Local start runs the project mounted, so its image holds the
	// dependencies alone.
	return forManifestWith(ctx, m, mounted, lockedBase(func(ctx context.Context, m ManifestBuild) (string, error) {
		return LocalRuntimeImageWith(ctx, m.AirflowVersion, m.Runtime, m.RequiresPython, o)
	}))
}

// forManifestWith is the rule ForManifest and ForLocalManifest share, with a
// generated build's base resolved by base. base is not called for a declared
// Dockerfile. A generated build of an image that ships carries the project's
// files; a declared Dockerfile's context is the project already.
func forManifestWith(ctx context.Context, m ManifestBuild, ships bool, base BaseImageFunc) (Request, error) {
	req := Request{
		Dependencies: m.Dependencies,
		FindLinks:    m.FindLinks,
		Packages:     m.Packages,
	}
	if m.Dockerfile != "" {
		// Dockerfile and Context are set together or not at all: the file
		// without the project as context builds against the generated
		// directory, where none of the project's COPY paths exist.
		req.Dockerfile = filepath.Join(m.ProjectDir, filepath.FromSlash(m.Dockerfile))
		req.Context = m.ProjectDir
		return req, nil
	}
	image, err := base(ctx, m)
	if err != nil {
		return Request{}, err
	}
	req.BaseImage = image
	req.ManifestDir = m.ProjectDir
	if ships {
		req.ProjectFiles = m.ProjectDir
	}
	return req, nil
}

// FromDeclaredDockerfile reports whether the request builds the project's own
// Dockerfile rather than an image generated over the runtime base.
func (r *Request) FromDeclaredDockerfile() bool {
	return r.Dockerfile != ""
}

// BuildLocal is Build for a caller that goes on to tag, inspect, save or push
// the image. It always builds and always returns req.Tag: a single-platform
// image in the local store, built at req.Platform when one is set.
//
// Build's fast path, which returns the runtime base unbuilt when there is
// nothing to install, is skipped here, and the generated Dockerfile is built
// anyway (its install steps run over the empty requirements.txt and
// packages.txt and install nothing). Pulling the base
// instead is not enough. The base tag names a multi-platform index, and with
// Docker's containerd image store and only a foreign platform pulled (amd64 on
// an arm64 machine), `docker image inspect` without --platform reads an empty
// config: no labels, so the runtime version cannot be read, and a push of that
// tag would send an index whose other platforms are not local. A built tag
// names one platform, so inspect, tag, save and push all see the same image
// whatever the store or engine version.
func (b *Builder) BuildLocal(ctx context.Context, req Request, cb rt.Callbacks) (string, error) {
	return b.buildImage(ctx, req, cb, false)
}
