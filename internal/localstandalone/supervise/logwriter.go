//go:build !windows

package supervise

import (
	"io"
	"os"
	"sync"
)

// Lifted from Astro Desktop's runtime/logwriter.go.

const (
	maxLogSize  = 50 * 1024 * 1024 // 50 MB
	keepLogSize = 25 * 1024 * 1024 // keep tail 25 MB after truncation
)

// cappedWriter wraps an *os.File and truncates it when it exceeds
// maxLogSize, keeping the tail keepLogSize bytes, so log files cannot grow
// unbounded during long-running Airflow sessions.
type cappedWriter struct {
	mu      sync.Mutex
	f       *os.File
	written int64
}

func newCappedWriter(f *os.File) *cappedWriter {
	info, _ := f.Stat()
	var size int64
	if info != nil {
		size = info.Size()
	}
	return &cappedWriter{f: f, written: size}
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.f.Write(p)
	w.written += int64(n)

	if w.written > maxLogSize {
		w.truncate()
	}
	return n, err
}

// truncate keeps the tail keepLogSize bytes and rewrites the file.
func (w *cappedWriter) truncate() {
	tailStart := w.written - keepLogSize
	if tailStart < 0 {
		return
	}

	tail := make([]byte, keepLogSize)
	n, err := w.f.ReadAt(tail, tailStart)
	if err != nil && err != io.EOF {
		return
	}
	tail = tail[:n]

	// Drop through the first newline so the file never starts mid-line.
	for i, b := range tail {
		if b == '\n' {
			tail = tail[i+1:]
			break
		}
	}

	_ = w.f.Truncate(0)
	_, _ = w.f.Seek(0, io.SeekStart)
	written, _ := w.f.Write(tail)
	w.written = int64(written)
}
