// Package localstate owns the on-disk record of a running local Airflow:
// one runtime.json under rt.StateDir(projectPath), next to (never
// inside) userstate's state.json of per-user preferences. The record is how
// tools coordinate through disk (docs/v2-architecture.md, "Local state"):
// any process can discover, inspect, or stop an Airflow another one
// started. Both engines — docker and standalone —
// read and write this same record; mode-specific behavior stays in the
// engines.
package localstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

const (
	recordFile = "runtime.json"
	// dirPerm/filePerm are owner-only, matching userstate: runtime state
	// should not be readable by other users on the machine.
	dirPerm  = 0o700
	filePerm = 0o600
)

// ErrNotRunning reports that a project has no runtime record on disk.
var ErrNotRunning = errors.New("no local Airflow is recorded for this project")

// Record describes one running local Airflow. Exactly one of PID or
// ComposeProject identifies the runtime, by mode: standalone records the
// supervisor process, docker records the compose project name. A docker
// record additionally carries the starter's PID when StopWithSession is
// true, so other tools can tell the owning session has ended (containers
// outlive their starter; a bare compose project name cannot show that).
type Record struct {
	// ProjectPath is the absolute project root, symlinks intact. The state
	// directory is keyed by the resolved path's hash, which cannot be
	// inverted, so List needs the path stored here.
	ProjectPath string  `json:"projectPath"`
	Mode        rt.Mode `json:"mode"`
	PID         int     `json:"pid,omitempty"`
	// Pgid is the process group of a standalone Airflow (the leader is
	// PID). Stop signals and liveness checks address the group, not the
	// master: `airflow standalone` spawns its components into the group
	// and the master often exits before they do.
	Pgid           int    `json:"pgid,omitempty"`
	ComposeProject string `json:"composeProject,omitempty"`
	Port           int    `json:"port"`
	Hostname       string `json:"hostname,omitempty"`
	// AirflowMajor is the Airflow generation this runtime runs ("2" or "3"),
	// recorded at start. It is here rather than re-read from the manifest
	// because it is a fact about the process: the pin can be edited while
	// Airflow keeps running, and anything that talks to that Airflow — the
	// health probe's paths, the credentials it accepts — must follow the
	// process. Empty on a record written before this field existed.
	AirflowMajor    string    `json:"airflowMajor,omitempty"`
	StartedAt       time.Time `json:"startedAt"`
	StopWithSession bool      `json:"stopWithSession,omitempty"`
}

// GroupID is the process group to address for a standalone record: Pgid when
// the record carries one, otherwise PID, which leads its own group.
//
// This is the single home for that fallback. It used to be spelled out at each
// of three readers — the prune predicate, the engine's liveness probe, and its
// stop path — which meant a change to the rule had to land in three places, and
// a miss would show up as a record one code path calls alive and another calls
// dead.
//
// Returns 0 whenever there is nothing addressable, which is what lets those
// callers treat non-positive as "no group". A NEGATIVE Pgid returns 0 rather
// than falling back to PID: it means the writer already negated for
// kill(-pgid, 0) and stored the argument instead of the group, and quietly
// substituting PID would turn that mistake into a record that looks healthy
// while naming a different group than the writer intended. Docker records have
// no group at all; their liveness is compose state, not a signal.
func (r Record) GroupID() int {
	if r.Pgid > 0 {
		return r.Pgid
	}
	if r.Pgid < 0 {
		return 0
	}
	if r.PID > 0 {
		return r.PID
	}
	return 0
}

// Save writes the record for its ProjectPath, creating the state directory
// as needed.
func Save(rec Record) error {
	dir, err := rt.StateDir(rec.ProjectPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding runtime record: %w", err)
	}
	return fsatomic.WriteFile(filepath.Join(dir, recordFile), append(data, '\n'), filePerm)
}

// Load reads a project's record. A missing record is ErrNotRunning.
func Load(projectPath string) (Record, error) {
	dir, err := rt.StateDir(projectPath)
	if err != nil {
		return Record{}, err
	}
	return loadFrom(filepath.Join(dir, recordFile))
}

func loadFrom(path string) (Record, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, ErrNotRunning
	}
	if err != nil {
		return Record{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Record{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rec, nil
}

// Remove deletes a project's record. Removing an absent record is not an
// error, so stop paths stay idempotent.
func Remove(projectPath string) error {
	dir, err := rt.StateDir(projectPath)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, recordFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// List returns every runtime record on this machine, by scanning the
// per-project state directories under the cache root. Unparseable records
// are skipped: a corrupt file must not hide every other project.
func List() ([]Record, error) {
	root, err := rt.CacheRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "projects"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning project state: %w", err)
	}
	var recs []Record
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rec, err := loadFrom(filepath.Join(root, "projects", e.Name(), recordFile))
		if err != nil {
			continue
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// Status converts a record plus a liveness fact into the rt.Status
// callers render. The caller supplies running because liveness is
// mode-specific: docker asks the container engine, standalone checks the
// PID.
func (r Record) Status(running bool) rt.Status {
	st := rt.Status{
		ProjectPath:     r.ProjectPath,
		Mode:            r.Mode,
		StopWithSession: r.StopWithSession,
		State:           rt.StateStopped,
		Hostname:        r.Hostname,
		AirflowMajor:    r.AirflowMajor,
		StartedAt:       r.StartedAt,
	}
	if running {
		st.State = rt.StateRunning
		// PID and Port describe a live runtime; on a stopped record they are
		// stale (the process is gone, the port reassignable), so report them
		// only while running.
		st.PID = r.PID
		st.Port = r.Port
	}
	return st
}
