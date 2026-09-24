package imagebuild

import (
	"context"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// ManifestBuild is the part of a v2 project manifest that decides which image
// the project builds. Plain fields rather than a pkg/manifest type, so this
// module takes no dependency on the manifest parser and a caller holding its
// own manifest read (Astro Desktop's, for one) fills it directly.
type ManifestBuild struct {
	// ProjectDir is the project's root, absolute. A declared Dockerfile resolves
	// against it and builds with it as the context.
	ProjectDir string
	// AirflowVersion is [tool.astro] airflow, the pin a generated build's
	// runtime base resolves from. Unused when a Dockerfile is declared.
	AirflowVersion string
	// Dockerfile is [tool.astro] dockerfile: slash-separated and relative to
	// ProjectDir, or empty when the project declares none.
	Dockerfile string
	// Dependencies are [project] dependencies (PEP 508).
	Dependencies []string
	// Packages are [tool.astro] packages, OS (apt) package names.
	Packages []string
}

// ForManifest is the one rule for which image a project's manifest builds, so
// every caller building from a manifest builds the same image from it.
//
// A declared Dockerfile is the build: the returned Request carries the file,
// resolved against the project, and the project as its context, and no runtime
// base is resolved, since the file names its own FROM and asking for a base it
// would not use only adds a way to fail. Otherwise the image is generated over
// the runtime base the Airflow pin resolves to (RuntimeImage), installing
// Dependencies and Packages; a pin RuntimeImage refuses is the error returned.
// Dependencies and Packages are carried in both cases, since Build ignores
// them in Dockerfile mode rather than rejecting them.
//
// The caller fills the rest: WorkDir, Tag, Platform ("linux/amd64" for a
// deploy), Bin, Env, and Secrets. Secrets only reach a build from a declared
// Dockerfile; see Request.FromDeclaredDockerfile for the check a caller
// accepting secrets applies before building.
func ForManifest(m ManifestBuild) (Request, error) {
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
	base, err := RuntimeImage(m.AirflowVersion)
	if err != nil {
		return Request{}, err
	}
	req.BaseImage = base
	return req, nil
}

// FromDeclaredDockerfile reports whether the request builds the project's own
// Dockerfile rather than an image generated over the runtime base.
//
// It is also the build-secret rule. A generated build's Dockerfile is
// `FROM <base>` with the install in the runtime image's ONBUILD triggers, so
// no step of the project's exists for a secret to be mounted into, and Build
// drops Secrets there. A caller offering secrets refuses them when this is
// false, rather than accepting values the build cannot read.
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
