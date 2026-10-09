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
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/proxy"
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
	// (see Is1xProject), or empty when there is none. A 1.x project has no
	// marker, so without this the error would only say what is missing, not
	// that `astro init` upgrades what is there.
	Project1xDir string
	// UnderAPC says the current context is Astro Private Cloud, where a 1.x
	// project is not to be converted yet (Project1xUnderAPC). Set by
	// AdviseUnderAPC, since this package does not read the context.
	UnderAPC bool
}

func (e *NotFoundError) Error() string {
	if e.Project1xDir != "" {
		return project1xMessage(e.Start, e.Project1xDir, e.UnderAPC)
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
	// Has1xProject is whether Dir also holds a 1.x project (see Is1xProject).
	Has1xProject bool
	// UnderAPC is NotFoundError.UnderAPC.
	UnderAPC bool
}

func (e *NoAstroSectionError) Error() string {
	if e.Has1xProject {
		return project1xMessage(e.Start, e.Dir, e.UnderAPC)
	}
	return fmt.Sprintf("%s has no [tool.astro] section, so this is not an Astro project yet.\n"+
		"Run `%s` in %s to add one; the rest of the file is left alone",
		filepath.Join(e.Dir, Marker), initCommand, e.Dir)
}

func (e *NoAstroSectionError) Unwrap() error { return manifest.ErrNoAstroSection }

// project1xMessage says that project1xDir holds a 1.x project and how to upgrade it,
// naming the directory only when it is not the one the command ran in. Under
// an Astro Private Cloud context (apc) it says what Project1xUnderAPC says
// instead.
func project1xMessage(start, project1xDir string, apc bool) string {
	where, there := "this directory", "here"
	if project1xDir != start {
		where, there = project1xDir, "in "+project1xDir
	}
	if apc {
		return Project1xUnderAPC(where)
	}
	return fmt.Sprintf("%s holds a project made by Astro CLI 1.x (Dockerfile and .astro/), which this CLI cannot run until it is upgraded.\n"+
		"Run `%s` %s to upgrade it in place", where, initCommand, there)
}

// Project1xUnderAPC is the one account of a 1.x project under an Astro
// Private Cloud context, where is the directory holding it as a sentence
// begins with it. APC's deploy still builds the 1.x layout, so astro init
// refuses to convert one there, and every hint that would otherwise say to
// run it says this instead: astro init's refusal, the errors of a command run
// in such a project, and the astro dev stub.
func Project1xUnderAPC(where string) string {
	return where + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context is " +
		"Astro Private Cloud, whose astro deploy still builds that layout. Leave the project as it is for now: " +
		"astro deploy keeps working with it on Astro Private Cloud, and converting it will be available once " +
		"Astro Private Cloud deploys pyproject.toml projects. To convert it anyway, for Astro or for local " +
		"development only, switch to an Astro context first (astro context switch astronomer.io, or astro login " +
		"to sign in to Astro) and run " + initCommand + " again"
}

// AdviseUnderAPC returns err with a 1.x project's NotFoundError or
// NoAstroSectionError in it marked as met under an Astro Private Cloud
// context, so its message says what Project1xUnderAPC says. A wrapping
// fmt.Errorf has already rendered the old message into its own, so that text
// is replaced in the outer message too; errors.Is and errors.As still see
// everything err wraps.
func AdviseUnderAPC(err error) error {
	var before, after string
	var nf *NotFoundError
	var ns *NoAstroSectionError
	switch {
	case errors.As(err, &nf) && nf.Project1xDir != "" && !nf.UnderAPC:
		before = nf.Error()
		nf.UnderAPC = true
		after = nf.Error()
	case errors.As(err, &ns) && ns.Has1xProject && !ns.UnderAPC:
		before = ns.Error()
		ns.UnderAPC = true
		after = ns.Error()
	default:
		return err
	}
	return &advisedError{err: err, msg: strings.Replace(err.Error(), before, after, 1)}
}

// advisedError is err with its message rewritten by AdviseUnderAPC.
type advisedError struct {
	err error
	msg string
}

func (e *advisedError) Error() string { return e.msg }
func (e *advisedError) Unwrap() error { return e.err }

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
	return &NoAstroSectionError{Start: start, Dir: dir, Has1xProject: Is1xProject(dir)}
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
		if project1xDir == "" && Is1xProject(dir) {
			project1xDir = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, &NotFoundError{Start: abs, Project1xDir: project1xDir}
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
	_, err := manifest.Load(filepath.Join(dir, Marker))
	switch {
	case err == nil:
		return true
	case errors.Is(err, manifest.ErrNotFound), errors.Is(err, manifest.ErrNoAstroSection):
		return false
	default:
		return true
	}
}

// Is1xProject reports whether dir holds a 1.x project: the old Dockerfile
// layout with a .astro/ directory, and no manifest. A Dockerfile on its own
// marks some container project, not necessarily a 1.x one, so it does not
// qualify — `astro dev` must not claim such a directory is 1.x. A
// pyproject.toml does not rule 1.x out: plenty of 1.x repositories keep one
// for ruff or pytest settings, and only one that HasManifest accepts makes the
// directory a project with a manifest. `astro init` converts a 1.x directory,
// reporting the 1.x files it could not read rather than refusing them, and
// consults this only under an Astro Private Cloud context, where it refuses
// one (Project1xUnderAPC).
func Is1xProject(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, ".astro"))
	return err == nil && info.IsDir() && !HasManifest(dir)
}

// ID returns the identity key for a project directory: the sha256 hex of
// its symlink-resolved absolute path. It is a thin wrapper over
// localrt.ProjectID, which owns project identity (docs/architecture.md).
func ID(dir string) (string, error) {
	return localrt.ProjectID(dir)
}
