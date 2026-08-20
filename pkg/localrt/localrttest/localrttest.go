// Package localrttest seeds runtime state records for tests.
//
// Writing a record is the engines' job, so pkg/localrt does not expose a writer —
// a consumer that could write one could also lie about what is running. But a test
// for anything that READS the record has to get one onto disk somehow, and both
// consumers have such tests.
//
// Hence a separate package, in the httptest tradition: importing it in production
// code is visibly wrong, while a _test.go file that needs a fixture has a
// supported way to build one instead of hand-rolling the JSON and drifting from
// the real format.
package localrttest

import (
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// Record is the subset of a runtime state record a test needs to set. Zero values
// are fine: a record with only ProjectPath is a valid "something is recorded here".
type Record struct {
	ProjectPath string
	Mode        localrt.Mode
	PID         int
	// Pgid is a standalone Airflow's process group; liveness addresses the
	// group rather than the leader, so a test about liveness has to set it.
	Pgid            int
	Port            int
	Hostname        string
	AirflowMajor    string
	ComposeProject  string
	StopWithSession bool
	StartedAt       time.Time
}

// Seed writes r as the runtime state record for its project, creating the state
// directory if needed. It writes through the same code the engines use, so a
// change to the record format cannot leave the fixtures behind.
func Seed(r Record) error {
	return localstate.Save(localstate.Record{
		ProjectPath:     r.ProjectPath,
		Mode:            r.Mode,
		PID:             r.PID,
		Pgid:            r.Pgid,
		Port:            r.Port,
		Hostname:        r.Hostname,
		AirflowMajor:    r.AirflowMajor,
		ComposeProject:  r.ComposeProject,
		StopWithSession: r.StopWithSession,
		StartedAt:       r.StartedAt,
	})
}
