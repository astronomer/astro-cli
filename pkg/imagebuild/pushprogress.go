package imagebuild

import (
	"bytes"
	"io"
	"sync"
)

// maxProgressLine bounds how much of a line redactingLines holds before
// writing it anyway, so output with no line end in it is not held without
// limit.
const maxProgressLine = 64 << 10

// redactingLines is the writer podman's stderr streams to Out through: it
// writes a line at a time, a line ending at '\r' (a progress update redrawn
// in place) or '\n', with each of secrets redacted from it.
//
// rt.LineWriter is not reused: it ends lines at '\n' alone, drops the line
// ends and empty lines, and holds a line without limit, and it is the
// OnLine contract other consumers read.
type redactingLines struct {
	w       io.Writer
	secrets []string
	// keep is how much a write forced by maxProgressLine leaves held: one
	// byte short of the longest secret, so a secret split across the cut
	// is still whole, and redacted, when the rest of it arrives.
	keep int
	buf  []byte
}

func newRedactingLines(w io.Writer, secrets []string) *redactingLines {
	keep := 0
	for _, s := range secrets {
		keep = max(keep, len(s)-1)
	}
	return &redactingLines{w: w, secrets: secrets, keep: keep}
}

// Write never fails: progress is best-effort, and an error would stop the
// stderr kept for the push's error along with it.
func (l *redactingLines) Write(b []byte) (int, error) {
	l.buf = append(l.buf, b...)
	for {
		i := bytes.IndexAny(l.buf, "\r\n")
		if i < 0 {
			break
		}
		l.write(redact(string(l.buf[:i+1]), l.secrets))
		l.buf = l.buf[i+1:]
	}
	if len(l.buf) >= maxProgressLine {
		held := redact(string(l.buf), l.secrets)
		if cut := len(held) - l.keep; cut > 0 {
			l.write(held[:cut])
			l.buf = []byte(held[cut:])
		}
	}
	return len(b), nil
}

// Flush writes what is held of a line not yet ended.
func (l *redactingLines) Flush() {
	if len(l.buf) > 0 {
		l.write(redact(string(l.buf), l.secrets))
		l.buf = nil
	}
}

func (l *redactingLines) write(s string) {
	io.WriteString(l.w, s) //nolint:errcheck // best-effort progress
}

// lockedWriter serializes writes to w, which a push's stdout and its
// streamed stderr share.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}
