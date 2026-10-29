package imagebuild

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
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

// containerIgnoreName is podman's and buildah's own ignore file, which they
// read before .dockerignore.
const containerIgnoreName = ".containerignore"

// declaredIgnoreFile is the ignore file a build of the project's own
// Dockerfile reads, relative to the project and slash-separated, or "" when
// there is none. Under podman a .containerignore comes first, as podman and
// buildah read it; docker never reads one. Then <Dockerfile>.dockerignore
// beside the Dockerfile, which BuildKit reads in place of the context's, else
// .dockerignore. dockerfile is the manifest's value, relative to the project
// and slash-separated.
func declaredIgnoreFile(root, dockerfile string, podman bool) string {
	names := []string{dockerfile + dockerIgnoreName, dockerIgnoreName}
	if podman {
		names = append([]string{containerIgnoreName}, names...)
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err == nil {
			return name
		}
	}
	return ""
}

// newDeclaredFilter reads the ignore file a build of the project's own
// Dockerfile reads (declaredIgnoreFile), which is all that build leaves out.
func newDeclaredFilter(root, ignoreFile string) (projectFilter, error) {
	var patterns []string
	if ignoreFile != "" {
		var err error
		if patterns, err = readIgnoreFile(root, filepath.FromSlash(ignoreFile)); err != nil {
			return projectFilter{}, err
		}
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return projectFilter{}, fmt.Errorf("reading the project's %s: %w", ignoreFile, err)
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
	// vcs, when set, takes version-control metadata (.git, a directory or a
	// submodule's file, and the like) that the filter leaves in, which is
	// then neither entered nor visited. A generated build's filter leaves all
	// of it out.
	vcs func(rel string)
	// cosmosBoost, when set, takes each Cosmos Boost file an earlier Astro CLI
	// wrote into the project (airflowrt.CosmosBoostFileWrittenByAstro) that
	// the walk carries. A generated build's filter leaves them out instead.
	cosmosBoost func(rel string)
	// link, when set, takes each symlink a following walk goes through: the
	// link's own path, for a file carried as what it points at or a directory
	// entered through it.
	link func(rel string)
	// visit is called for each file carried, with its path relative to the
	// project (slash-separated), its source path and its mode: a regular
	// file's, or a symlink's when links are not followed.
	visit func(rel, src string, mode fs.FileMode) error
	// nested, when set, takes each nested checkout (nestedCheckout) the walk
	// carries, which a declared Dockerfile's context does. A generated build
	// leaves them out instead.
	nested func(rel string)
	// root is the project root the walk started at, resolved.
	root string
	// tracked holds the checkouts the project's git tracks (submodules,
	// trackedCheckouts), read once a walk first meets a nested .git.
	tracked *trackedCheckouts
}

// nestedCheckout reports whether the directory at src, rel from the project
// root, is a git checkout of its own inside the project that the project's
// git does not track: a worktree (a .git file pointing elsewhere), or a clone
// (a .git directory). Agents and tools put worktrees inside a project
// (.claude/worktrees/, a clone checked out under it), and an image would carry
// a second copy of the repository, with whatever is ignored there. A submodule,
// or a repository the project's index records as a gitlink, is part of the
// project and is carried like any other directory.
func (w walker) nestedCheckout(root, src, rel string) bool {
	if w.tracked == nil {
		return false // a walk that applies no rules (countDagsOnDisk)
	}
	if _, err := os.Lstat(filepath.Join(src, ".git")); err != nil {
		return false
	}
	return !w.tracked.has(root, rel)
}

// trackedCheckouts are the directories, slash-separated from the project root,
// that the project's git tracks as checkouts of their own: the paths its
// .gitmodules lists, and the gitlinks (mode 160000) its index records, which
// cover a repository added without a .gitmodules entry. A submodule's own
// submodules are read when the walk enters it.
type trackedCheckouts struct {
	read  map[string]bool // the repositories, by prefix, already read
	paths map[string]bool
}

func newTrackedCheckouts() *trackedCheckouts {
	return &trackedCheckouts{read: map[string]bool{}, paths: map[string]bool{}}
}

// has reports whether rel is a tracked checkout, reading the project's own
// list first if nothing has been read yet.
func (t *trackedCheckouts) has(root, rel string) bool {
	t.readRepo(root, "")
	return t.paths[rel]
}

// enter reads the tracked checkouts of the submodule at src (rel from the
// root), for the walk below it.
func (t *trackedCheckouts) enter(src, rel string) {
	if t != nil && t.paths[rel] {
		t.readRepo(src, rel)
	}
}

// readRepo adds the checkouts the repository at dir (prefix from the project
// root, "" for the project's own) tracks, once. Git is asked for the gitlinks
// in its index; when it cannot answer (no git, no repository) .gitmodules
// alone answers.
func (t *trackedCheckouts) readRepo(dir, prefix string) {
	if t.read[prefix] {
		return
	}
	t.read[prefix] = true
	readGitmodules(dir, prefix, t.paths)
	out, err := exec.Command("git", "-C", dir, "ls-files", "--stage", "-z").Output()
	if err != nil {
		return
	}
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, p, ok := strings.Cut(entry, "\t")
		if ok && strings.HasPrefix(meta, "160000 ") {
			t.paths[path.Join(prefix, p)] = true
		}
	}
}

