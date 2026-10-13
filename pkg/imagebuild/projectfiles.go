package imagebuild

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

// A generated build for an image that is shipped — a deploy, `astro package
// astro` — carries the project's own files, not only its dependencies: a DAG
// that imports include/ or a plugin from plugins/ fails on Astro without them,
// since a deploy ships dags/ alone beside the image. The runtime image's
// ONBUILD `COPY . .` runs after both installs, so the files land in
// /usr/local/airflow in a layer of their own and the dependency layers keep
// their cache.

// skippedProjectNames are never carried, at any depth: version control, local
// environments and caches. None belongs in an image, and some (a virtualenv)
// are larger than the rest of the project.
var skippedProjectNames = map[string]bool{
	".git":          true,
	".hg":           true,
	".svn":          true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".pytest_cache": true,
	".mypy_cache":   true,
	".ruff_cache":   true,
	".tox":          true,
	"node_modules":  true,
	".DS_Store":     true,
}

// ShippedImageIgnores are .dockerignore rules every shipped image applies on
// top of the project's own .dockerignore, and after it, so no `!` line of the
// project's can bring one back: the plaintext local .env (where `astro local
// env set --plain` writes), and the per-machine files Astro tools write into a
// project. They hold something true of one machine and wrong, or secret, on
// any other, and an image is pushed to a registry.
//
// pkg/scaffold writes the same rules into the .dockerignore a declared
// Dockerfile's build reads. The two modules do not import each other, so a
// root-module test holds this list to cover scaffold's.
var ShippedImageIgnores = []string{
	".env",
	".astro/standalone/",
	".astro/worktrees/",
	".astro/*.local.yaml",
	".astro/*.local.yml",
	".astro/otto/*.local.json",
	".astro/otto/mcp.json",
	"plugins/fix_local_executor_pickle.py",
}

// generatedNames are the files a generated build writes at the context root.
// The project's own copies, in any case, are not carried, so they cannot
// replace the generated ones, even on a filesystem that folds case.
var generatedNames = []string{requirementsName, packagesName}

const (
	dockerIgnoreName = ".dockerignore"
	ownerAll         = 0o700
)

// projectFilter decides which of the project's files a shipped image carries.
type projectFilter struct {
	pm *patternmatcher.PatternMatcher
}

// newProjectFilter reads the project's .dockerignore, if it has one, and adds
// ShippedImageIgnores after it.
func newProjectFilter(root string) (projectFilter, error) {
	var patterns []string
	data, err := os.ReadFile(filepath.Join(root, dockerIgnoreName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return projectFilter{}, fmt.Errorf("reading the project's %s: %w", dockerIgnoreName, err)
	default:
		if patterns, err = ignorefile.ReadAll(strings.NewReader(string(data))); err != nil {
			return projectFilter{}, fmt.Errorf("reading the project's %s: %w", dockerIgnoreName, err)
		}
	}
	pm, err := patternmatcher.New(append(patterns, ShippedImageIgnores...))
	if err != nil {
		return projectFilter{}, fmt.Errorf("reading the project's %s: %w", dockerIgnoreName, err)
	}
	return projectFilter{pm: pm}, nil
}

// skip reports whether the file or directory at rel (slash-separated, from the
// project root) stays out of the image. A directory a rule matches is not
// descended into unless a `!` rule could bring something under it back.
func (f projectFilter) skip(rel string, isDir bool) (skip, descend bool) {
	base := path.Base(rel)
	if skippedProjectNames[base] {
		return true, false
	}
	if !strings.Contains(rel, "/") {
		for _, g := range generatedNames {
			if strings.EqualFold(rel, g) {
				return true, false
			}
		}
		if rel == dockerIgnoreName {
			return true, false
		}
	}
	matched, err := f.pm.MatchesOrParentMatches(rel)
	if err != nil || !matched {
		return false, isDir
	}
	return true, isDir && f.pm.Exclusions()
}

// walkProject calls visit for every regular file the project's image carries,
// with its path relative to root (slash-separated) and its source path. It
// follows symlinks, so a link to a file or directory, inside the project or
// out of it, is carried as what it points at: a link copied as a link would
// dangle in the image when it points outside the project, and creating one
// needs a privilege most Windows users lack. A link that resolves to a
// directory already being walked is not followed again, and a dangling one is
// left out. The root itself may be a symlink.
func walkProject(root string, visit func(rel, src string, mode fs.FileMode) error) error {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	filter, err := newProjectFilter(resolved)
	if err != nil {
		return err
	}
	return walkDir(resolved, "", filter, map[string]bool{resolved: true}, visit)
}

func walkDir(dir, rel string, filter projectFilter, onPath map[string]bool, visit func(rel, src string, mode fs.FileMode) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		childRel := path.Join(rel, e.Name())
		src := filepath.Join(dir, e.Name())
		info, err := os.Stat(src) // follows a symlink
		if errors.Is(err, fs.ErrNotExist) {
			// A dangling link has nothing to carry, and a file removed since
			// the directory was read (a tool's temp file) is not there to.
			continue
		}
		if err != nil {
			return err
		}
		skip, descend := filter.skip(childRel, info.IsDir())
		if skip && !descend {
			continue
		}
		switch {
		case info.IsDir():
			realDir, err := filepath.EvalSymlinks(src)
			if err != nil {
				return err
			}
			if onPath[realDir] {
				continue // a link back up the tree
			}
			onPath[realDir] = true
			err = walkDir(src, childRel, filter, onPath, visit)
			delete(onPath, realDir)
			if err != nil {
				return err
			}
		case skip:
			// Under a directory a rule matched, kept only for a `!` rule
			// that did not bring this file back.
		case info.Mode().IsRegular():
			if err := visit(childRel, src, info.Mode().Perm()); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		// A socket, a pipe or a device has no place in an image.
	}
	return nil
}

// copyProjectFiles copies the files the project's image carries from src into
// the build context dst.
func copyProjectFiles(src, dst string) error {
	err := walkProject(src, func(rel, from string, mode fs.FileMode) error {
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), ownerAll); err != nil {
			return err
		}
		return copyFile(from, target, mode)
	})
	if err != nil {
		return fmt.Errorf("copying the project into the image: %w", err)
	}
	return nil
}

// ProjectFilesDigest is a digest of the files a shipped image carries from the
// project in dir: their paths, modes and contents, after every rule
// copyProjectFiles applies. A content-addressed image tag includes it, since
// the image changes whenever one of those files does.
func ProjectFilesDigest(dir string) (string, error) {
	h := sha256.New()
	err := walkProject(dir, func(rel, src string, mode fs.FileMode) error {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(h, "%s\x00%o\x00", rel, mode)
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		_, err = h.Write([]byte{0})
		return err
	})
	if err != nil {
		return "", fmt.Errorf("reading the project's files: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
