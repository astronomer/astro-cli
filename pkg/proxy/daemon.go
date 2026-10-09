package proxy

import "path/filepath"

const (
	daemonRecordName = "proxy.pid"
	daemonLogName    = "proxy.log"
)

// Daemon is the proxy run as a background process of its own: one per route
// store, started by whichever tool needs it first and adopted by the rest.
//
// It lives here rather than in either tool because both have to agree on all
// of it — which files name a running daemon, when one is reused, how it is
// stopped. A host supplies only what differs between them: the executable that
// serves (the CLI re-execs itself; another host points at the astro binary it
// ships), and anything it must do before a start.
type Daemon struct {
	// Store is the route store the daemon serves. Its directory also holds the
	// daemon's own files: the record, the bound port and the log.
	Store *Store

	// Exe and ServeArgs are what Start runs, with "--port <port>" appended:
	// for the CLI, its own binary and its hidden serve subcommand. ServeArgs
	// also identifies a daemon by its command line when its port cannot be
	// asked (see processLooksLikeProxy).
	Exe       string
	ServeArgs []string

	// Version is the host's version, written into the record for whoever reads
	// it next. It does not decide reuse; DaemonProtocol does.
	Version string

	// BeforeStart runs under the routes lock just before a start, with the
	// port about to be bound. The CLI stops an astro 1.x proxy holding that
	// port here, so the daemon serves there rather than on a fallback. Nil
	// does nothing.
	BeforeStart func(port string)

	// Logf receives debug messages about the daemon's lifecycle. Nil discards
	// them.
	Logf func(format string, args ...any)
}

// RecordPath is where the running daemon's record lives (proxy.pid).
func (d *Daemon) RecordPath() string { return filepath.Join(d.Store.Dir(), daemonRecordName) }

// LogPath is where the daemon's output goes (proxy.log).
func (d *Daemon) LogPath() string { return filepath.Join(d.Store.Dir(), daemonLogName) }

// readRecord reads this daemon's record.
//
// The format is Record's, because it is a contract between this daemon and
// whoever else needs to find a running proxy — the desktop publishes a record
// of its own in the same shape. Parsing it anywhere else as well is how two
// readers come to disagree about a field.
//
// It returns the record rather than its fields: unpacking them here only to
// thread them through every caller means each field the contract gains costs
// another return value and an edit at each one.
func (d *Daemon) readRecord() (Record, error) {
	return ReadRecord(d.RecordPath())
}

// IsRunning reports the PID the record names and whether that process is
// alive.
func (d *Daemon) IsRunning() (int, bool) {
	r, err := d.readRecord()
	if err != nil {
		return 0, false
	}
	// Not LiveRecord: this reports the PID it found even when that process is
	// gone, which is what a caller cleaning up a stale record needs.
	return r.PID, IsPIDAlive(r.PID)
}

// BoundPort returns the port the running daemon reported at bind time, or ""
// when unknown (daemon not running, or started by a build that didn't record
// it).
func (d *Daemon) BoundPort() string {
	r, ok := LiveRecord(d.RecordPath())
	if !ok {
		return ""
	}
	return r.Port
}
