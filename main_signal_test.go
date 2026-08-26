//go:build !windows

// Unix only: there is no portable way to deliver SIGINT to your own process on
// Windows, and syscall.Kill does not exist there. The behavior under test —
// cancel instead of exit — is what Ctrl-C produces on both, but only one of them
// can be provoked from inside a test.

package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

// An interrupt has to CANCEL the command rather than end the process, because
// the cleanup that matters runs on the way out.
//
// `astro local start` writes its state record only after `compose up` returns.
// A start killed in between leaves containers that no later `astro local stop`
// can find, since stop looks the project up through that record. pkg/localrt
// already tears those down when an up fails — under a context deliberately
// detached from the caller's, so a cancellation cannot turn the cleanup into a
// no-op. None of it was reachable from a Ctrl-C until the root context could be
// canceled by one.
func TestInterruptCancelsRatherThanExits(t *testing.T) {
	ctx := signalContext()

	select {
	case <-ctx.Done():
		t.Fatal("context was already canceled before any signal")
	default:
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("raise SIGINT: %v", err)
	}

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(5 * time.Second):
		// Reaching here means the process survived the signal without the
		// cancellation landing, which is the state this exists to prevent: the
		// command keeps running, or dies with no chance to clean up.
		t.Fatal("SIGINT did not cancel the context")
	}
}
