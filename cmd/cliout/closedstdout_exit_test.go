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

// A bare broken pipe says nothing about which pipe: it is stdout's only once
// SIGPIPE has canceled the run with ErrClosedStdout, which Go raises for fd 1
// and 2 alone.
func TestABareBrokenPipeIsStdoutsOnlyOnceSIGPIPESaidSo(t *testing.T) {
	bare := fmt.Errorf("render: %w", syscall.EPIPE)
	assert.False(t, ClosedStdout(bare))
	assert.False(t, ClosedStdout(fmt.Errorf("upload: %w", &net.OpError{Op: "write", Err: syscall.EPIPE})))
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), bare))
	assert.Equal(t, 0, ExitCode(cancelledByClosedStdout(), bare))
}

func cancelledByClosedStdout() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrClosedStdout)
	return ctx
}

// (a) A read whose stdout reader left exits 0, whether the run saw the write
// fail or the cancellation, and says nothing.
func TestAReadWithAClosedStdoutExitsZero(t *testing.T) {
	assert.Equal(t, 0, ExitCode(context.Background(), stdoutWriteError(syscall.EPIPE)))
	assert.Equal(t, 0, ExitCode(cancelledByClosedStdout(), context.Canceled))
	assert.Equal(t, 0, ExitCode(cancelledByClosedStdout(), nil))
	assert.True(t, EndedByClosedStdout(cancelledByClosedStdout(), context.Canceled))
	r := execTree(cancelledByClosedStdout(), testTree(context.Canceled), "thing", "list")
	assert.Empty(t, r.stderr)
}

// (b) A write was in flight when the reader left: the error stays the
// outcome, reported as usual, and the exit is 1.
func TestAClosedStdoutDuringAWriteIsAnUnknownOutcome(t *testing.T) {
	inFlight := fmt.Errorf("update deployment: %w", &httputil.OutcomeUnknownError{
		Method: "POST", Path: "/v1/organizations/o/deployments/dep1", Err: context.Canceled,
	})
	ctx := cancelledByClosedStdout()
	assert.Equal(t, ExitFailure, ExitCode(ctx, inFlight))
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), errors.Join(syscall.EPIPE, inFlight)))
	assert.False(t, EndedByClosedStdout(ctx, inFlight))

	r := execTree(ctx, testTree(inFlight), "thing", "list")
	assert.Contains(t, r.stderr, "may have been applied")

	// an interrupt stays 128+n whatever else happened
	sig, cancel := context.WithCancelCause(context.Background())
	cancel(&Signaled{Name: "interrupt", Number: int(syscall.SIGINT)})
	assert.Equal(t, 130, ExitCode(sig, fmt.Errorf("x: %w", syscall.EPIPE)))
}
