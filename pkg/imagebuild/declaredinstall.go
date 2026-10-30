package imagebuild

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// A project's own Dockerfile that astro init moved onto its runtime's -base
// tag installs the same lists a generated build does, resolved by the CLI
// before the build (airflowrt.InstallDockerfile). It reads them from a second,
// named build context, airflowrt.InstallContext, rather than from the
// project: Build writes the lists into a directory of WorkDir and passes it
// as `--build-context astro-install=<dir>`, so nothing is written into the
// project. BuildKit (Docker Engine 23, or Buildx 0.8, and later) and Buildah
// 1.26.4 or later (Podman 4.2 or later) take named contexts; Docker's legacy
// builder does not. A Dockerfile that does not read the context is built
// without one, as before.

// installContextDir is the sub-directory of a Request's WorkDir that holds
// the lists a declared Dockerfile reads from the named context, and of a
// directory WriteDeclaredContext writes.
const installContextDir = airflowrt.InstallContext

// ReadsInstallContext reports whether the request builds a declared
// Dockerfile that reads the install lists from the named context
// (airflowrt.ReadsInstallContext): only then does the build need them. A
// Dockerfile that cannot be read reads nothing; Build reports it.
func (r *Request) ReadsInstallContext() bool {
	if !r.FromDeclaredDockerfile() {
		return false
	}
	data, err := os.ReadFile(r.Dockerfile)
	return err == nil && airflowrt.ReadsInstallContext(data)
}

// installContextArgs writes the lists a declared Dockerfile reads from the
// named context into WorkDir, started empty, and returns the --build-context
// values that pass it; none for a Dockerfile that does not read it.
func installContextArgs(req Request) ([]string, error) {
	if !req.ReadsInstallContext() {
		return nil, nil
	}
	if req.WorkDir == "" {
		return nil, errors.New("the project's Dockerfile reads the install lists from the " + airflowrt.InstallContext + " build context, and the build has no directory to write them in")
	}
	dir := filepath.Join(req.WorkDir, installContextDir)
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("clearing %s: %w", dir, err)
	}
	if err := writeLists(dir, req.Lists()); err != nil {
		return nil, err
	}
	return []string{airflowrt.InstallContext + "=" + dir}, nil
}

// writeLists writes the lists' files into dir, creating it.
func writeLists(dir string, lists InstallLists) error {
	if err := os.MkdirAll(dir, contextDirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	for name, data := range lists.Files() {
		if err := os.WriteFile(filepath.Join(dir, name), data, filePermRW); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}

// The layout WriteDeclaredContext writes under its directory.
const (
	// DeclaredContextDir holds the project as the build reads it.
	DeclaredContextDir = "context"
	// DeclaredInstallDir holds the install lists, the named context.
	DeclaredInstallDir = installContextDir
	// DeclaredDockerfileName is the project's Dockerfile, outside the
	// context, as a build of the project reads it whatever its ignore file
	// says.
	DeclaredDockerfileName = "Dockerfile"
)

// WriteDeclaredContext writes what a build of the project's own Dockerfile
// reads into dir, which must not exist or be empty, for a `docker build` run
// with nothing from the CLI: the Dockerfile (DeclaredDockerfileName), the
// project as the build's context (DeclaredContextDir), its ignore file
// already applied as docker applies it (declaredIgnoreFile), links kept as
// links, and, when the Dockerfile reads them, the install lists
// (DeclaredInstallDir), which the build takes as
// `--build-context astro-install=<dir>/astro-install`. It reports whether it
// wrote the lists.
func WriteDeclaredContext(dir string, req Request) (lists bool, err error) {
	if !req.FromDeclaredDockerfile() {
		return false, errors.New("the project declares no Dockerfile of its own")
	}
	switch entries, err := os.ReadDir(dir); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", dir, err)
	case len(entries) > 0:
		return false, fmt.Errorf("%s %w", dir, ErrContextDirNotEmpty)
	}
	data, err := os.ReadFile(req.Dockerfile)
	if err != nil {
		return false, fmt.Errorf("the Dockerfile this project declares (%s) could not be read: %w", req.Dockerfile, err)
	}
	contextDir := filepath.Join(dir, DeclaredContextDir)
	if err := os.MkdirAll(contextDir, contextDirPerm); err != nil {
		return false, fmt.Errorf("creating %s: %w", contextDir, err)
	}
	if err := copyDeclaredContext(req.Context, req.Dockerfile, dir, contextDir); err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(dir, DeclaredDockerfileName), data, filePermRW); err != nil { //nolint:gosec // G703: dir is the caller's new or empty --context-dir, and the name a constant
		return false, fmt.Errorf("writing %s: %w", DeclaredDockerfileName, err)
	}
	if !airflowrt.ReadsInstallContext(data) {
		return false, nil
	}
	return true, writeLists(filepath.Join(dir, DeclaredInstallDir), req.Lists())
}

// copyDeclaredContext copies the project at src into dst as a build of its
// Dockerfile at dockerfile reads it: under docker's ignore file, version
// control and links included, each link as itself. out, the directory being
// written, which holds dst, is left out when it is inside the project
// (`--context-dir ./build`), so the context never carries a copy of itself.
func copyDeclaredContext(src, dockerfile, out, dst string) error {
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	outInfo, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	rel, err := filepath.Rel(src, dockerfile)
	if err != nil {
		return fmt.Errorf("reading the project's files: %w", err)
	}
	filter, err := newDeclaredFilter(resolved, declaredIgnoreFile(resolved, filepath.ToSlash(rel), false))
	if err != nil {
		return err
	}
	w := walker{filter: filter, worktrees: projectWorktrees(resolved), exclude: outInfo, visit: func(rel, from string, mode fs.FileMode) error {
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), ownerAll); err != nil {
			return err
		}
		if mode&fs.ModeSymlink != 0 {
			link, err := os.Readlink(from)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(from, target, mode.Perm())
	}}
	if err := w.walkDir(resolved, "", map[string]bool{resolved: true}); err != nil {
		return fmt.Errorf("copying the project's files: %w", err)
	}
	return nil
}
