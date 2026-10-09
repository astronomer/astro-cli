package proxy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// A running proxy publishes what it is and where it listens, so a tool that did
// not start it can find it instead of assuming a port. Both the CLI's daemon
// and the desktop's in-process proxy write one.
//
// The format lives here rather than beside either writer because it is a wire
// contract between them: "<pid> <version> <port> [<protocol>]", one line,
// space separated. The protocol is the daemon's (DaemonProtocol); a record
// that does not carry one is protocol 0.
// Two spellings of it is how one tool comes to read a record the other wrote as
// a port that is not there.

// missingVersion stands in for an empty version, so the port stays in field
// three for every reader. A record written by a build with no version stamp is
// still parseable by one that expects three fields.
const missingVersion = "-"

// protocolField is the protocol's index among the record's fields, after the
// pid, the version and the port.
const protocolField = 3

// Record is what a running proxy publishes about itself.
//
// Port is the port it actually BOUND, which is not always the one it was asked
// for: a proxy whose preferred port is taken falls back to an OS-assigned one,
// and that is the case this record exists to make discoverable.
type Record struct {
	PID     int
	Version string
	Port    string
	// Protocol is the DaemonProtocol of the daemon that wrote the record, or 0
	// for a record without one: written by a daemon from before protocols, or
	// by a proxy that is not a daemon (the desktop's in-process one).
	Protocol int
}

// WriteRecord publishes r at path.
//
// Fields are appended, never reordered: an older reader takes the first one or
// two and ignores the rest, which is what lets a field be added without
// coordinating a release.
//
// It refuses what ReadRecord would refuse, so a bad record is an error to
// whoever wrote it rather than a file every reader rejects and nobody owns.
// Whitespace is the subtle half: the fields are space separated, so a space
// inside one shifts every field after it, and a reader takes the tail of a
// version for the port.
//
// The write is atomic. A plain truncating write has a window in which a reader
// sees an empty file and concludes no proxy is running, and nothing here holds
// a lock against that — the desktop's writer takes none, and neither will a
// reader.
func WriteRecord(path string, r Record) error {
	if r.PID <= 0 {
		return fmt.Errorf("proxy record needs a PID, got %d", r.PID)
	}
	ver := r.Version
	if ver == "" {
		ver = missingVersion
	}
	if strings.ContainsAny(ver, " \t\n") || strings.ContainsAny(r.Port, " \t\n") {
		return fmt.Errorf("proxy record fields cannot contain whitespace: version %q, port %q", ver, r.Port)
	}
	if r.Protocol < 0 {
		return fmt.Errorf("proxy record protocol cannot be negative, got %d", r.Protocol)
	}
	line := fmt.Sprintf("%d %s %s", r.PID, ver, r.Port)
	if r.Protocol > 0 {
		// An empty port collapses field three, and the protocol would be read
		// back as the port.
		if r.Port == "" {
			return fmt.Errorf("proxy record with a protocol needs a port")
		}
		line += " " + strconv.Itoa(r.Protocol)
	}
	return fsatomic.WriteFile(path, []byte(line), FilePermRW)
}

// ReadRecord parses the record at path.
//
// Version, port and protocol may be absent — a record written by an older
// proxy has fewer fields — and are returned empty rather than as an error,
// because a reader that can still learn the PID should not be denied it. A
// protocol that is not a positive integer reads as 0, the oldest, which is
// the safe way to be wrong about it: the daemon is replaced rather than
// trusted.
func ReadRecord(path string) (Record, error) {
	data, err := fsatomic.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) == 0 {
		return Record{}, fmt.Errorf("empty proxy record %s", path)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return Record{}, fmt.Errorf("invalid PID in proxy record %s", path)
	}
	out := Record{PID: pid}
	if len(fields) > 1 && fields[1] != missingVersion {
		out.Version = fields[1]
	}
	if len(fields) > 2 {
		out.Port = fields[2]
	}
	if len(fields) > protocolField {
		if n, err := strconv.Atoi(fields[protocolField]); err == nil && n > 0 {
			out.Protocol = n
		}
	}
	return out, nil
}

// LiveRecord is the record at path when the process that wrote it is still
// alive, and false otherwise.
//
// A record outlives the proxy that wrote it — a killed process leaves the file
// behind — so a reader that trusts one without this check is reading a port
// nothing is listening on, or worse a PID some unrelated process has since
// recycled. Liveness is the cheap half of that; a caller that must be certain
// also probes the port.

func LiveRecord(path string) (Record, bool) {
	r, err := ReadRecord(path)
	if err != nil || !IsPIDAlive(r.PID) {
		return Record{}, false
	}
	return r, true
}
