package imagebuild

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// What a build installs comes from the project's pyproject.toml and uv.lock,
// read inside the build by one helper, install-project.py, which every build
// path runs: a generated build carries it in its context, and a project with a
// Dockerfile of its own carries it at InstallHelperPath, so `docker build .`
// needs nothing from the CLI. The helper writes the runtime's install inputs:
// requirements.txt (the dependencies, without Airflow, a [tool.uv.sources] git
// or URL source as a direct reference), packages.txt ([tool.astro] packages,
// plus git when a dependency is fetched with it), and, from uv.lock, a
// constraints file holding each package at the version the lock resolved,
// which requirements.txt names with -c. So the image installs the versions
// `uv sync` installs in standalone mode. A dependency an index of its own
// serves is installed from the exact file the lock recorded, by URL and hash,
// so PyPI is never asked for it; without a lock it is a --find-links page,
// which leaves PyPI a candidate too.
//
// It runs in a stage of its own (InstallDockerfile) over a copy of just its
// inputs, so an edit elsewhere in the project does not rerun it, and the
// install steps copy only what it wrote: an edit to pyproject.toml that leaves
// the lists as they were (a [tool.ruff] setting, the version) keeps the install
// layers cached.

//go:embed install-project.py
var installHelper []byte

// InstallHelperPath is where the install helper sits in a build's context,
// slash-separated: in a generated build's context, and in a project that
// builds its own Dockerfile (InstallDockerfile reads it from there).
const InstallHelperPath = ".astro/install-project.py"

// InstallHelper is the install helper's source, for a caller that writes it
// into a project or addresses an image by what builds it.
func InstallHelper() []byte {
	return bytes.Clone(installHelper)
}

// generatedInputsDir is where a generated build stages the install helper's
// inputs in its context: the helper, the project's pyproject.toml and, when it
// has them, its uv.lock and (for a request naming no project) the find-links
// file. The directory always exists, so the install stage copies it whole and
// needs no wildcard for the optional files, which Buildah has refused when
// they match nothing.
const generatedInputsDir = ".astro/install"

// installFindLinksName is the file a build request naming no project leaves
// the find-links pages it was given in, one per line, beside pyproject.toml in
// generatedInputsDir, for the helper to pass on as they are
// (install-project.py's FIND_LINKS). It is only that: the helper derives a
// project's pages from its pyproject.toml every time, as
// manifest.UV.IndexPagesFor does, so no build of a project writes it, a
// project's own copy is never read, and a plain `docker build .` of a
// project's own Dockerfile reads the same pages a CLI build does.
const installFindLinksName = "install-find-links.txt"

// Paths the install stage uses inside the build: where its inputs are copied
// to, and the directory it writes the install's lists to.
const (
	inputsDir = "/tmp/astro-project"
	listsDir  = "/tmp/astro-install"
)

// projectInputs is what a project's own Dockerfile copies into the install
// stage, from the project as its context: pyproject.toml, uv.lock when there is
// one, and the helper at InstallHelperPath. The wildcard is how uv.lock stays
// optional; Docker's BuildKit and its legacy builder both copy nothing for a
// wildcard that matches nothing, but Buildah has refused one, so a project
// built with Podman keeps a uv.lock, or takes `uv.lock*` out of the line.
var projectInputs = pyprojectName + " " + uvLockName + "* " + InstallHelperPath

// generatedInputs is what a generated build's install stage copies: the whole
// of generatedInputsDir.
const generatedInputs = generatedInputsDir + "/"

// InstallDockerfile is the Dockerfile that installs a project over from, an
// Astro Runtime -base image (baseOf): the install helper's stage, then the steps
// the runtime's ONBUILD tag runs as triggers (astro-runtime's
// image/Dockerfile.onbuild), in the same order and as the same users: the OS
// packages, then the Python dependencies under the runtime's constraints, then
// the context's files owned by astro. A project's own Dockerfile is this
// followed by its own steps, and it mounts the netrc secret for the Python
// install whether or not a build passes one (required=false), since a plain
// `docker build .` decides that, not the file. Its install stage copies
// projectInputs, which under Podman needs a uv.lock in the project.
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
	return installDockerfile(from, true, projectInputs)
}

