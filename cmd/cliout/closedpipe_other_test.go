//go:build !windows

package cliout

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An EPIPE from a pipe that is not stdout (a child's stdin once the child has
// exited) is that write's own failure: it keeps its error and exits non-zero.
func TestABrokenPipeThatIsNotStdoutKeepsItsError(t *testing.T) {
	cmd := exec.Command("true")
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	// Process.Wait, not Cmd.Wait: that one closes the pipe itself.
	_, err = cmd.Process.Wait()
	require.NoError(t, err)

	_, err = stdin.Write([]byte(strings.Repeat("x", 1<<16)))
	require.ErrorIs(t, err, syscall.EPIPE)

	assert.False(t, ClosedStdout(err))
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), err))
	r := execTree(context.Background(), testTree(err), "thing", "list")
	assert.Contains(t, r.stderr, "broken pipe")

	// the same through a pipe nobody reads
	rd, w, err := os.Pipe()
	require.NoError(t, err)
	rd.Close()
	defer w.Close()
	_, err = w.WriteString("x")
	require.ErrorIs(t, err, syscall.EPIPE)
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), err))
}
