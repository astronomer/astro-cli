package cliout

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// sawSIGPIPE makes the process look as if SIGPIPE had been raised, for the
// rest of the test.
func sawSIGPIPE(t *testing.T) {
	t.Helper()
	NoteSIGPIPE()
	t.Cleanup(func() { sigpipeSeen.Store(false) })
}

// A bare broken pipe says nothing about which pipe: it is stdout's only once
// SIGPIPE has been seen, which Go raises for fd 1 and 2 alone.
func TestABareBrokenPipeIsStdoutsOnlyOnceSIGPIPEWasSeen(t *testing.T) {
	bare := fmt.Errorf("render: %w", syscall.EPIPE)
	assert.False(t, ClosedStdout(bare))
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), bare))

	sawSIGPIPE(t)
	assert.True(t, ClosedStdout(bare))
	assert.Equal(t, 0, ExitCode(context.Background(), bare))
	assert.False(t, ClosedStdout(fmt.Errorf("upload: %w", &net.OpError{Op: "write", Err: syscall.EPIPE})), "a socket's is not")
}

// Only an error that is itself the failed write counts, whatever happened to
// stdout: every other error is reported, and exits as it would.
func TestAClosedStdoutHidesNoOtherError(t *testing.T) {
	sawSIGPIPE(t)
	inFlight := fmt.Errorf("update deployment: %w", &httputil.OutcomeUnknownError{
		Method: "POST", Path: "/v1/organizations/o/deployments/dep1", Err: context.Canceled,
	})
	for name, err := range map[string]error{
		"a canceled context": context.Canceled,
		"an api error":       errBoom,
		"an unknown outcome": inFlight,
		"a write that was in flight and a closed stdout": errors.Join(syscall.EPIPE, inFlight),
	} {
		assert.False(t, ClosedStdout(err), name)
		assert.Equal(t, ExitFailure, ExitCode(context.Background(), err), name)
		r := execTree(context.Background(), testTree(err), "thing", "list")
		assert.NotEmpty(t, r.stderr, name)
	}
	assert.Contains(t, execTree(context.Background(), testTree(inFlight), "thing", "list").stderr, "may have been applied")

	// the command's own code, and an interrupt, are never replaced
	assert.Equal(t, 7, ExitCode(context.Background(), &ExitError{Code: 7}))
	sig, cancel := context.WithCancelCause(context.Background())
	cancel(&Signaled{Name: "interrupt", Number: int(syscall.SIGINT)})
	assert.Equal(t, 130, ExitCode(sig, fmt.Errorf("x: %w", syscall.EPIPE)))
}

// (a) A read whose stdout reader left exits 0 and says nothing.
func TestAReadWithAClosedStdoutExitsZeroSilently(t *testing.T) {
	err := stdoutWriteError(syscall.EPIPE)
	assert.Equal(t, 0, ExitCode(context.Background(), err))
	for _, args := range [][]string{{"thing", "list"}, {"thing", "list", "-o", "json"}} {
		r := execTree(context.Background(), testTree(err), args...)
		assert.Empty(t, r.stderr, args)
		assert.Empty(t, r.stdout, args)
	}
}
