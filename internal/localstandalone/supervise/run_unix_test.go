//go:build !windows

package supervise

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRejectsBadUsage(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "airflow.log")

	// No log file.
	err := Run([]string{"--", "/bin/true"})
	assert.ErrorContains(t, err, "usage")

	// No command.
	err = Run([]string{LogFileFlag, logPath})
	assert.ErrorContains(t, err, "usage")
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
