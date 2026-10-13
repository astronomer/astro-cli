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
	"slices"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
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
// project's can bring one back: the files no shipped image carries
// (airflowrt.ShippedImageRules: the .env family, airflow_settings.yaml, and a
// root airflow.db, airflow.cfg and logs/), and the per-machine files Astro
// tools write into a project (airflowrt.LocalFileRules). They hold something
// true of one machine and wrong, or secret, on any other, and an image is
// pushed to a registry.
//
// Both lists are airflowrt's, the same ones pkg/scaffold writes into the
// .dockerignore a declared Dockerfile's build reads, so the two kinds of image
// leave out the same files without a second copy to keep in step.
var ShippedImageIgnores = slices.Concat(airflowrt.ShippedImageRules(), airflowrt.LocalFileRules())

// generatedNames are the files a generated build writes at the context root.
// The project's own copies, in any case, are not carried, so they cannot
// replace the generated ones, even on a filesystem that folds case.
var generatedNames = []string{requirementsName, packagesName}

const (
	dockerIgnoreName = ".dockerignore"
	ownerAll         = 0o700
)

// projectFilter decides which of the project's files an image carries.
type projectFilter struct {
	pm *patternmatcher.PatternMatcher
	// generated applies a generated build's own rules on top of pm:
	// skippedProjectNames, and the root files the build writes itself. A
	// declared Dockerfile's context has none of them.
	generated bool
}

// newProjectFilter reads the project's .dockerignore, if it has one, and adds
// ShippedImageIgnores after it.
func newProjectFilter(root string) (projectFilter, error) {
	patterns, err := readIgnoreFile(root, dockerIgnoreName)
	if err != nil {
		return projectFilter{}, err
	}
	pm, err := patternmatcher.New(append(patterns, ShippedImageIgnores...))
	if err != nil {
		return projectFilter{}, fmt.Errorf("reading the project's %s: %w", dockerIgnoreName, err)
	}
	return projectFilter{pm: pm, generated: true}, nil
}

// newDeclaredFilter reads the ignore file a build of the project's own
// Dockerfile reads, which is all that build leaves out: BuildKit reads
// <Dockerfile>.dockerignore beside the Dockerfile when there is one, else the
// context's .dockerignore. dockerfile is the manifest's value, relative to the
// project and slash-separated.
func newDeclaredFilter(root, dockerfile string) (projectFilter, error) {
	name := dockerIgnoreName
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dockerfile)+dockerIgnoreName)); err == nil {
		name = filepath.FromSlash(dockerfile) + dockerIgnoreName
	}
	patterns, err := readIgnoreFile(root, name)
	if err != nil {
		return projectFilter{}, err
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return projectFilter{}, fmt.Errorf("reading the project's %s: %w", filepath.ToSlash(name), err)
	}
	return projectFilter{pm: pm}, nil
}

// readIgnoreFile reads the ignore file at name under root; none is no rules.
func readIgnoreFile(root, name string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, name))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the project's %s: %w", filepath.ToSlash(name), err)
	}
	patterns, err := ignorefile.ReadAll(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("reading the project's %s: %w", filepath.ToSlash(name), err)
	}
	return patterns, nil
}