// installDockerfile is InstallDockerfile, its install stage copying inputs (a
// COPY's sources), with the Python install's netrc mount left out when
// mountNetrc is false: a generated build mounts the secret only when the build
// passes it. RUN --mount needs BuildKit (Docker) or Buildah 1.21 (Podman 3.2),
// so a generated build with no secret to mount keeps to a plain RUN, and its
// install stage copies its inputs rather than mounting them: with no secret,
// the legacy builder and an older Podman build it end to end. The runtime's
// own 3.3-9 trigger mounts the secret unconditionally; a build that took that
// trigger needed BuildKit whether it had a secret or not.
func installDockerfile(from string, mountNetrc bool, inputs string) string {
	netrc := ""
	if mountNetrc {
		netrc = "--mount=type=secret,id=" + manifest.RuntimeSecretID + ",target=/root/.netrc,required=false "
	}
	return "FROM " + from + " AS astro-install\n" +
		// Root, because the context keeps its files' modes, and a project's may
		// be readable by its owner alone.
		"USER root\n" +
		// Just the inputs, so an edit elsewhere in the project does not rerun
		// the stage.
		"COPY " + inputs + " " + inputsDir + "/\n" +
		"RUN python " + inputsDir + "/" + filepath.Base(InstallHelperPath) + " --project " + inputsDir + " --out " + listsDir + "\n" +
		"FROM " + from + "\n" +
		"COPY --from=astro-install " + listsDir + "/" + packagesName + " .\n" +
		"USER root\n" +
		"RUN if [ -x " + installSystemPackages + " ]; then " + installSystemPackages + "; " +
		"elif [ -s " + packagesName + " ]; then apt-get update && cat " + packagesName + " | tr '\\r\\n' '\\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi\n" +
		"COPY --from=astro-install " + listsDir + "/" + requirementsName + " " + listsDir + "/" + lockConstraintsName + " ./\n" +
		"RUN " + netrc +
		"if [ -x " + installPythonDependencies + " ]; then " + installPythonDependencies + "; " +
		"else pip install --no-cache-dir --root-user-action=ignore -r " + requirementsName + "; fi\n" +
		"USER astro\n" +
		"COPY --chown=astro:0 . .\n"
}

// GeneratedDockerfile is the Dockerfile a generated build over the runtime
// base baseImage runs (Request.BaseImage, before baseOf), with the netrc
// secret mounted into the Python install when mountNetrc is set, as Build
// sets it when the request carries that secret. It is for a caller that
// addresses the image by what builds it (`astro package astro`'s content tag
// hashes it, so a change to the recipe is a change to the tag).
func GeneratedDockerfile(baseImage string, mountNetrc bool) string {
	return installDockerfile(baseOf(baseImage), mountNetrc, generatedInputs)
}

// The runtime's install scripts, from runtime 11.0.0 on: one installs
// packages.txt with apt, the other requirements.txt with uv under the
// runtime's constraints file.
const (
	installSystemPackages     = "/usr/local/bin/install-system-packages"
	installPythonDependencies = "/usr/local/bin/install-python-dependencies"
)

// The manifest files the helper reads.
const (
	pyprojectName = "pyproject.toml"
	uvLockName    = "uv.lock"
)

// manifestProject is the project a generated build installs from:
// req.ManifestDir, else the ProjectFiles the image carries (a caller that
// predates ManifestDir), else "" for a request that names none.
func manifestProject(req Request) string {
	if req.ManifestDir != "" {
		return req.ManifestDir
	}
	return req.ProjectFiles
}

// projectManifest reads manifestProject's pyproject.toml, or returns nil for a
// request that names no project or a project without one.
func projectManifest(req Request) ([]byte, error) {
	project := manifestProject(req)
	if project == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(project, pyprojectName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the project's %s: %w", pyprojectName, err)
	}
	return data, nil
}

