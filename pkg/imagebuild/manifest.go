package imagebuild

import (
	"context"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// ManifestBuild is the part of a v2 project manifest that decides which image
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
	return ManifestBuild{
		ProjectDir:     dir,
		AirflowVersion: airflow.Pin,
		Runtime:        airflow.Runtime,
		RequiresPython: m.Project.RequiresPython,
		Dockerfile:     m.Astro.Dockerfile,
		Dependencies:   m.Requirements(),
		Packages:       m.Astro.Packages,
	}
}

// baseImageFunc resolves the runtime base a generated build starts FROM, from
// the manifest's Airflow pin, its [tool.astro] runtime build ("" for none) and
// its requires-python.
type baseImageFunc func(ctx context.Context, m ManifestBuild) (string, error)

// ForManifest is the one rule for which image a project's manifest builds, so
// every caller building from a manifest builds the same image from it.
//
// A declared Dockerfile is the build: the returned Request carries the file,
// resolved against the project, and the project as its context, and no runtime
// base is resolved, since the file names its own FROM and asking for a base it
// would not use only adds a way to fail. Otherwise the image is generated over
// the runtime base the Airflow pin, runtime build and requires-python resolve
// to (RuntimeImageForPython), installing Dependencies and Packages; a pin it refuses is
// the error returned.
// Dependencies and Packages are carried in both cases, since Build ignores
// them in Dockerfile mode rather than rejecting them.
//
// Deploy and `astro package astro` call this. Local Docker mode calls
// ForLocalManifest, the same rule over a base that also takes Airflow 2.
//
// The caller fills the rest: WorkDir, Tag, Platform ("linux/amd64" for a
// deploy), Bin, Env, and Secrets. A generated build reads only the
// manifest.RuntimeSecretID secret.
func ForManifest(m ManifestBuild, catalog func() *runtimeversions.Catalog) (Request, error) {
	return forManifestWith(context.Background(), m, func(_ context.Context, m ManifestBuild) (string, error) {
		return RuntimeImageForPython(m.AirflowVersion, m.Runtime, m.RequiresPython, catalog)
	})
}

// ForLocalManifest is ForManifest for local Docker mode. The one difference is
// the base a generated build starts FROM: LocalRuntimeImageWith, which resolves
// an Airflow 3 pin exactly as ForManifest does (RuntimeImageForPython, its
// Python read from the runtime catalog o describes) and also runs Airflow 2,
// looking it up in that catalog. So an Airflow 3 project
// starts from the image it deploys, and an Airflow 2 one, which deploy refuses,
// still starts.
func ForLocalManifest(ctx context.Context, m ManifestBuild, o runtimeversions.Options) (Request, error) {
	return forManifestWith(ctx, m, func(ctx context.Context, m ManifestBuild) (string, error) {
		return LocalRuntimeImageWith(ctx, m.AirflowVersion, m.Runtime, m.RequiresPython, o)
	})
}

// forManifestWith is the rule ForManifest and ForLocalManifest share, with a
// generated build's base resolved by base. base is not called for a declared
// Dockerfile.
func forManifestWith(ctx context.Context, m ManifestBuild, base baseImageFunc) (Request, error) {
	req := Request{
		Dependencies: m.Dependencies,
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
// nothing to install, is skipped here, and the one-line `FROM <base>` is built
// anyway (the runtime image's ONBUILD triggers run over the empty
// requirements.txt and packages.txt and install nothing). Pulling the base
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
