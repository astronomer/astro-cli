package airflowrt

import (
	"regexp"
	"strings"
)

// How an image installs a project over an Astro Runtime base: the recipe every
// build path runs, a generated build's and a project's own Dockerfile alike. It
// lives here, beside the rest of what this package knows about runtime images,
// because both of its writers need it: pkg/imagebuild writes it for a generated
// build, and pkg/scaffold writes it into a project whose Dockerfile it rewrites
// onto a -base tag.
//
// What a build installs is decided before the build, by the CLI, which
// resolves the project's pyproject.toml and uv.lock with the host's uv and
// writes the lists: requirements.txt, uv-lock-constraints.txt (which
// requirements.txt names with -c) and packages.txt, and, for the packages a
// [tool.uv.sources] entry routes to an index, routed-requirements.txt and
// routed-links.html. The build resolves nothing; it installs the lists. A generated build finds them in its own
// context. A project's own Dockerfile finds them in a second, named build
// context, InstallContext, which the CLI passes beside the project
// (`--build-context astro-install=<dir>`), so nothing is written into the
// project.

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

// InstallContext is the named build context a project's own Dockerfile copies
// the install lists from (COPY --from=astro-install). A build passes it as
// `--build-context astro-install=<dir>`, a directory holding the lists;
// a build that passes none fails at that COPY.
const InstallContext = "astro-install"

// readsInstallContextRe finds an instruction that reads InstallContext:
// --from=astro-install, as InstallDockerfile writes it, quoted or not.
var readsInstallContextRe = regexp.MustCompile(`(?i)--from=["']?` + InstallContext + `\b`)

// ReadsInstallContext reports whether a Dockerfile, data, reads the install
// lists from InstallContext, as InstallDockerfile's steps do: a build of it
// has to pass that context, and a build of any other Dockerfile has no use
// for it. Comments are not read.
func ReadsInstallContext(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") && readsInstallContextRe.MatchString(t) {
			return true
		}
	}
	return false
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

// InstallDockerfile is a project's own Dockerfile's install over from, an
// Astro Runtime -base image (BaseOf): the steps the runtime's ONBUILD tag runs
// as triggers (astro-runtime's image/Dockerfile.onbuild), in the same order
// and as the same users, reading the lists from InstallContext: the OS
// packages, then the Python dependencies under the runtime's constraints, then
// the context's files owned by astro. A project's own Dockerfile is this
// followed by its own steps. It mounts the netrc secret for the Python install
// whether or not a build passes one (required=false), since the build decides
// that, not the file.
//
// from is everything a FROM line takes after the keyword and before any AS, so
// a stage flag (--platform=linux/amd64) comes with the reference.
//
// The install steps are written out rather than inherited for two reasons. The
// trigger list has changed across runtimes (the netrc mount is in 3.3-9 but not
// 3.1-12 or 12.6.0), so a build inherited whichever list its base happened to
// carry; written here, every runtime installs the same way, and the netrc
// secret reaches the install on all of them. And a Dockerfile that states its
// install is one `docker build` runs as written, with nothing happening at FROM
// that the file does not show.
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
func InstallDockerfile(from string) string {
	return InstallDockerfileWith(from, InstallOptions{ListsFrom: InstallContext})
}

// InstallOptions are the ways a caller's install differs from the plainest
// one, a generated build's with no secret.
type InstallOptions struct {
	// Alias names the final stage, the project's own (FROM … AS alias), for a
	// Dockerfile whose later lines refer to it by name.
	Alias string
	// ListsFrom is the build context or stage the lists are copied from
	// (COPY --from=…). Empty copies them from the build's own context, as a
	// generated build does; a project's own Dockerfile reads them from
	// InstallContext.
	ListsFrom string
	// MountNetrc mounts the netrc secret into the Python install, required=false.
	// RUN --mount needs BuildKit (Docker) or Buildah 1.21 (Podman 3.2), so a
	// generated build with no secret to mount keeps to a plain RUN, and the
	// legacy builder and an older Podman build it end to end. The runtime's
	// own 3.3-9 trigger mounts the secret unconditionally; a build that took
	// that trigger needed BuildKit whether it had a secret or not. A project's
	// own Dockerfile always mounts it.
	MountNetrc bool
}

// InstallDockerfileWith is the install over from, with opts.
func InstallDockerfileWith(from string, opts InstallOptions) string {
	run := "RUN "
	if opts.MountNetrc || opts.ListsFrom != "" {
		run = "RUN --mount=type=secret,id=" + netrcSecretID + ",target=/root/.netrc,required=false "
	}
	copyFrom := "COPY "
	if opts.ListsFrom != "" {
		copyFrom = "COPY --from=" + opts.ListsFrom + " "
	}
	final := "FROM " + from + "\n"
	if opts.Alias != "" {
		final = "FROM " + from + " AS " + opts.Alias + "\n"
	}
	return final +
		copyFrom + PackagesName + " .\n" +
		"USER root\n" +
		"RUN if [ -x " + installSystemPackages + " ]; then " + installSystemPackages + "; " +
		"elif [ -s " + PackagesName + " ]; then apt-get update && cat " + PackagesName + " | tr '\\r\\n' '\\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi\n" +
		copyFrom + RequirementsName + " " + LockConstraintsName + " " + RoutedName + " " + RoutedLinksName + " ./\n" +
		run +
		"if [ -s " + RoutedName + " ]; then pip install --no-cache-dir --root-user-action=ignore --no-index --no-deps --find-links " + RoutedLinksName + " -r " + RoutedName + "; fi && " +
		"if [ -x " + installPythonDependencies + " ]; then " + installPythonDependencies + "; " +
		"else pip install --no-cache-dir --root-user-action=ignore -r " + RequirementsName + "; fi\n" +
		"USER astro\n" +
		"COPY --chown=astro:0 . .\n"
}
