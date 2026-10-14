// Package project finds the astro project containing a directory and
// derives its identity. A project is any directory holding a pyproject.toml
// (the manifest); identity is the sha256 of the symlink-resolved absolute
// path, which keys all per-project state. Hostnames are display labels only.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Marker is the file whose presence makes a directory a project root.
//
// Re-exported rather than moved outright: this package's callers ask it what a
// project looks like, and sending every one of them to pkg/manifest for the
// filename would be a wider change than the one it saves. The single definition
// is manifest.Marker.
const Marker = manifest.Marker

// initCommand is what turns a directory, a 1.x project included, into a
// project in place.
const initCommand = "astro init"

// NotFoundError reports that no project marker was found in the start
// directory or any of its parents.
type NotFoundError struct {
	Start string
	// Project1xDir is the nearest directory on the walk up that holds a 1.x project
	// (scaffold.Is1xProject), or empty when there is none. A 1.x project has no
	// marker, so without this the error would only say what is missing, not
	// that `astro init` upgrades what is there.
	Project1xDir string
	// blocked is why astro init refuses Project1xDir (blockedFor), decided
	// when the error is made.
	blocked Block
}

func (e *NotFoundError) Error() string {
	if e.blocked != NotBlocked && e.Project1xDir != "" {
		return Blocked1xMessage(e.blocked, e.Project1xDir)
	}
	if e.Project1xDir != "" {
		return project1xMessage(e.Project1xDir)
	}
	return fmt.Sprintf("no Astro project found: no %s in %s or any parent directory.\nRun `%s` to make this directory one",
		Marker, e.Start, initCommand)
}

// NoAstroSectionError reports a project root whose pyproject.toml has no
// [tool.astro] table: a Python project, often a 1.x one keeping ruff or
// pytest settings there, that is not yet an astro project. It wraps
// manifest.ErrNoAstroSection, so callers matching the sentinel still match.
type NoAstroSectionError struct {
	// Start is the directory the command ran in, Dir or one below it.
	Start string
	Dir   string
	// Has1xProject is whether a 1.x project lies between Start and Dir, Dir
	// included, or, where astro init refuses one (an APC or unresolved
	// context), anywhere Start is in; Project1xDir names it.
	Has1xProject bool
	Project1xDir string
	// blocked is why astro init refuses Project1xDir, decided when the error
	// is made.
	blocked Block
}

func (e *NoAstroSectionError) Error() string {
	root := e.Project1xDir
	if root == "" {
		root = e.Dir
	}
	if e.blocked != NotBlocked {
		return Blocked1xMessage(e.blocked, root)
	}
	if e.Has1xProject {
		return project1xMessage(root)
	}
	return fmt.Sprintf("%s has no [tool.astro] section, so this is not an Astro project yet.\n"+
		"Run `%s` in %s to add one; the rest of the file is left alone",
		filepath.Join(e.Dir, Marker), initCommand, e.Dir)
}

func (e *NoAstroSectionError) Unwrap() error { return manifest.ErrNoAstroSection }

// project1xMessage says that project1xDir holds a 1.x project and how to
// upgrade it: astro init in that directory, wherever the command ran.
func project1xMessage(project1xDir string) string {
	return fmt.Sprintf("%s holds a project made by Astro CLI 1.x (Dockerfile and .astro/), which this CLI cannot run until it is upgraded.\n"+
		"Run %s in %s to upgrade it in place", project1xDir, initCommand, scaffold.ShellQuote(project1xDir))
}

