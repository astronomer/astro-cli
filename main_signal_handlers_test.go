//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// Only this package's handler goes when the first signal lands. Others (a
// hidden prompt that restores terminal echo, the login flow, the proxy) keep
// theirs, and still get the signal.
func TestFirstSignalLeavesOtherHandlersInPlace(t *testing.T) {
	other := make(chan os.Signal, 4)
	signal.Notify(other, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(other) })

	ctx := signalContext()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not cancel the context")
	}
	// A second signal must still reach the other handler. Had the first one
	// reset every handler, this would end the test process instead.
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	for got := 0; got < 2; got++ {
		select {
		case <-other:
		case <-time.After(5 * time.Second):
			t.Fatalf("the other handler saw %d signals, want 2", got)
		}
	}
}
