//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// breakStdout makes fd 1 a pipe with no reader for the rest of the test, as
// `astro ... | head -1` leaves it once head is done.
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

// A write to a closed stdout does not end the process by SIGPIPE: it returns
// the *fs.PathError that cliout reads as a closed stdout, wrapped or not, and
// the exit is 0. A failed command is still a failed command.
func TestAWriteToAClosedStdoutIsTheCommandsOwnError(t *testing.T) {
	keepRunningOnClosedStdout()
	breakStdout(t)

	_, err := os.Stdout.WriteString("a line\n")
	if !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("write to a closed stdout: %v, want EPIPE", err)
	}
	for _, e := range []error{err, fmt.Errorf("render: %w", err)} {
		if !cliout.ClosedStdout(e) {
			t.Errorf("%v is not read as a closed stdout", e)
		}
		if code := cliout.ExitCode(context.Background(), e); code != 0 {
			t.Errorf("exit %d for a write that failed on a closed stdout, want 0", code)
		}
	}
	if code := cliout.ExitCode(context.Background(), errors.New("deployment not found")); code != cliout.ExitFailure {
		t.Errorf("exit %d for a failed command, want %d", code, cliout.ExitFailure)
	}
}
