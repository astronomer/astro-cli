package localdocker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// An interrupted health wait says what it left, the way the timeout beside it
// already did.
//
// Start publishes the record and the route before this wait and does not tear
// them down on the way out, so the containers keep coming up either way. The
// deadline branch has said so for a while; the cancel branch returned the
// context's error bare, and `astro local start --docker` answered a Ctrl-C
// with "Error: context canceled" — the name of a Go value, to somebody who
// could not tell from it whether an Airflow was now running on their machine.
func TestAnInterruptedHealthWaitSaysWhatItLeft(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitHealthy(ctx, []string{"http://127.0.0.1:1/health"}, time.Minute)
	if err == nil {
		t.Fatal("waitHealthy() = nil, want the cancellation")
	}

	for _, want := range []string{"interrupted", "keep starting", "astro local stop"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Error() = %q, still names the mechanism rather than the outcome", err.Error())
	}

	// And the identity a caller branches on, which main.go's exit code and
	// every retry decision above this are keyed on.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Error() = %v, want errors.Is(err, context.Canceled)", err)
	}
}

// A deadline is not an interrupt: nobody asked for it, and the message that
// names the timeout is the one that explains it.
func TestATimedOutHealthWaitStillReportsTheTimeout(t *testing.T) {
	err := waitHealthy(context.Background(), []string{"http://127.0.0.1:1/health"}, 10*time.Millisecond)
	if !errors.Is(err, ErrHealthTimeout) {
		t.Fatalf("waitHealthy() = %v, want ErrHealthTimeout", err)
	}
	if strings.Contains(err.Error(), "interrupted") {
		t.Errorf("Error() = %q, reports a deadline as something a person did", err.Error())
	}
}
