package imagebuild

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A generated build for an image that is shipped — a deploy, `astro package
// astro` — carries the project's own files, not only its dependencies: a DAG
// that imports include/ or a plugin from plugins/ fails on Astro without them,
// since a deploy ships dags/ alone beside the image. The runtime image's
// ONBUILD `COPY . .` runs after both installs, so the files land in
// /usr/local/airflow in a layer of their own and the dependency layers keep
// their cache.

// skippedProjectNames are never copied, at any depth: version control, local
// environments, caches, and this CLI's per-machine state. None belongs in an
// image, and some (a virtualenv) are larger than the rest of the project.
var skippedProjectNames = map[string]bool{
	".git":          true,
	".hg":           true,
	".svn":          true,
	".venv":         true,
	"venv":          true,
	".astro":        true,
	"__pycache__":   true,
	".pytest_cache": true,
	".mypy_cache":   true,
	".ruff_cache":   true,
	".tox":          true,
	"node_modules":  true,
	".DS_Store":     true,
}

// generatedNames are the files a generated build writes at the context root.
// The project's own copies are not carried, so they cannot replace the
// generated ones, and the .dockerignore written into the context always keeps
// the generated ones.
var generatedNames = map[string]bool{
	requirementsName: true,
	packagesName:     true,
	dockerIgnoreName: true,
}

const (
	dockerIgnoreName = ".dockerignore"
	ownerAll         = 0o700
)

// copyProjectFiles copies the project in src into the build context dst,
// leaving out skippedProjectNames everywhere and generatedNames at the root.
// A .dockerignore the project has is carried with the generated files kept
// out of it, so the daemon applies the project's own exclusions to the rest.
// Files keep their mode, and a symlink is copied as a symlink.
func copyProjectFiles(src, dst string) error {
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skippedProjectNames[d.Name()] || (!strings.ContainsRune(rel, filepath.Separator) && generatedNames[rel]) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			// The owner keeps full access, so the copy can be written into
			// and cleaned up whatever the source's mode.
			return os.MkdirAll(target, info.Mode().Perm()|ownerAll)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		}
		// A socket, a pipe or a device has no place in an image.
		return nil
	})
	if err != nil {
		return fmt.Errorf("copying the project into the image: %w", err)
	}
	return writeDockerIgnore(src, dst)
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // the copy error is the one reported
		return err
	}
	return out.Close()
}

// writeDockerIgnore carries the project's .dockerignore into the context, with
// the generated files re-included after it: a pattern of the project's that
// matched requirements.txt would otherwise drop the dependency list from the
// build. A project without one gets none.
func writeDockerIgnore(src, dst string) error {
	data, err := os.ReadFile(filepath.Join(src, dockerIgnoreName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the project's %s: %w", dockerIgnoreName, err)
	}
	keep := "\n!" + requirementsName + "\n!" + packagesName + "\n"
	//nolint:gosec // G703: dst is the build's own context directory
	return os.WriteFile(filepath.Join(dst, dockerIgnoreName), append(data, keep...), filePermRW)
}
