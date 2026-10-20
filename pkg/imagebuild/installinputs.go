package imagebuild

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
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

// The helper and the recipe live in pkg/airflowrt, which pkg/scaffold also
// writes into a project whose Dockerfile it moves onto a -base tag. These are
// the names this package has always had for them.

// installHelper is the install helper's source (airflowrt.InstallHelper).
var installHelper = airflowrt.InstallHelper()

// InstallHelperPath is where the install helper sits in a build's context
// (airflowrt.InstallHelperPath).
const InstallHelperPath = airflowrt.InstallHelperPath

// InstallHelper is the install helper's source, for a caller that writes it
// into a project or addresses an image by what builds it.
func InstallHelper() []byte {
	return airflowrt.InstallHelper()
}

// installFindLinksPath is where a build request naming no project leaves the
// find-links pages it was given (airflowrt.InstallFindLinksPath).
const installFindLinksPath = airflowrt.InstallFindLinksPath

// InstallDockerfile is the Dockerfile that installs a project over from, an
// Astro Runtime -base image (airflowrt.InstallDockerfile): a project's own
// Dockerfile is this followed by its own steps.
func InstallDockerfile(from string) string {
	return airflowrt.InstallDockerfile(from)
}

// installDockerfile is InstallDockerfile, with the Python install's netrc
// mount left out when mountNetrc is false: a generated build mounts the secret
// only when the build passes it (airflowrt.InstallOptions.NoNetrc).
func installDockerfile(from string, mountNetrc bool) string {
	return airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{NoNetrc: !mountNetrc})
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

// installInputs are the files a generated build's install stage reads, by their
// slash-separated path in the context: the helper, and the project's
// pyproject.toml and uv.lock as they are (manifestProject), so the context's
// copies are always the project's own. A request naming no project, or whose
// project has no pyproject.toml, gets a pyproject.toml stating its Dependencies
// and Packages, and its FindLinks pages as they were given.
func installInputs(req Request) (map[string][]byte, error) {
	inputs := map[string][]byte{InstallHelperPath: installHelper}
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
		inputs[installFindLinksPath] = []byte(strings.Join(req.FindLinks, "\n") + "\n")
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

// writeInstallInputs writes installInputs into the generated build's context,
// replacing a copy of the project's own there (copyProjectFiles carries its
// pyproject.toml, its uv.lock, and any install helper it has): removed first,
// since a file the project keeps read-only arrives read-only.
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
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("replacing %s in the build context: %w", rel, err)
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
