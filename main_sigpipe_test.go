//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
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

// A stdout whose reader went away does not stop the run: SIGPIPE is recorded,
// and nothing is canceled, so a deploy piped into `head -1` still finishes.
// What ends a command is its own error, from the write that failed, and a
// bare broken pipe is then read as stdout's.
func TestClosedStdoutIsRecordedAndCancelsNothing(t *testing.T) {
	ctx := signalContext()
	bare := fmt.Errorf("render: %w", syscall.EPIPE)
	if cliout.ClosedStdout(bare) {
		t.Skip("SIGPIPE was already seen by an earlier test")
	}

	breakStdout(t)
	if _, err := os.Stdout.WriteString("a line\n"); !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("write to a closed stdout: %v, want EPIPE", err)
	}

	deadline := time.After(5 * time.Second)
	for !cliout.ClosedStdout(bare) {
		select {
		case <-deadline:
			t.Fatal("SIGPIPE was not recorded")
		case <-time.After(10 * time.Millisecond):
		}
	}
	select {
	case <-ctx.Done():
		t.Fatalf("a closed stdout canceled the run: %v", context.Cause(ctx))
	case <-time.After(300 * time.Millisecond):
	}
	// The command's own failure is not hidden by it.
	if code := cliout.ExitCode(ctx, errors.New("deployment not found")); code != cliout.ExitFailure {
		t.Fatalf("exit %d for a failed command, want %d", code, cliout.ExitFailure)
	}
	if code := cliout.ExitCode(ctx, bare); code != 0 {
		t.Fatalf("exit %d for a write that failed on a closed stdout, want 0", code)
	}
}
