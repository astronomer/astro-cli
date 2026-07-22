// Package project finds the astro project containing a directory and
// derives its identity. A project is any directory holding a pyproject.toml
// (the v2 manifest); identity is the sha256 of the symlink-resolved absolute
// path, which keys all per-project state. Hostnames are display labels only.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Marker is the file whose presence makes a directory a project root.
const Marker = "pyproject.toml"

// NotFoundError reports that no project marker was found in the start
// directory or any of its parents.
type NotFoundError struct {
	Start string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no %s found in %s or any parent directory", Marker, e.Start)
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
// walk reaches the filesystem root without a match.
func Discover(startDir string) (*Project, error) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", startDir, err)
	}
	for dir := abs; ; {
		info, err := os.Stat(filepath.Join(dir, Marker))
		if err == nil && info.Mode().IsRegular() {
			return New(dir)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, &NotFoundError{Start: abs}
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

// ID returns the identity key for a project directory: the sha256 hex of
// its symlink-resolved absolute path. It is a thin wrapper over
// localrt.ProjectID, which owns project identity (docs/v2-architecture.md).
func ID(dir string) (string, error) {
	return localrt.ProjectID(dir)
}
