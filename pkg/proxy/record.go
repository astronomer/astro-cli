package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A running proxy publishes what it is and where it listens, so a tool that did
// not start it can find it instead of assuming a port. Both the CLI's daemon
// and the desktop's in-process proxy write one.
//
// The format lives here rather than beside either writer because it is a wire
// contract between them: "<pid> <version> <port>", one line, space separated.
// Two spellings of it is how one tool comes to read a record the other wrote as
// a port that is not there.

// missingVersion stands in for an empty version, so the port stays in field
// three for every reader. A record written by a build with no version stamp is
// still parseable by one that expects three fields.
const missingVersion = "-"

// Record is what a running proxy publishes about itself.
//
// Port is the port it actually BOUND, which is not always the one it was asked
// for: a proxy whose preferred port is taken falls back to an OS-assigned one,
// and that is the case this record exists to make discoverable.
type Record struct {
	PID     int
	Version string
	Port    string
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
	return writeFileAtomic(path, []byte(fmt.Sprintf("%d %s %s", r.PID, ver, r.Port)))
}

// writeFileAtomic publishes data at path via a temp file and a rename, so a
// reader sees either the previous content or the new one and never a partial
// write. The explicit chmod is because CreateTemp's 0o600 is not what a
// pre-existing file's mode would have been.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmpPath, FilePermRW)
	}
	if werr == nil {
		werr = renameOver(tmpPath, path)
	}
	if werr != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup of our own temp
		return fmt.Errorf("writing %s: %w", path, werr)
	}
	return nil
}

// renameRetries and renameRetryWait bound the waits below, on both sides of the
// same contention: a publish that cannot replace a record someone is reading,
// and a read that cannot open one being replaced. Either window is the length
// of one small file operation, so a few milliseconds covers it.
const (
	renameRetries   = 20
	renameRetryWait = 2 * time.Millisecond
)

// readFileContended reads path, retrying briefly while it cannot be opened for
// a reason other than not being there.
//
// The mirror of renameOver's problem. On Windows a file being renamed onto
// cannot be opened at that instant — "used by another process" — so a reader
// arriving during a publish fails for a reason that is neither "no proxy is
// running" nor anything about the record.
//
// A missing file is NOT retried: that is the ordinary answer when nothing has
// published, and waiting on it would turn the common case into a delay.
func readFileContended(path string) ([]byte, error) {
	var err error
	for i := 0; i < renameRetries; i++ {
		var data []byte
		data, err = os.ReadFile(path) //nolint:gosec // G304: the caller owns this path; it is a well-known state file
		if err == nil || os.IsNotExist(err) {
			return data, err
		}
		time.Sleep(renameRetryWait)
	}
	return nil, err
}

// renameOver replaces path with tmpPath, retrying briefly.
//
// On Windows a rename onto a file another process has OPEN fails outright —
// POSIX replaces it, Windows refuses — so a publish landing while someone reads
// the record errors for a reason that has nothing to do with either. Nothing
// holds a lock here by design, so the reader is expected and the collision is
// ordinary rather than exceptional.
//
// A bounded retry rather than a lock: the contended window is one small read,
// and a writer that gives up after it leaves the previous record in place,
// which is a correct outcome rather than a corrupt one.
func renameOver(tmpPath, path string) error {
	var err error
	for i := 0; i < renameRetries; i++ {
		if err = os.Rename(tmpPath, path); err == nil {
			return nil
		}
		time.Sleep(renameRetryWait)
	}
	return err
}

// ReadRecord parses the record at path.
//
// Version and port may be absent — a record written by an older proxy has
// fewer fields — and are returned empty rather than as an error, because a
// reader that can still learn the PID should not be denied it.
func ReadRecord(path string) (Record, error) {
	data, err := readFileContended(path)
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
