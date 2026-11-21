package cliout

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

type failingStdout struct{}

func (failingStdout) Write([]byte) (int, error) { return 0, stdoutWriteError(syscall.EPIPE) }

// Only a stdout PathError counts, found through any %w wrapping and through
// the renderer's own writers; a plain EPIPE says nothing about which pipe.
func TestOnlyAStdoutWriteErrorIsAClosedStdout(t *testing.T) {
	pe := stdoutWriteError(syscall.EPIPE)
	assert.True(t, ClosedStdout(pe))
	assert.True(t, ClosedStdout(fmt.Errorf("render: %w", pe)), "wrapped with %w")
	assert.False(t, ClosedStdout(fmt.Errorf("render: %v", pe)), "wrapped with %v the PathError is gone")

	bare := fmt.Errorf("render: %w", syscall.EPIPE)
	assert.False(t, ClosedStdout(bare), "a plain EPIPE")
	assert.Equal(t, ExitFailure, ExitCode(context.Background(), bare))
	assert.False(t, ClosedStdout(fmt.Errorf("upload: %w", &net.OpError{Op: "write", Err: syscall.EPIPE})), "a socket's")

	// what the renderer returns for a stdout nobody reads
	err := Renderer{Format: FormatText, Out: failingStdout{}}.Emit(struct{}{}, Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, "a line")
	}))
	assert.True(t, ClosedStdout(err), "%v", err)
	err = Renderer{Format: FormatJSON, Out: failingStdout{}}.Emit(map[string]string{"a": "b"}, nil)
	assert.True(t, ClosedStdout(err), "%v", err)
}

// Only an error that is itself the failed write counts: every other error is
// reported, and exits as it would, whatever happened to stdout.
func TestAClosedStdoutHidesNoOtherError(t *testing.T) {
	inFlight := fmt.Errorf("update deployment: %w", &httputil.OutcomeUnknownError{
		Method: "POST", Path: "/v1/organizations/o/deployments/dep1", Err: context.Canceled,
	})
	for name, err := range map[string]error{
		"a canceled context":                    context.Canceled,
		"an api error":                          errBoom,
		"an unknown outcome":                    inFlight,
		"a write in flight and a closed stdout": errors.Join(stdoutWriteError(syscall.EPIPE), inFlight),
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
	assert.Equal(t, 130, ExitCode(sig, stdoutWriteError(syscall.EPIPE)))
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
