package airflowrt

import (
	"strings"
)

// How an image installs a project over an Astro Runtime base: the recipe every
// build path runs. It lives here, beside the rest of what this package knows
// about runtime images, because pkg/imagebuild writes it for a generated build
// and for the install base a project's own Dockerfile builds on, and
// pkg/scaffold writes the Dockerfile that builds on that base.
//
// What a build installs is decided before the build, by the CLI, which
// resolves the project's pyproject.toml and uv.lock with the host's uv and
// writes the lists: requirements.txt, uv-lock-constraints.txt (which
// requirements.txt names with -c) and packages.txt, and, for the packages a
// [tool.uv.sources] entry routes to an index, routed-requirements.txt and
// routed-links.html. The build resolves nothing; it installs the lists from
// its own context.
//
// A generated build is the recipe over the runtime's -base image, then the
// project's files (GeneratedDockerfile). A project's own Dockerfile builds on
// an install base instead: the CLI builds the recipe alone over the -base
// image into a local image (InstallBaseDockerfile), and passes its tag to the
// project's Dockerfile as the build argument BaseArg, which the file's final
// stage is FROM (BaseArgHeader). So the project's file states nothing about
// the install, and nothing is written into the project.

// The install lists (pkg/imagebuild's InstallLists.Files). The first two are
// the names the runtime's install scripts read from their working directory;
// the third is the file of uv.lock's versions requirements.txt names with -c;
// the last two are the routed install's requirements and the page of the
// files it finds them on.
const (
	RequirementsName    = "requirements.txt"
	PackagesName        = "packages.txt"
	LockConstraintsName = "uv-lock-constraints.txt"
	RoutedName          = "routed-requirements.txt"
	RoutedLinksName     = "routed-links.html"
)

// BaseArg is the build argument a project's own Dockerfile builds its final
// stage FROM: the install base the CLI builds from pyproject.toml and uv.lock
// (InstallBaseDockerfile), which a build passes as
// `--build-arg ASTRO_BASE=<tag>`. A build that passes none fails at that FROM,
// whose reference is then empty.
const BaseArg = "ASTRO_BASE"

// BaseArgCheckDirective is the parser directive a Dockerfile built FROM
// BaseArg starts with. The argument has no default, on purpose (a default base
// without the project's packages would build and break at run time), and
// BuildKit warns about exactly that on every build (InvalidDefaultArgInFrom),
// argument given or not.
const BaseArgCheckDirective = "# check=skip=InvalidDefaultArgInFrom"

// BaseArgComment says, in a Dockerfile built FROM BaseArg, where the base
// comes from and how to build the file.
const BaseArgComment = "# Built on the install base the Astro CLI makes from pyproject.toml and\n" +
	"# uv.lock. Build it with astro package or astro deploy, or run\n" +
	"# astro package --context-dir <dir> to build it with docker yourself.\n"

// BaseArgFrom is the reference a final stage built on the install base names.
const BaseArgFrom = "${" + BaseArg + "}"

// ProjectCopy is the step that lays the project's files over the install, as
// the runtime's ONBUILD tag's last trigger does: owned by astro, into the
// image's working directory.
const ProjectCopy = "COPY --chown=astro:0 . ."

// UsesInstallBase reports whether a Dockerfile, data, builds its final stage
// FROM the install base (BaseArg, spelled ${ASTRO_BASE} or $ASTRO_BASE): a
// build of it has to build that base first and pass it, and a build of any
// other Dockerfile has no use for one.
func UsesInstallBase(data []byte) bool {
	stages := DockerfileStages(data)
	if len(stages) == 0 {
		return false
	}
	ref := stages[len(stages)-1].Ref
	return ref == BaseArgFrom || ref == "$"+BaseArg
}

// baseTagSuffix turns a runtime tag into its -base tag (3.1-5 to 3.1-5-base,
// 12.6.0-python-3.11 to 12.6.0-python-3.11-base), the same image without the
// ONBUILD triggers. The runtime publishes one beside every tag, in the same
// repository: scripts/check-runtime-base-tags.sh checks each one the CLI can
// pick.
const baseTagSuffix = "-base"

// BaseOf is the -base reference for a runtime image reference: the same
// repository, its tag with -base appended. A reference already on a -base tag
// is returned as it is. It does not check that ref names a runtime image; the
// caller has decided that.
func BaseOf(ref string) string {
	if isBaseTag(ref) {
		return ref
	}
	return ref + baseTagSuffix
}

// isBaseTag reports whether ref's tag is a runtime -base tag, the image
// without the ONBUILD triggers.
func isBaseTag(ref string) bool {
	return strings.HasSuffix(ref, baseTagSuffix)
}

// WithRegistry is ref, a runtime image reference on one of Astronomer's
// repositories (astrocrpublic.azurecr.io/runtime:3.3-8), pulled from registry
// instead: registry.example.com/astronomer/runtime:3.3-8, the repository's
// last path segment kept. An empty registry returns ref as it is.
func WithRegistry(ref, registry string) string {
	registry = strings.TrimRight(strings.TrimSpace(registry), "/")
	if registry == "" {
		return ref
	}
	image, tag := splitImageRef(ref)
	name := image[strings.LastIndex(image, "/")+1:]
	if tag == "" {
		return registry + "/" + name
	}
	return registry + "/" + name + ":" + tag
}

