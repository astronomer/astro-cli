// Package localrttest seeds runtime state records for tests.
//
// This doc used to say that pkg/localrt exposes no writer, because "a consumer
// that could write one could also lie about what is running". The first half
// stopped being true when Reservation.Claim landed; the second half was right,
// and is why that API validates rather than trusting. A claim must name a
// process group that is actually alive, must not overwrite another mode's
// record, and cannot be released by anything but its own pid — each of those
// checks exists because a record is a claim about the world that readers cannot
// re-verify cheaply.
//
// So the distinction this package rests on is no longer writer versus no writer.
// It is that Claim publishes a runtime somebody is really supervising, while
// Seed fabricates one for a test. A test that wants a fixture should not have to
// stand up a live process group to get it, and production code should never
// reach for something that skips those checks.
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
