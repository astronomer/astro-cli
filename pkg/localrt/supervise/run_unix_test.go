//go:build !windows

package supervise

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRejectsBadUsage(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "airflow.log")

	// No command is the only usage error left. A missing log file used to be one
	// too; it is now a supported mode — see TestRunWithoutLogFileInheritsStdio.
	err := Run([]string{LogFileFlag, logPath})
	assert.ErrorContains(t, err, "usage")

	err = Run([]string{ParentPIDFlag, "1"})
	assert.ErrorContains(t, err, "usage")
}

// Without a log file the child inherits the supervisor's stdout and stderr, so a
// spawner that already owns the log keeps owning it. Astro Desktop depends on this:
// it hands the child a log file descriptor rather than a pipe, so the kernel writes
// and the log outlives the app, and it caps that file from the app instead. A
// supervisor-owned file would truncate it and cap it from a process that dies with
// the app.
//
// Not parallel: it swaps the process's own stdout, which is global.
func TestRunWithoutLogFileInheritsStdio(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	runErr := Run([]string{"--", "/bin/sh", "-c", "echo marker-out"})
	require.NoError(t, w.Close())
	require.NoError(t, runErr)

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(out), "marker-out", "the child's stdout must reach the spawner's, not a file")
}

func TestRunCapturesChildOutputInLogFile(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "airflow.log")

	err := Run([]string{LogFileFlag, logPath, "--", "/bin/sh", "-c", "echo out; echo err 1>&2"})
	require.NoError(t, err)

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "out")
	assert.Contains(t, string(data), "err")
}

func TestRunReportsChildFailure(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "airflow.log")

	err := Run([]string{LogFileFlag, logPath, "--", "/bin/sh", "-c", "exit 3"})
	assert.Error(t, err)
}

// The parent-death path (waitForProcessExit firing and the pgid SIGTERM)
// is not unit-testable in-process: the supervisor signals its own process
// group, which here would be the test runner's. The end-to-end smoke and
// the desktop lineage of this code cover it.
