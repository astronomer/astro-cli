//go:build !windows

package supervise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCappedWriterTruncatesPastMax drives the writer past maxLogSize and
// asserts the file is capped back to roughly keepLogSize and still starts on
// a line boundary. It opens the file O_RDWR because truncate() reads the tail
// back with ReadAt, which fails with EBADF on a write-only fd — the open-flag
// regression that let the log grow without bound.
func TestCappedWriterTruncatesPastMax(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "airflow.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	cw := newCappedWriter(f)
	// A ~1 KB line that starts with a marker, so a file left starting
	// mid-line (with filler) is distinguishable from one trimmed to a clean
	// line boundary (starting with the marker).
	line := append([]byte("START"+strings.Repeat("x", 1018)), '\n')
	for written := 0; written <= maxLogSize; written += len(line) {
		_, err := cw.Write(line)
		require.NoError(t, err)
	}

	info, err := f.Stat()
	require.NoError(t, err)
	assert.LessOrEqual(t, info.Size(), int64(maxLogSize), "file should be capped at or below maxLogSize")
	assert.Less(t, info.Size(), int64(keepLogSize)+int64(len(line)), "file should be trimmed to about keepLogSize")

	// After truncation the file must start on a line boundary, not mid-line.
	head := make([]byte, len(line))
	n, err := f.ReadAt(head, 0)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(head[:n]), "START"), "file should start on a line boundary after truncation")
}