// installInputs are the files a generated build's install stage reads, by
// their name in generatedInputsDir: the helper, and the project's
// pyproject.toml and uv.lock as they are (manifestProject), so the copies are
// always the project's own. A request naming no project, or whose
// project has no pyproject.toml, gets a pyproject.toml stating its Dependencies
// and Packages, and its FindLinks pages as they were given.
func installInputs(req Request) (map[string][]byte, error) {
	inputs := map[string][]byte{filepath.Base(InstallHelperPath): installHelper}
	data, err := projectManifest(req)
	if err != nil {
		return nil, err
	}
	if data != nil {
		inputs[pyprojectName] = data
		lock, err := os.ReadFile(filepath.Join(manifestProject(req), uvLockName))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("reading the project's %s: %w", uvLockName, err)
		default:
			inputs[uvLockName] = lock
		}
		return inputs, nil
	}
	if inputs[pyprojectName], err = requestPyproject(req); err != nil {
		return nil, err
	}
	if len(req.FindLinks) > 0 {
		inputs[installFindLinksName] = []byte(strings.Join(req.FindLinks, "\n") + "\n")
	}
	return inputs, nil
}

// installsNothing reports whether a generated build of req would install
// nothing over its base: no Python dependency besides Airflow and no OS
// package, by the same manifest the install reads, the project's own
// pyproject.toml when it has one. A pyproject.toml that cannot be read or
// parsed counts as installing something, so the build runs and says why.
func (r *Request) installsNothing() bool {
	data, err := projectManifest(*r)
	if err != nil {
		return false
	}
	if data == nil {
		return len(runtimeDeps(r.Dependencies)) == 0 && len(r.Packages) == 0
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return false
	}
	return len(runtimeDeps(m.Requirements())) == 0 && len(m.Astro.Packages) == 0
}

// writeInstallInputs writes installInputs into generatedInputsDir in the
// generated build's context, replacing whatever a project carried there, and
// removes a find-links file the project carried where only an older build
// wrote one: the helper reads neither, and neither belongs in the image.
func writeInstallInputs(contextDir string, req Request) error {
	inputs, err := installInputs(req)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(contextDir, ".astro", installFindLinksName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing the project's %s from the build context: %w", installFindLinksName, err)
	}
	dir := filepath.Join(contextDir, filepath.FromSlash(generatedInputsDir))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clearing %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, contextDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	for name, data := range inputs {
		if err := os.WriteFile(filepath.Join(dir, name), data, filePermRW); err != nil {
			return fmt.Errorf("writing %s into the build context: %w", name, err)
		}
	}
	return nil
}

// InstallInputsDigest is a digest of the files a generated build of req
// installs from (installInputs: the helper, pyproject.toml, uv.lock), for a
// caller that addresses the image by what builds it. It covers everything the
// install reads from the manifest, so the caller hashes no list of its own.
func InstallInputsDigest(req Request) (string, error) {
	inputs, err := installInputs(req)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	slices.Sort(names)
	h := sha256.New()
	for _, name := range names {
		fmt.Fprintf(h, "%s\x00%x\x00", name, sha256.Sum256(inputs[name]))
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// requestPyproject is a pyproject.toml stating a request's Dependencies and
// Packages, for the helper to read when the request names no project.
func requestPyproject(req Request) ([]byte, error) {
	type project struct {
		Name         string   `toml:"name"`
		Version      string   `toml:"version"`
		Dependencies []string `toml:"dependencies"`
	}
	type astro struct {
		Packages []string `toml:"packages"`
	}
	doc := struct {
		Project project `toml:"project"`
		Tool    struct {
			Astro astro `toml:"astro"`
		} `toml:"tool"`
	}{Project: project{Name: "astro-generated", Version: "0", Dependencies: nonNil(req.Dependencies)}}
	doc.Tool.Astro.Packages = nonNil(req.Packages)
	data, err := toml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("writing the build's %s: %w", pyprojectName, err)
	}
	return data, nil
}

// nonNil is s, or an empty list for nil, which TOML writes as [] rather than
// leaving the key out.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