// readGitmodules adds the submodule paths the .gitmodules in dir lists, as
// paths from the project root (prefix is dir's own path, "" for the root).
// Git writes `path = <dir>` under each [submodule] section; a missing or
// unreadable file lists none.
func readGitmodules(dir, prefix string, into map[string]bool) {
	data, err := os.ReadFile(filepath.Join(dir, ".gitmodules"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "path" {
			continue
		}
		p := path.Clean(strings.Trim(strings.TrimSpace(value), `"`))
		if p == "." || strings.HasPrefix(p, "../") {
			continue
		}
		into[path.Join(prefix, p)] = true
	}
}

// walkProject calls visit for every regular file the project's generated
// image carries. It follows symlinks, so a link to a file or directory, inside
// the project or out of it, is carried as what it points at: a link copied as
// a link would dangle in the image when it points outside the project, and
// creating one needs a privilege most Windows users lack. A link that resolves
// to a directory already being walked is not followed again, and a dangling
// one is left out. The root itself may be a symlink. A git checkout nested in
// the project that is not one of its submodules is left out whole
// (walker.nestedCheckout).
func walkProject(root string, visit func(rel, src string, mode fs.FileMode) error) error {
	return walkProjectLinks(root, visit, nil)
}

// walkProjectLinks is walkProject, handing link each symlink it goes through
// (walker.link).
func walkProjectLinks(root string, visit func(rel, src string, mode fs.FileMode) error, link func(rel string)) error {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	filter, err := newProjectFilter(resolved)
	if err != nil {
		return err
	}
	w := walker{filter: filter, follow: true, link: link, visit: visit, root: resolved, tracked: newTrackedCheckouts()}
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
		info, err := w.stat(src)
		if err != nil {
			return err
		}
		if info == nil {
			continue
		}
		skip, descend := w.filter.skip(childRel, info.IsDir())
		if w.vcs != nil && vcsNames[e.Name()] {
			if !skip {
				w.vcs(childRel)
			}
			continue
		}
		if skip && !descend {
			continue
		}
		if info.IsDir() && w.leavesOutCheckout(src, childRel, skip) {
			continue
		}
		if w.follow && w.link != nil && e.Type()&fs.ModeSymlink != 0 && (info.IsDir() || !skip) {
			w.link(childRel)
		}
		switch {
		case info.IsDir():
			if err := w.enter(src, childRel, onPath); err != nil {
				return err
			}
		case skip:
			// Under a directory a rule matched, kept only for a `!` rule
			// that did not bring this file back.
		case info.Mode().IsRegular(), info.Mode()&fs.ModeSymlink != 0:
			// A symlink gets here only when links are not followed: it is
			// carried as itself.
			if err := w.carry(childRel, src, info.Mode()); err != nil {
				return err
			}
		}
		// A socket, a pipe or a device has no place in an image.
	}
	return nil
}

// stat is the entry at src as the walk sees it: what a symlink points at when
// links are followed, the link itself otherwise. A nil info and no error is an
// entry gone: a dangling link has nothing to carry, and a file removed since
// the directory was read (a tool's temp file) is not there to.
func (w walker) stat(src string) (fs.FileInfo, error) {
	stat := os.Stat // follows a symlink
	if !w.follow {
		stat = os.Lstat
	}
	info, err := stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return info, err
}

// leavesOutCheckout reports whether the walk leaves the directory at src (rel
// from the root) out because it is a nested checkout (nestedCheckout), which a
// generated build does. A declared build's walk carries it and hands it to
// nested, unless a rule already leaves it out (skip). A submodule's own
// tracked checkouts are read on the way in.
func (w walker) leavesOutCheckout(src, rel string, skip bool) bool {
	if w.nestedCheckout(w.root, src, rel) {
		if w.filter.generated {
			return true
		}
		if w.nested != nil && !skip {
			w.nested(rel)
		}
	}
	w.tracked.enter(src, rel)
	return false
}

