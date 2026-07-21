// Package userstate stores per-user, per-project local state: which
// deployment the project points at, the preferred port, dev mode. It lives
// under the astro cache directory, keyed by project path hash
// (internal/project.ID), so worktrees and copies of a project each get
// their own state.
package userstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/internal/project"
)

const (
	stateFile = "state.json"
	dirPerm   = 0o755
	filePerm  = 0o600
)

// State is what astro remembers about a project for this user. It is not
// committed and not shared; everything here is rebuildable or re-choosable,
// which is why it lives under the cache directory.
type State struct {
	Deployment string `json:"deployment,omitempty"`
	// Port is the preferred webserver port. The runtime may pick another;
	// this is the request, not the fact.
	Port int `json:"port,omitempty"`
	// DevMode records whether dev-mode Airflow overrides are on.
	DevMode bool `json:"devMode,omitempty"`
}

// DecodeError reports a state file that exists but could not be parsed.
type DecodeError struct {
	Path string
	Err  error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("parsing %s: %v", e.Path, e.Err)
}

func (e *DecodeError) Unwrap() error { return e.Err }

// CacheRoot returns the astro cache directory: $XDG_CACHE_HOME/astro when
// set, otherwise ~/.cache/astro. Per the XDG spec a relative XDG_CACHE_HOME
// is invalid and ignored.
func CacheRoot() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "astro"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "astro"), nil
}

// Dir returns the state directory for a project,
// <cache>/projects/<path-hash>. It does not create the directory.
func Dir(projectPath string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	id, err := project.ID(projectPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "projects", id), nil
}

// Load reads the project's state. A missing file is an empty State, not an
// error. After parsing, the state is normalized, and when the normalized
// encoding differs from what was on disk (older schema, dropped fields,
// hand edits) the file is rewritten — so the file on disk always matches
// what this build would write.
func Load(projectPath string) (State, error) {
	dir, err := Dir(projectPath)
	if err != nil {
		return State{}, err
	}
	path := filepath.Join(dir, stateFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, &DecodeError{Path: path, Err: err}
	}
	s.normalize()
	canonical, err := encode(s)
	if err != nil {
		return State{}, err
	}
	if !bytes.Equal(canonical, raw) {
		if err := writeAtomic(path, canonical); err != nil {
			return State{}, err
		}
	}
	return s, nil
}

// Save writes the project's state, creating the state directory as needed.
func Save(projectPath string, s State) error {
	dir, err := Dir(projectPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	s.normalize()
	data, err := encode(s)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, stateFile), data)
}

func (s *State) normalize() {
	if s.Port < 0 {
		s.Port = 0
	}
}

func encode(s State) ([]byte, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding state: %w", err)
	}
	return append(data, '\n'), nil
}

// writeAtomic writes data via a temp file in the same directory plus
// rename, so readers never see a half-written file and a crash leaves the
// old contents intact.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+stateFile+".*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmpPath, filePerm)
	}
	if werr == nil {
		werr = os.Rename(tmpPath, path)
	}
	if werr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing %s: %w", path, werr)
	}
	return nil
}