// skip reports whether the file or directory at rel (slash-separated, from the
// project root) stays out of the image. A directory a rule matches is not
// descended into unless a `!` rule could bring something under it back.
func (f projectFilter) skip(rel string, isDir bool) (skip, descend bool) {
	base := path.Base(rel)
	if f.generated && skippedProjectNames[base] {
		return true, false
	}
	if f.generated && !strings.Contains(rel, "/") {
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

// walker walks a project for the files an image carries of it.
type walker struct {
	filter projectFilter
	// follow carries a symlink as what it points at, as a generated build
	// copies it. Without it a link is visited as itself and never entered, as
	// docker copies a declared Dockerfile's context.
	follow bool
	// skipVCS leaves out version-control metadata, which a generated build's
	// filter already does.
	skipVCS bool
	// visit is called for each file carried, with its path relative to the
	// project (slash-separated), its source path and its mode: a regular
	// file's, or a symlink's when links are not followed.
	visit func(rel, src string, mode fs.FileMode) error
}

// walkProject calls visit for every regular file the project's generated
// image carries. It follows symlinks, so a link to a file or directory, inside
// the project or out of it, is carried as what it points at: a link copied as
// a link would dangle in the image when it points outside the project, and
// creating one needs a privilege most Windows users lack. A link that resolves
// to a directory already being walked is not followed again, and a dangling
// one is left out. The root itself may be a symlink.
func walkProject(root string, visit func(rel, src string, mode fs.FileMode) error) error {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	filter, err := newProjectFilter(resolved)
	if err != nil {
		return err
	}
	w := walker{filter: filter, follow: true, visit: visit}
	return w.walkDir(resolved, "", map[string]bool{resolved: true})
}

func (w walker) walkDir(dir, rel string, onPath map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		childRel := path.Join(rel, e.Name())
		src := filepath.Join(dir, e.Name())
		stat := os.Stat // follows a symlink
		if !w.follow {
			stat = os.Lstat
		}
		info, err := stat(src)
		if errors.Is(err, fs.ErrNotExist) {
			// A dangling link has nothing to carry, and a file removed since
			// the directory was read (a tool's temp file) is not there to.
			continue
		}
		if err != nil {
			return err
		}
		if w.skipVCS && info.IsDir() && vcsNames[e.Name()] {
			continue
		}
		skip, descend := w.filter.skip(childRel, info.IsDir())
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
			err = w.walkDir(src, childRel, onPath)
			delete(onPath, realDir)
			if err != nil {
				return err
			}
		case skip:
			// Under a directory a rule matched, kept only for a `!` rule
			// that did not bring this file back.
		case info.Mode().IsRegular(), info.Mode()&fs.ModeSymlink != 0:
			// A symlink gets here only when links are not followed: it is
			// carried as itself.
			if err := w.visit(childRel, src, info.Mode()); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		// A socket, a pipe or a device has no place in an image.
	}
	return nil
}

// vcsNames are version-control metadata directories.
var vcsNames = map[string]bool{".git": true, ".hg": true, ".svn": true}

// dagsDir is where a project's DAGs live, relative to its root.
const dagsDir = "dags"

// ProjectSurvey is what an image built from a project would carry of it.
type ProjectSurvey struct {
	// Files are the project's files the image carries, slash-separated and
	// relative to the project, in walk order. Version-control metadata is not
	// listed, even where a declared Dockerfile's context carries it.
	Files []string
	// DagsOnDisk counts the DAG files (.py, as the 1.x CLI counted them)
	// under dags/, links followed and no rule applied. DagsCarried counts
	// those of them the image carries.
	DagsOnDisk, DagsCarried int
}

// SurveyProject lists what the image a deploy or `astro package astro` builds
// from the project at dir would carry of it. dockerfile is the manifest's
// [tool.astro] dockerfile: "" for a generated build, which carries what
// copyProjectFiles copies; otherwise the declared Dockerfile's build, whose
// context is the project under its ignore file alone, links carried as links.
func SurveyProject(dir, dockerfile string) (ProjectSurvey, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ProjectSurvey{}, fmt.Errorf("reading the project's files: %w", err)
	}
	var s ProjectSurvey
	collect := func(rel, _ string, mode fs.FileMode) error {
		s.Files = append(s.Files, rel)
		if mode.IsRegular() && isDagFile(rel) {
			s.DagsCarried++
		}
		return nil
	}
	if dockerfile == "" {
		err = walkProject(resolved, collect)
	} else {
		var filter projectFilter
		if filter, err = newDeclaredFilter(resolved, dockerfile); err != nil {
			return ProjectSurvey{}, err
		}
		w := walker{filter: filter, skipVCS: true, visit: collect}
		err = w.walkDir(resolved, "", map[string]bool{resolved: true})
	}
	if err != nil {
		return ProjectSurvey{}, fmt.Errorf("reading the project's files: %w", err)
	}
	if s.DagsOnDisk, err = countDagsOnDisk(resolved); err != nil {
		return ProjectSurvey{}, err
	}
	return s, nil
}

// isDagFile reports whether rel, slash-separated, is a .py file under dags/.
func isDagFile(rel string) bool {
	return strings.HasPrefix(rel, dagsDir+"/") && strings.HasSuffix(rel, ".py")
}

// countDagsOnDisk counts the .py files under root's dags/, links followed and
// no ignore rule applied, so an empty dags/ can be told from one whose DAGs an
// image leaves out.
func countDagsOnDisk(root string) (int, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, dagsDir))
	if err != nil {
		return 0, nil // no dags/, or a dangling link: no DAGs
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return 0, nil
	}
	none, err := patternmatcher.New(nil)
	if err != nil {
		return 0, err
	}
	n := 0
	w := walker{filter: projectFilter{pm: none}, follow: true, visit: func(rel, _ string, _ fs.FileMode) error {
		if strings.HasSuffix(rel, ".py") {
			n++
		}
		return nil
	}}
	if err := w.walkDir(resolved, dagsDir, map[string]bool{resolved: true}); err != nil {
		return 0, fmt.Errorf("reading the project's dags/: %w", err)
	}
	return n, nil
}

// copyProjectFiles copies the files the project's image carries from src into
// the build context dst.
func copyProjectFiles(src, dst string) error {
	err := walkProject(src, func(rel, from string, mode fs.FileMode) error {
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), ownerAll); err != nil {
			return err
		}
		return copyFile(from, target, mode.Perm())
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
		fmt.Fprintf(h, "%s\x00%o\x00", rel, mode.Perm())
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
