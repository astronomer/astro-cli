package uv

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A canceled invocation is an interruption, not a failure. interruptedError
// says why that distinction earns a type; this is the behavior.
func TestACanceledInvocationReportsAnInterruption(t *testing.T) {
	// A uv that never finishes, so the cancel is what ends it.
	c := newTestClient(t, Options{}, "sleep 60\nexit 0")
	// Without this the case spends defaultWaitDelay — ten seconds — waiting
	// on the pipes the sleeping child still holds open, for a result it
	// already has.
	c.waitDelay = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	err := c.Sync(ctx, t.TempDir(), "", Stdio{})
	if err == nil {
		t.Fatal("Sync() = nil, want the cancellation")
	}

	// The words a reader gets.
	if got := err.Error(); !strings.Contains(got, "interrupted") {
		t.Errorf("Error() = %q, want it to say the run was interrupted", got)
	}
	if got := err.Error(); strings.Contains(got, "failed") {
		t.Errorf("Error() = %q, still reports a failure that did not happen", got)
	}

	// And the identity a caller branches on. The engine above this one decides
	// whether to retry a sync by asking exactly this, so the message must not
	// cost the answer.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Error() = %v, want errors.Is(err, context.Canceled)", err)
	}

	// The command's own error is still reachable. An earlier revision dropped
	// it, on the reasoning that there is nothing to diagnose in a run somebody
	// stopped — which is true of the message and false of the value. A cancel
	// can land while uv is in the middle of a real failure, and
	// cmd/local/preflight.go reaches through this error with errors.As for a
	// *ResolutionError to build a constraint conflict. What changes is what is
	// rendered, not what is carried.
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Error("the invocation's own error is gone, so a caller cannot reach its exit code or stderr")
	}
}

// A deadline is not a keystroke.
//
// One is the caller's own bound being reached and the other is somebody
// pressing Ctrl-C. "interrupted" is only true of the second, and an embedder
// that wraps a sync in context.WithTimeout — which is what pkg/uv exists for —
// should be told which happened.
func TestADeadlineIsReportedAsATimeoutNotAnInterruption(t *testing.T) {
	c := newTestClient(t, Options{}, "sleep 60\nexit 0")
	c.waitDelay = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	err := c.Sync(ctx, t.TempDir(), "", Stdio{})
	if err == nil {
		t.Fatal("Sync() = nil, want the deadline")
	}
	if got := err.Error(); !strings.Contains(got, "ran out of time") {
		t.Errorf("Error() = %q, want it to report a deadline", got)
	}
	if got := err.Error(); strings.Contains(got, "interrupted") {
		t.Errorf("Error() = %q, reports a deadline as something a person did", got)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Error() = %v, want errors.Is(err, context.DeadlineExceeded)", err)
	}
}
