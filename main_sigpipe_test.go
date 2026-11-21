//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// breakStdout makes fd 1 a pipe with no reader for the rest of the test, as
// `astro ... | head -1` leaves it once head is done. Go raises SIGPIPE only for
// a write to fd 1 or 2, so the test has to write to the real descriptor.
func breakStdout(t *testing.T) {
	t.Helper()
	saved, err := syscall.Dup(1)
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	if err := dup2(int(w.Fd()), 1); err != nil {
		t.Fatal(err)
	}
	w.Close()
	t.Cleanup(func() {
		_ = dup2(saved, 1)
		_ = syscall.Close(saved)
	})
}

// A stdout whose reader went away cancels the run, with a cause that is not a
// signal's: a ctx-aware wait or poll stops at once, and the exit is not 141.
func TestClosedStdoutCancelsTheRunPromptly(t *testing.T) {
	ctx := signalContext()

	waited := make(chan error, 1)
	go func() {
		select { // stands in for a poll/wait loop that watches the context
		case <-ctx.Done():
			waited <- ctx.Err()
		case <-time.After(30 * time.Second):
			waited <- errors.New("kept waiting")
		}
	}()
	breakStdout(t)
	_, err := os.Stdout.WriteString("a line\n")
	if !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("write to a closed stdout: %v, want EPIPE", err)
	}

	select {
	case err := <-waited:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait ended with %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a closed stdout did not cancel the context")
	}
	if cause := context.Cause(ctx); !errors.Is(cause, cliout.ErrClosedStdout) {
		t.Fatalf("cause = %v, want ErrClosedStdout", cause)
	}
	if code := cliout.ExitCode(ctx, context.Canceled); code != 0 {
		t.Fatalf("exit %d for a listing whose reader left, want 0", code)
	}
}

// A SIGPIPE that is not stdout's (a closed stderr raises it too) cancels
// nothing.
func TestASIGPIPEThatIsNotStdoutsCancelsNothing(t *testing.T) {
	prev := stdoutGone.Load()
	stdoutGone.Store(func() bool { return false })
	t.Cleanup(func() { stdoutGone.Store(prev) })
	ctx := signalContext()
	breakStdout(t) // the signal is real; the verdict is stubbed
	_, _ = os.Stdout.WriteString("x")
	select {
	case <-ctx.Done():
		t.Fatal("a SIGPIPE that is not stdout's canceled the run")
	case <-time.After(300 * time.Millisecond):
	}
}
