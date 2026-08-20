package localrt

import (
	"errors"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// ErrNotRunning reports that a project has no runtime record on disk.
var ErrNotRunning = localstate.ErrNotRunning

// RecordedStatus reports what a project's state record says, without checking
// whether any of it is still true.
//
// State is deliberately left at its zero value rather than filled in. Record.Status
// takes a `running bool` because liveness is mode-specific and only an engine can
// answer it; passing an unconditional true here would fabricate exactly the fact
// this function cannot know, and a stale record — one left by a crash, where no
// stop path ran — would report StateRunning with a dead PID and a port that may
// since have been reassigned. Callers that need the live answer use
// Runtime.ReadStatus, which probes.
//
// What IS trustworthy: ProjectPath, Mode, Port, Hostname, AirflowMajor,
// StopWithSession, StartedAt — the fields the record was written to carry.
//
// This exists because the record is the interop contract, and the readers that
// only want those fields (an env lister, an agent config writer) should not have to
// build an engine, a proxy daemon, and an image builder to read them.
func RecordedStatus(projectPath string) (Status, error) {
	rec, err := localstate.Load(projectPath)
	if err != nil {
		return Status{}, err
	}
	return recordedStatus(rec), nil
}

// RecordedList reports every project with a runtime record, from the records
// alone. Same trade as RecordedStatus: no probing, no Config, no State.
func RecordedList() ([]Status, error) {
	recs, err := localstate.List()
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(recs))
	for _, rec := range recs {
		out = append(out, recordedStatus(rec))
	}
	return out, nil
}

// recordedStatus projects a record without asserting liveness. It reads the
// fields directly rather than going through Record.Status, whose whole signature
// is the liveness question this function refuses to answer.
func recordedStatus(rec localstate.Record) Status {
	return Status{
		ProjectPath:     rec.ProjectPath,
		Mode:            rec.Mode,
		Port:            rec.Port,
		Hostname:        rec.Hostname,
		AirflowMajor:    rec.AirflowMajor,
		StopWithSession: rec.StopWithSession,
		StartedAt:       rec.StartedAt,
	}
}

// IsNotRunning reports whether err means "no record for this project", so callers
// can tell that apart from a real read failure without naming the engine's errors.
func IsNotRunning(err error) bool { return errors.Is(err, localstate.ErrNotRunning) }
