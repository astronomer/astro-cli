package cliout

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stdoutWriteError(errno error) error {
	return &fs.PathError{Op: "write", Path: os.Stdout.Name(), Err: errno}
}

func TestClosedStdoutIsOnlyAWriteToStdoutWhoseReaderWentAway(t *testing.T) {
	assert.True(t, ClosedStdout(stdoutWriteError(syscall.EPIPE)))
	assert.True(t, ClosedStdout(fmt.Errorf("list: %w", stdoutWriteError(syscall.EPIPE))), "wrapped")

	assert.False(t, ClosedStdout(nil))
	assert.False(t, ClosedStdout(errors.New("write /dev/stdout: broken pipe")), "text is not a type")
	assert.False(t, ClosedStdout(stdoutWriteError(syscall.ENOSPC)), "another write failure")
	// A connection that broke while a request was being sent is a failure
	// whose outcome the user must see, not a reader that went away.
	assert.False(t, ClosedStdout(&net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)}))
}

func TestAClosedStdoutExitsZeroAndReportsNothing(t *testing.T) {
	err := stdoutWriteError(syscall.EPIPE)
	assert.Equal(t, 0, ExitCode(context.Background(), err))
	// What decides the code is the error: a failure of another kind keeps its own.
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), errBoom))
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), stdoutWriteError(syscall.ENOSPC)))
	assert.Equal(t, 7, ExitCode(context.Background(), &ExitError{Code: 7}), "a command's own code wins")

	// An interrupted run is still an interrupted run.
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(&Signaled{Name: "interrupt", Number: int(syscall.SIGINT)})
	assert.Equal(t, 130, ExitCode(ctx, err))

	for _, args := range [][]string{{"thing", "list"}, {"thing", "list", "-o", "json"}} {
		r := runTree(t, err, args...)
		require.Error(t, r.err)
		assert.Empty(t, r.stderr, args)
		assert.Empty(t, r.stdout, args)
	}
}

// A mutation that failed reports its own failure however stdout is.
func TestAFailureOtherThanAClosedStdoutIsStillReported(t *testing.T) {
	r := runTree(t, errBoom, "thing", "list")
	assert.Contains(t, r.stderr, "boom")
}

func runTree(t *testing.T, fail error, args ...string) run {
	t.Helper()
	return execTree(context.Background(), testTree(fail), args...)
}