// carry visits a file the walk carries, unless it is a Cosmos Boost file an
// earlier Astro CLI wrote (staleCosmosBoost), which a generated build leaves
// out and a declared one reports.
func (w walker) carry(rel, src string, mode fs.FileMode) error {
	if mode.IsRegular() {
		stale, err := w.staleCosmosBoost(rel, src)
		if err != nil {
			return err
		}
		if stale && w.filter.generated {
			return nil
		}
		if stale && w.cosmosBoost != nil {
			w.cosmosBoost(rel)
		}
	}
	if err := w.visit(rel, src, mode); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// staleCosmosBoost reports whether the regular file at src is a Cosmos Boost
// sidecar an earlier Astro CLI wrote into the project, which the plugin would
// read as current in the image (airflowrt.CosmosBoostFileWrittenByAstro). The
// CLI no longer writes them; a deploy's bundle leaves them out, and so does a
// generated image. Only a file at a sidecar's path is read, and only when the
// answer is used.
func (w walker) staleCosmosBoost(rel, src string) (bool, error) {
	if (!w.filter.generated && w.cosmosBoost == nil) || !airflowrt.IsCosmosBoostFile(rel) {
		return false, nil
	}
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return airflowrt.CosmosBoostFileWrittenByAstro(rel, data), nil
}

// enter walks the directory at src, unless it is one already being walked
// (a link back up the tree).
func (w walker) enter(src, rel string, onPath map[string]bool) error {
	realDir, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	if onPath[realDir] {
		return nil
	}
	onPath[realDir] = true
	defer delete(onPath, realDir)
	return w.walkDir(src, rel, onPath)
}

// vcsNames are version-control metadata.
var vcsNames = map[string]bool{".git": true, ".hg": true, ".svn": true}

// dagsDir is where a project's DAGs live, relative to its root.
const dagsDir = "dags"

// ProjectSurvey is what an image built from a project would carry of it.
type ProjectSurvey struct {
	// Files are the project's files the image carries, slash-separated and
	// relative to the project, in walk order. Version-control metadata is not
	// listed here.
	Files []string
	// VCS is the version-control metadata (.git and the like) a declared
	// Dockerfile's context carries, which its ignore file leaves in: each
	// one's path, not what is in it. A generated build carries none.
	VCS []string
	// Nested are the git checkouts inside the project that are not its
	// submodules (a worktree under .claude/worktrees/, a clone) and that a
	// declared Dockerfile's context carries, slash-separated: each
	// directory's path. A generated build leaves them out and lists none.
	Nested []string
	// CosmosBoost are the Cosmos Boost files an earlier Astro CLI wrote into
	// the project that a declared Dockerfile's context carries, which the
	// plugin would read as current in the image. A generated build leaves
	// them out and lists none.
	CosmosBoost []string
	// IgnoreFile is the ignore file a declared Dockerfile's build reads
	// (declaredIgnoreFile), "" for none or for a generated build.
	IgnoreFile string
	// Links are the symlinks a generated build goes through, slash-separated
	// and relative to the project: each carried file is either one of them
	// (a file link, carried as its target) or under one (a linked directory)
	// or neither, and only those are anywhere but where Files says. A
	// declared build carries a link as a link, and lists none here.
	Links []string
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
// podman says the engine building it is podman, which reads a
// .containerignore (declaredIgnoreFile).
func SurveyProject(dir, dockerfile string, podman bool) (ProjectSurvey, error) {
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
		err = walkProjectLinks(resolved, collect, func(rel string) { s.Links = append(s.Links, rel) })
	} else {
		s.IgnoreFile = declaredIgnoreFile(resolved, dockerfile, podman)
		var filter projectFilter
		if filter, err = newDeclaredFilter(resolved, s.IgnoreFile); err != nil {
			return ProjectSurvey{}, err
		}
		w := walker{
			filter:      filter,
			vcs:         func(rel string) { s.VCS = append(s.VCS, rel) },
			cosmosBoost: func(rel string) { s.CosmosBoost = append(s.CosmosBoost, rel) },
			nested:      func(rel string) { s.Nested = append(s.Nested, rel) },
			visit:       collect,
			root:        resolved,
			tracked:     newTrackedCheckouts(),
		}
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
