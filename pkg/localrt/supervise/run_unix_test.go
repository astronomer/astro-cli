//go:build !windows

package supervise

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
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

// handoffFD returns an inheritable read descriptor whose pipe holds payload,
// standing in for the one the engine passes through ExtraFiles. Go's own
// descriptors are close-on-exec; a dup is not, so only the supervisor closing
// it keeps the child from inheriting it, which is the behavior under test.
func handoffFD(t *testing.T, payload string) int {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	fd, err := syscall.Dup(int(r.Fd()))
	require.NoError(t, err)
	require.NoError(t, r.Close())
	_, err = w.WriteString(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return fd
}

// The values on the descriptor reach the child's environment intact, a pair no
// environment can hold is dropped, and the descriptor itself is closed before
// the child starts.
//
// Not parallel: the descriptor is inheritable by any process the test binary
// starts meanwhile.
func TestRunHandsSecretEnvToTheChildOnly(t *testing.T) {
	value := `pa$$ 'w"o` + "\nrd"
	fd := handoffFD(t, `{"HANDOFF_TEST":`+strconv.Quote(value)+`,"BAD=KEY":"x"}`)

	logPath := filepath.Join(t.TempDir(), "airflow.log")
	script := `printf '[%s]' "$HANDOFF_TEST"; if (: <&` + strconv.Itoa(fd) + `) 2>/dev/null; then echo fd-leaked; fi`
	require.NoError(t, Run([]string{LogFileFlag, logPath, EnvFDFlag, strconv.Itoa(fd), "--", "/bin/sh", "-c", script}))

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "["+value+"]", "the value must reach the child byte for byte")
	assert.NotContains(t, string(data), "fd-leaked",
		"the supervisor must close the descriptor after reading it, or the child inherits it")
}

// A payload that cannot be read stops the start rather than launching Airflow
// without its values.
func TestRunRefusesAnUnreadableSecretEnv(t *testing.T) {
	fd := handoffFD(t, "not json")

	logPath := filepath.Join(t.TempDir(), "airflow.log")
	marker := filepath.Join(t.TempDir(), "ran")
	err := Run([]string{LogFileFlag, logPath, EnvFDFlag, strconv.Itoa(fd), "--", "/bin/sh", "-c", "touch " + marker})
	require.ErrorContains(t, err, "reading secret values")
	assert.NoFileExists(t, marker, "the child must not start without its values")
}
