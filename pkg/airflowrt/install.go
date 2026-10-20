package airflowrt

import (
	"bytes"
	_ "embed"
	"path"
	"strings"
)

// How an image installs a project over an Astro Runtime base: the recipe every
// build path runs, a generated build's and a project's own Dockerfile alike. It
// lives here, beside the rest of what this package knows about runtime images,
// because both of its writers need it: pkg/imagebuild writes it for a generated
// build, and pkg/scaffold writes it into a project whose Dockerfile it rewrites
// onto a -base tag.
//
// What a build installs comes from the project's pyproject.toml and uv.lock,
// read inside the build by one helper, install-project.py, at InstallHelperPath
// in the build's context, so `docker build .` needs nothing from the CLI. The
// helper writes the runtime's install inputs: requirements.txt (the
// dependencies, without Airflow, a [tool.uv.sources] git or URL source as a
// direct reference, an index source as a --find-links line), packages.txt
// ([tool.astro] packages, plus git when a dependency is fetched with it), and,
// from uv.lock, a constraints file holding each package at the version the lock
// resolved, which requirements.txt names with -c. So the image installs the
// versions `uv sync` installs in standalone mode.
//
// It runs in a stage of its own (InstallDockerfile), and the install steps copy
// only what it wrote. The stage reads the whole project and reruns on any edit,
// in well under a second; the install layers are keyed on the files it wrote,
// so an edit to pyproject.toml that leaves the lists as they were (a [tool.ruff]
// setting, the version) keeps them cached.

//go:embed install-project.py
var installHelper []byte

// InstallHelperPath is where the install helper sits in a build's context,
// slash-separated: in a generated build's context, and in a project that
// builds its own Dockerfile (InstallDockerfile reads it from there).
const InstallHelperPath = ".astro/install-project.py"

// InstallHelper is the install helper's source, for a caller that writes it
// into a build's context or a project, or addresses an image by what builds it.
func InstallHelper() []byte {
	return bytes.Clone(installHelper)
}

// The install inputs the helper writes (install-project.py's REQUIREMENTS,
// PACKAGES and LOCK_CONSTRAINTS). The first two are the names the runtime's
// install scripts read from their working directory; the third is the file of
// uv.lock's versions requirements.txt names with -c.
const (
	RequirementsName    = "requirements.txt"
	PackagesName        = "packages.txt"
	LockConstraintsName = "uv-lock-constraints.txt"
)

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

// The manifest files the helper reads.
const (
	pyprojectName = "pyproject.toml"
	uvLockName    = "uv.lock"
)

// Paths the install stage uses inside the build: where its inputs are copied
// to, and the directory it writes the install's lists to.
const (
	inputsDir = "/tmp/astro-project"
	listsDir  = "/tmp/astro-install"
)

// The runtime's install scripts, from runtime 11.0.0 on: one installs
// packages.txt with apt, the other requirements.txt with uv under the
// runtime's constraints file.
const (
	installSystemPackages     = "/usr/local/bin/install-system-packages"
	installPythonDependencies = "/usr/local/bin/install-python-dependencies"
)

// ProjectInputs is what a project's own Dockerfile copies into the install
// stage, from the project as its context: pyproject.toml, uv.lock when there is
// one, and the helper at InstallHelperPath. The wildcard is how uv.lock stays
// optional; Docker's BuildKit and its legacy builder both copy nothing for a
// wildcard that matches nothing, but Buildah has refused one, so a project
// built with Podman keeps a uv.lock, or takes uv.lock* out of the line.
const ProjectInputs = pyprojectName + " " + uvLockName + "* " + InstallHelperPath

// InstallStage is the name InstallDockerfile gives the helper's stage.
const InstallStage = "astro-install"

