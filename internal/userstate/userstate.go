// Package userstate stores per-user, per-project local state: which deployment
// the project points at, the preferred port, dev mode. It lives under the
// astro cache directory, keyed by project path hash (localrt.ProjectID), so
// worktrees and copies of a project each get their own state.
package userstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

const (
	stateFile = "state.json"
	// dirPerm is owner-only: per-user state should not be readable by
	// other users on the machine.
	dirPerm  = 0o700
	filePerm = 0o600
)

// State is what astro remembers about a project for this user. It is not
// committed and not shared; everything here is rebuildable or re-choosable,
// which is why it lives under the cache directory.
type State struct {
	// Instance is the pin `astro use` writes: the name of a manifest deployment
	// link. It is one layer of deployment resolution, below the --deployment
	// flag and ASTRO_DEPLOYMENT and above the manifest's default link
	// (docs/v2-instances.md). The field keeps its older name so state written
	// by an earlier build still reads.
	Instance string `json:"instance,omitempty"`
	// Port is the preferred webserver port. The runtime may pick another;
	// this is the request, not the fact.
	Port int `json:"port,omitempty"`
	// DevMode records whether dev-mode Airflow overrides are on.
	DevMode bool `json:"devMode,omitempty"`
}

// The pin was called `deployment` while an Astro Deployment was the only thing
// a project could point at. Both halves of the rename handle both names —
// UnmarshalJSON falls back to the old one, MarshalJSON writes both — so a
// machine that flips between a build from before the rename and one from after
// keeps its pin either way.
//
// Drop the fallback and the dual write together at cutover, when no build in
// the field reads the old name (docs/v2-release.md, D2).
//
// UnmarshalJSON decodes a state file, taking the pin from the older name when
// the current one is absent.
func (s *State) UnmarshalJSON(data []byte) error {
	type stateJSON State // sheds this method, so Unmarshal does not recurse
	aux := struct {
		*stateJSON
		Deployment string `json:"deployment"`
	}{stateJSON: (*stateJSON)(s)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if s.Instance == "" {
		s.Instance = aux.Deployment
	}
	return nil
}

// MarshalJSON writes the pin under both names, current and old — see
// UnmarshalJSON.
func (s State) MarshalJSON() ([]byte, error) {
	type stateJSON State // sheds this method, so Marshal does not recurse
	return json.Marshal(struct {
		stateJSON
		Deployment string `json:"deployment,omitempty"`
	}{stateJSON: stateJSON(s), Deployment: s.Instance})
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
// set, otherwise ~/.cache/astro. It is a thin wrapper over
// localrt.CacheRoot, which owns state locations (docs/v2-architecture.md).
func CacheRoot() (string, error) {
	return localrt.CacheRoot()
}

// Dir returns the state directory for a project,
// <cache>/projects/<path-hash>. It does not create the directory. It is a
// thin wrapper over localrt.StateDir, which owns state locations.
func Dir(projectPath string) (string, error) {
	return localrt.StateDir(projectPath)
}

// Load reads the project's state. A missing file is an empty State, not an
// error. After parsing, the state is normalized, and when the normalized
// encoding differs from what was on disk (older schema, dropped fields,
// hand edits) the file is rewritten — so the file on disk usually matches what
// this build would write, and always parses to what the caller gets back.
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
		if err := fsatomic.WriteFile(path, canonical, filePerm); err != nil {
			// Healing is best effort: the state parsed fine, and a cache
			// directory that cannot be written should not fail a command that
			// only wanted to read the pin. The file stays as it is and heals
			// the next time it can.
			return s, nil
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
	return fsatomic.WriteFile(filepath.Join(dir, stateFile), data, filePerm)
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