// LoadError returns the error to report for a manifest.Load of dir's marker,
// discovered from start, that failed with err: a *NoAstroSectionError in place
// of the bare manifest.ErrNoAstroSection, and err unchanged otherwise.
func LoadError(start, dir string, err error) error {
	if !errors.Is(err, manifest.ErrNoAstroSection) {
		return err
	}
	if abs, absErr := filepath.Abs(start); absErr == nil {
		start = abs
	}
	// Where astro init refuses, the 1.x project is the one its own walk finds
	// (Convert1xBlocked), so this error says what init would. Elsewhere it is
	// one between start and dir: a tools-only pyproject.toml above a 1.x
	// project (a monorepo root) does not hide it, while a plain package's
	// pyproject.toml inside a 1.x tree gets the add-[tool.astro] hint.
	if why, root := Convert1xBlocked(start); why != NotBlocked {
		return &NoAstroSectionError{Start: start, Dir: dir, Has1xProject: true, Project1xDir: root, blocked: why}
	}
	root := find1xBetween(start, dir)
	return &NoAstroSectionError{Start: start, Dir: dir, Has1xProject: root != "", Project1xDir: root}
}

// find1xBetween is the nearest 1.x project from start up to dir, dir
// included, or "".
func find1xBetween(start, dir string) string {
	dir = filepath.Clean(dir)
	for d := filepath.Clean(start); ; {
		if scaffold.Is1xProject(d) {
			return d
		}
		parent := filepath.Dir(d)
		if d == dir || parent == d {
			return ""
		}
		d = parent
	}
}

// Project is a discovered astro project.
type Project struct {
	// Dir is the absolute project root as the user references it, with
	// symlinks intact — error messages and display should use this.
	Dir string
	// ID keys per-project state (see ID). Never display it; never key
	// anything by Hostname instead of it.
	ID string
	// Hostname is the display label for the project's local Airflow:
	// <dir>.localhost, or <worktree>.<repo>.localhost inside a linked git
	// worktree. Distinct projects can share a hostname, so it is never an
	// identity.
	Hostname   string
	IsWorktree bool
}

// Discover walks up from startDir looking for a directory that contains
// Marker and returns it as the project. It returns *NotFoundError when the
// walk reaches the filesystem root without a match, naming the nearest 1.x
// project it passed on the way.
func Discover(startDir string) (*Project, error) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", startDir, err)
	}
	var project1xDir string
	for dir := abs; ; {
		info, err := os.Stat(filepath.Join(dir, Marker))
		if err == nil && info.Mode().IsRegular() {
			return New(dir)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		// No pyproject.toml anywhere up to here, so this is the walk
		// scaffold.Find1xProject makes, kept in step rather than repeated.
		if project1xDir == "" && scaffold.Is1xProject(dir) {
			project1xDir = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, &NotFoundError{Start: abs, Project1xDir: project1xDir, blocked: blockedFor(project1xDir)}
		}
		dir = parent
	}
}

// New builds a Project for a directory already known to be a project root.
func New(dir string) (*Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", dir, err)
	}
	id, err := ID(abs)
	if err != nil {
		return nil, err
	}
	hostname, isWorktree, err := proxy.DeriveHostname(abs)
	if err != nil {
		// Hostname is a cosmetic display label; identity is the hash (id).
		// A directory name with no DNS-usable characters (all non-ASCII, all
		// punctuation) leaves DeriveHostname with no label to build from.
		// Fall back to an ID-derived label so a valid, unique hostname always
		// exists and project-scoped commands keep working.
		hostname = "astro-" + id[:8] + proxy.LocalhostSuffix
	}
	return &Project{
		Dir:        abs,
		ID:         id,
		IsWorktree: isWorktree,
		Hostname:   hostname,
	}, nil
}

// HasManifest reports whether dir's pyproject.toml carries a [tool.astro]
// table, the manifest. A pyproject that fails to parse, or one whose
// [tool.astro] fails validation, still counts — it is a project with a
// manifest to fix, and the code that loads the manifest gives the clearer
// error. A pyproject without [tool.astro] (a plain Python project, or one that
// only configures tools such as ruff or pytest) and a missing pyproject do not.
func HasManifest(dir string) bool {
	return scaffold.HasManifest(dir)
}

// ID returns the identity key for a project directory: the sha256 hex of
// its symlink-resolved absolute path. It is a thin wrapper over
// localrt.ProjectID, which owns project identity (docs/architecture.md).
func ID(dir string) (string, error) {
	return localrt.ProjectID(dir)
}