// InstallDockerfile is the Dockerfile that installs a project over from, an
// Astro Runtime -base image (BaseOf): the install helper's stage, then the
// steps the runtime's ONBUILD tag runs as triggers (astro-runtime's
// image/Dockerfile.onbuild), in the same order and as the same users: the OS
// packages, then the Python dependencies under the runtime's constraints, then
// the context's files owned by astro. A project's own Dockerfile is this
// followed by its own steps, and it mounts the netrc secret for the Python
// install whether or not a build passes one (required=false), since a plain
// `docker build .` decides that, not the file.
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
// Each install runs the runtime's own script, so the runtime's constraints file
// and its choice of installer (uv, or pip with ASTRO_RUNTIME_USE_PIP) still
// apply. Runtimes before 11.0.0 ship no scripts: their triggers run the apt
// install and `pip install -r` inline, so each step falls back to that command
// where the script is missing. The inline triggers also refused an
// apache-airflow pin, which the helper has already removed. The test is POSIX,
// since a runtime's SHELL is not the same on every line.
func InstallDockerfile(from string) string {
	return InstallDockerfileWith(from, InstallOptions{})
}

// InstallOptions are the two ways a caller's install differs from a project's
// own Dockerfile.
type InstallOptions struct {
	// Alias names the final stage, the project's own (FROM … AS alias), for a
	// Dockerfile whose later lines refer to it by name.
	Alias string
	// Inputs are the install stage's COPY sources, from the build's context.
	// Empty is a project's own: pyproject.toml, uv.lock when there is one,
	// and the helper at InstallHelperPath (ProjectInputs).
	Inputs string
	// NoNetrc leaves the Python install's netrc mount out: a generated build
	// mounts the secret only when the build passes it. RUN --mount needs
	// BuildKit (Docker) or Buildah 1.21 (Podman 3.2), so a generated build
	// with no secret to mount keeps to a plain RUN, and the legacy builder and
	// an older Podman build it end to end. The runtime's own 3.3-9 trigger
	// mounts the secret unconditionally; a build that took that trigger needed
	// BuildKit whether it had a secret or not.
	NoNetrc bool
}

// InstallDockerfileWith is InstallDockerfile with opts.
func InstallDockerfileWith(from string, opts InstallOptions) string {
	netrc := "--mount=type=secret,id=" + netrcSecretID + ",target=/root/.netrc,required=false "
	if opts.NoNetrc {
		netrc = ""
	}
	inputs := opts.Inputs
	if inputs == "" {
		inputs = ProjectInputs
	}
	final := "FROM " + from + "\n"
	if opts.Alias != "" {
		final = "FROM " + from + " AS " + opts.Alias + "\n"
	}
	return "FROM " + from + " AS " + InstallStage + "\n" +
		// Root, because the context keeps its files' modes, and a project's may
		// be readable by its owner alone.
		"USER root\n" +
		// Just the inputs, so an edit elsewhere in the project does not rerun
		// the stage.
		"COPY " + inputs + " " + inputsDir + "/\n" +
		"RUN python " + inputsDir + "/" + path.Base(InstallHelperPath) + " --project " + inputsDir + " --out " + listsDir + "\n" +
		final +
		"COPY --from=" + InstallStage + " " + listsDir + "/" + PackagesName + " .\n" +
		"USER root\n" +
		"RUN if [ -x " + installSystemPackages + " ]; then " + installSystemPackages + "; " +
		"elif [ -s " + PackagesName + " ]; then apt-get update && cat " + PackagesName + " | tr '\\r\\n' '\\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi\n" +
		"COPY --from=" + InstallStage + " " + listsDir + "/" + RequirementsName + " " + listsDir + "/" + LockConstraintsName + " ./\n" +
		"RUN " + netrc +
		"if [ -x " + installPythonDependencies + " ]; then " + installPythonDependencies + "; " +
		"else pip install --no-cache-dir --root-user-action=ignore -r " + RequirementsName + "; fi\n" +
		"USER astro\n" +
		"COPY --chown=astro:0 . .\n"
}
