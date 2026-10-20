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

// installFindLinksPath is where a build request naming no project leaves the
// find-links pages it was given, one per line, for the helper to pass on as
// they are (install-project.py's FIND_LINKS).
const installFindLinksPath = ".astro/install-find-links.txt"

// Paths the install stage uses inside the build: where its inputs are copied
// to, and the directory it writes the install's lists to.
const (
	inputsDir = "/tmp/astro-project"
	listsDir  = "/tmp/astro-install"
)

// InstallDockerfile is the Dockerfile that installs a project over from, an
// Astro Runtime -base image (baseOf): the install helper's stage, then the steps
// the runtime's ONBUILD tag runs as triggers (astro-runtime's
// image/Dockerfile.onbuild), in the same order and as the same users: the OS
// packages, then the Python dependencies under the runtime's constraints, then
// the context's files owned by astro. A project's own Dockerfile is this
// followed by its own steps, and it mounts the netrc secret for the Python
// install whether or not a build passes one (required=false), since a plain
// `docker build .` decides that, not the file.
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
	return installDockerfile(from, true)
}

// installDockerfile is InstallDockerfile, with the Python install's netrc
// mount left out when mountNetrc is false: a generated build mounts the secret
// only when the build passes it. RUN --mount needs BuildKit (Docker) or Buildah
// 1.21 (Podman 3.2), so a generated build with no secret to mount keeps to a
// plain RUN, and its install stage copies its inputs rather than mounting
// them: with no secret, the legacy builder and an older Podman build it end to
// end. The runtime's own 3.3-9 trigger mounts the secret unconditionally; a
// build that took that trigger needed BuildKit whether it had a secret or not.
func installDockerfile(from string, mountNetrc bool) string {
	netrc := ""
	if mountNetrc {
		netrc = "--mount=type=secret,id=" + manifest.RuntimeSecretID + ",target=/root/.netrc,required=false "
	}
	return "FROM " + from + " AS astro-install\n" +
		// Root, because the context keeps its files' modes, and a project's may
		// be readable by its owner alone.
		"USER root\n" +
		// Just the inputs, so an edit elsewhere in the project does not rerun
		// the stage. uv.lock and the find-links file are optional: a wildcard
		// that matches nothing copies nothing.
		"COPY " + pyprojectName + " " + uvLockName + "* " + installFindLinksPath + "* " + InstallHelperPath + " " + inputsDir + "/\n" +
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
	return installDockerfile(baseOf(baseImage), mountNetrc)
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

// installInputs are the files a generated build's install stage reads, by their
// slash-separated path in the context: the helper, and the project's
// pyproject.toml and uv.lock as they are. The project is req.ManifestDir, else
// the ProjectFiles the image carries (a caller that predates ManifestDir), so
// the context's copies are always the project's own. A request with neither, or
// whose project has no pyproject.toml, gets a pyproject.toml stating its
// Dependencies and Packages, and its FindLinks pages as they were given.
func installInputs(req Request) (map[string][]byte, error) {
	inputs := map[string][]byte{InstallHelperPath: installHelper}
	project := req.ManifestDir
	if project == "" {
		project = req.ProjectFiles
	}
	if project != "" {
		data, err := os.ReadFile(filepath.Join(project, pyprojectName))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("reading the project's %s: %w", pyprojectName, err)
		default:
			inputs[pyprojectName] = data
			lock, err := os.ReadFile(filepath.Join(project, uvLockName))
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				return nil, fmt.Errorf("reading the project's %s: %w", uvLockName, err)
			default:
				inputs[uvLockName] = lock
			}
			return inputs, nil
		}
	}
	inputs[pyprojectName] = requestPyproject(req)
	if len(req.FindLinks) > 0 {
		inputs[installFindLinksPath] = []byte(strings.Join(req.FindLinks, "\n") + "\n")
	}
	return inputs, nil
}

// writeInstallInputs writes installInputs into the generated build's context.
func writeInstallInputs(contextDir string, req Request) error {
	inputs, err := installInputs(req)
	if err != nil {
		return err
	}
	for rel, data := range inputs {
		dst := filepath.Join(contextDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), contextDirPerm); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, data, filePermRW); err != nil {
			return fmt.Errorf("writing %s into the build context: %w", rel, err)
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
func requestPyproject(req Request) []byte {
	return []byte("[project]\nname = \"astro-generated\"\nversion = \"0\"\n" +
		"dependencies = " + tomlList(req.Dependencies) + "\n" +
		"\n[tool.astro]\npackages = " + tomlList(req.Packages) + "\n")
}

// tomlString quotes s as a TOML basic string: a backslash, a quote and any
// control character escaped, everything else as it is.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func tomlList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = tomlString(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
