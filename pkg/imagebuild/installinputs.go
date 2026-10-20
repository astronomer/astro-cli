package imagebuild

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// What a build installs comes from the project's pyproject.toml and uv.lock,
// read inside the build by one helper, install-project.py, which every build
// path runs: a generated build carries it in its context, and a project with a
// Dockerfile of its own carries it at InstallHelperPath, so `docker build .`
// needs nothing from the CLI. The helper writes the runtime's install inputs:
// requirements.txt (the dependencies, without Airflow, a [tool.uv.sources] git
// or URL source as a direct reference, an index source as a --find-links
// line), packages.txt ([tool.astro] packages, plus git when a dependency is
// fetched with it), and, from uv.lock, a constraints file holding each package
// at the version the lock resolved, which requirements.txt names with -c. So
// the image installs the versions `uv sync` installs in standalone mode.
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
// into a project or addresses an image by what builds it.
func InstallHelper() []byte {
	return bytes.Clone(installHelper)
}

// Paths the install stage uses inside the build: the context, mounted read-only
// for the helper to read, and the directory it writes the install inputs to.
const (
	contextMount = "/tmp/astro-project"
	listsDir     = "/tmp/astro-install"
)

// InstallDockerfile is the Dockerfile that installs a project over from, an
// Astro Runtime -base image (baseOf): the install helper's stage, then the steps
// the runtime's ONBUILD tag runs as triggers (astro-runtime's
// image/Dockerfile.onbuild), in the same order and as the same users: the OS
// packages, then the Python dependencies under the runtime's constraints, then
// the context's files owned by astro. A generated build runs it as it is; a
// project's own Dockerfile is this followed by its own steps.
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
	return "FROM " + from + " AS astro-install\n" +
		// Root, because the context keeps its files' modes and owners, and a
		// project's may be readable by its owner alone.
		"USER root\n" +
		"RUN --mount=type=bind,target=" + contextMount + " python " + contextMount + "/" + InstallHelperPath +
		" --project " + contextMount + " --out " + listsDir + "\n" +
		"FROM " + from + "\n" +
		"COPY --from=astro-install " + listsDir + "/" + packagesName + " .\n" +
		"USER root\n" +
		"RUN if [ -x " + installSystemPackages + " ]; then " + installSystemPackages + "; " +
		"elif [ -s " + packagesName + " ]; then apt-get update && cat " + packagesName + " | tr '\\r\\n' '\\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi\n" +
		"COPY --from=astro-install " + listsDir + "/" + requirementsName + " " + listsDir + "/" + lockConstraintsName + " ./\n" +
		"RUN --mount=type=secret,id=" + manifest.RuntimeSecretID + ",target=/root/.netrc,required=false " +
		"if [ -x " + installPythonDependencies + " ]; then " + installPythonDependencies + "; " +
		"else pip install --no-cache-dir --root-user-action=ignore -r " + requirementsName + "; fi\n" +
		"USER astro\n" +
		"COPY --chown=astro:0 . .\n"
}

// GeneratedDockerfile is the Dockerfile a generated build over the runtime
// base baseImage runs (Request.BaseImage, before baseOf), for a caller that
// addresses the image by what builds it: `astro package astro`'s content tag
// hashes it, so a change to the recipe is a change to the tag.
func GeneratedDockerfile(baseImage string) string {
	return InstallDockerfile(baseOf(baseImage))
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

// writeInstallInputs writes what the install stage reads into the generated
// build's context: the helper, and the project's pyproject.toml and uv.lock
// (req.ManifestDir), or, for a request that names no project or one without a
// pyproject.toml, a pyproject.toml stating its Dependencies, FindLinks and
// Packages.
func writeInstallInputs(contextDir string, req Request) error {
	var data []byte
	if req.ManifestDir != "" {
		var err error
		data, err = os.ReadFile(filepath.Join(req.ManifestDir, pyprojectName))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("reading the project's %s: %w", pyprojectName, err)
		}
	}
	if data != nil {
		if err := os.WriteFile(filepath.Join(contextDir, pyprojectName), data, filePermRW); err != nil { //nolint:gosec // G703: contextDir is the WorkDir the caller owns, joined with a constant name
			return fmt.Errorf("writing %s into the build context: %w", pyprojectName, err)
		}
		switch lock, err := os.ReadFile(filepath.Join(req.ManifestDir, uvLockName)); {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return fmt.Errorf("reading the project's %s: %w", uvLockName, err)
		default:
			if err := os.WriteFile(filepath.Join(contextDir, uvLockName), lock, filePermRW); err != nil { //nolint:gosec // G703: as above
				return fmt.Errorf("writing %s into the build context: %w", uvLockName, err)
			}
		}
	} else if err := os.WriteFile(filepath.Join(contextDir, pyprojectName), requestPyproject(req), filePermRW); err != nil {
		return fmt.Errorf("writing %s into the build context: %w", pyprojectName, err)
	}
	helper := filepath.Join(contextDir, filepath.FromSlash(InstallHelperPath))
	if err := os.MkdirAll(filepath.Dir(helper), contextDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(helper), err)
	}
	if err := os.WriteFile(helper, installHelper, filePermRW); err != nil {
		return fmt.Errorf("writing the install helper: %w", err)
	}
	return nil
}

// requestPyproject is a pyproject.toml stating a request's lists, for the
// helper to read when the request names no project: Dependencies as they are,
// Packages as [tool.astro] packages, and each FindLinks page (an index's page
// for one package, <index>/<package>/) as the index and the source routing that
// package to it, which the helper turns back into the same page.
func requestPyproject(req Request) []byte {
	var b strings.Builder
	b.WriteString("[project]\nname = \"astro-generated\"\nversion = \"0\"\n")
	b.WriteString("dependencies = " + tomlList(req.Dependencies) + "\n")
	b.WriteString("\n[tool.astro]\npackages = " + tomlList(req.Packages) + "\n")
	var sources []string
	for i, page := range req.FindLinks {
		index, pkg := path.Split(strings.TrimSuffix(page, "/"))
		if pkg == "" || index == "" {
			continue
		}
		name := fmt.Sprintf("find-links-%d", i)
		b.WriteString("\n[[tool.uv.index]]\nname = " + tomlString(name) + "\nurl = " + tomlString(strings.TrimSuffix(index, "/")) + "\n")
		sources = append(sources, tomlString(pkg)+" = { index = "+tomlString(name)+" }")
	}
	if len(sources) > 0 {
		b.WriteString("\n[tool.uv.sources]\n" + strings.Join(sources, "\n") + "\n")
	}
	return []byte(b.String())
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