// RegistryOf is the registry a runtime image reference's repository is under,
// for a mirror of one of Astronomer's repositories (registry.example/astronomer
// from registry.example/astronomer/astro-runtime), and "" for Astronomer's own
// repositories, whose registry needs no record. ok is false for a repository
// not named as Astro Runtime's.
func RegistryOf(image string) (registry string, ok bool) {
	if IsPublishedRuntimeRepo(image) {
		return "", true
	}
	if !isRuntimeNamedRepo(image) {
		return "", false
	}
	return image[:strings.LastIndex(image, "/")], true
}

// netrcSecretID is the build secret the install step mounts at /root/.netrc,
// for a private index. manifest.RuntimeSecretID names the same id for the
// manifest's side; pkg/imagebuild's tests hold the two together.
const netrcSecretID = "netrc"

// The runtime's install scripts, from runtime 11.0.0 on: one installs
// packages.txt with apt, the other requirements.txt with uv under the
// runtime's constraints file.
const (
	installSystemPackages     = "/usr/local/bin/install-system-packages"
	installPythonDependencies = "/usr/local/bin/install-python-dependencies"
)

// InstallBaseDockerfile is the install over from, an Astro Runtime -base image
// (BaseOf), reading the lists from the build's own context: the steps the
// runtime's ONBUILD tag runs as triggers (astro-runtime's
// image/Dockerfile.onbuild), in the same order and as the same users, without
// the last one, which copies the project's files. The OS packages, then the
// Python dependencies under the runtime's constraints. It is the install base a
// project's own Dockerfile builds on (BaseArg), and the first part of a
// generated build (GeneratedDockerfile).
//
// mountNetrc mounts the netrc secret into the Python install, required=false,
// for a build that passes one. RUN --mount needs BuildKit (Docker) or Buildah
// 1.21 (Podman 3.2), so a build with no secret to mount keeps to a plain RUN,
// and the legacy builder and an older Podman build it end to end. The
// runtime's own 3.3-9 trigger mounts the secret unconditionally; a build that
// took that trigger needed BuildKit whether it had a secret or not.
//
// The install steps are written out rather than inherited because the trigger
// list has changed across runtimes (the netrc mount is in 3.3-9 but not
// 3.1-12 or 12.6.0), so a build inherited whichever list its base happened to
// carry; written here, every runtime installs the same way, and the netrc
// secret reaches the install on all of them.
//
// One step is the CLI's own: the packages a [tool.uv.sources] entry routes to
// an index install first, in the same RUN as the rest, with pip, from the
// lock's files alone (--no-index, --find-links on routed-links.html), without
// their dependencies. pip picks the file the image's Python and platform take
// and checks the lock's hash, and the runtime's install then finds them
// installed, so it never asks the image's index for those names. pip, with
// the runtime's constraints in /etc/pip.conf, is in every runtime image, and
// both pip 23 (runtime 10) and 26 (3.3) follow a local page's links to remote
// files and verify their #sha256 fragments. With nothing routed both files
// are empty and the step does nothing.
//
// Each install runs the runtime's own script, so the runtime's constraints file
// and its choice of installer (uv, or pip with ASTRO_RUNTIME_USE_PIP) still
// apply. Runtimes before 11.0.0 ship no scripts: their triggers run the apt
// install and `pip install -r` inline, so each step falls back to that command
// where the script is missing. The inline triggers also refused an
// apache-airflow pin, which the lists never carry. The test is POSIX, since a
// runtime's SHELL is not the same on every line.
func InstallBaseDockerfile(from string, mountNetrc bool) string {
	run := "RUN "
	if mountNetrc {
		run = "RUN --mount=type=secret,id=" + netrcSecretID + ",target=/root/.netrc,required=false "
	}
	return "FROM " + from + "\n" +
		"COPY " + PackagesName + " .\n" +
		"USER root\n" +
		"RUN if [ -x " + installSystemPackages + " ]; then " + installSystemPackages + "; " +
		"elif [ -s " + PackagesName + " ]; then apt-get update && cat " + PackagesName + " | tr '\\r\\n' '\\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi\n" +
		"COPY " + RequirementsName + " " + LockConstraintsName + " " + RoutedName + " " + RoutedLinksName + " ./\n" +
		run +
		"if [ -s " + RoutedName + " ]; then pip install --no-cache-dir --root-user-action=ignore --no-index --no-deps --find-links " + RoutedLinksName + " -r " + RoutedName + "; fi && " +
		"if [ -x " + installPythonDependencies + " ]; then " + installPythonDependencies + "; " +
		"else pip install --no-cache-dir --root-user-action=ignore -r " + RequirementsName + "; fi\n" +
		"USER astro\n"
}

// GeneratedDockerfile is a generated build's Dockerfile over from: the install
// (InstallBaseDockerfile), then the context's files owned by astro
// (ProjectCopy), as the runtime's ONBUILD tag's last trigger lays them down.
func GeneratedDockerfile(from string, mountNetrc bool) string {
	return InstallBaseDockerfile(from, mountNetrc) + ProjectCopy + "\n"
}
