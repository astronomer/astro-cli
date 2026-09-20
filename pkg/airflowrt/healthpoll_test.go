package airflowrt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A wait shorter than the poll interval still asks.
//
// The loop's first answer used to arrive a whole interval in, so any timeout
// under a second expired having issued no request at all: a caller asking to
// fail fast was told Airflow had not come up without Airflow ever being
// asked. Invisible while the wait was a hardcoded five minutes, reachable the
// moment a caller could choose it.
//
// At exactly one second the deadline and the tick were both ready and Go
// picked between them at random, so the same call could pass or fail on the
// same healthy server.
func TestAHealthCheckShorterThanThePollIntervalStillAsks(t *testing.T) {
	var asked atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Well under the one-second poll interval, and enough for a loopback
	// round trip. Not a millisecond: the context expires during the dial, so
	// the request never reaches the server, and that is the timeout doing what
	// it was asked rather than the loop failing to ask.
	for _, timeout := range []time.Duration{100 * time.Millisecond, 500 * time.Millisecond} {
		asked.Store(0)
		err := CheckHealth(context.Background(), portOf(srv.Listener.Addr().String()), timeout, HealthCheckConfig{})
		if err != nil {
			t.Errorf("CheckHealth(timeout=%s) = %v, want nil against a server answering 200", timeout, err)
		}
		if n := asked.Load(); n == 0 {
			t.Errorf("CheckHealth(timeout=%s) issued no request at all", timeout)
		}
	}
}

// A cancellation is reported as a cancellation.
//
// CheckHealth hangs its own timeout on the caller's context, so ctx.Done()
// fires for either reason and the branch reported both as the timeout. Someone
// who pressed Ctrl-C four seconds into a start was told the health check had
// timed out after five minutes — and, before the advice moved to the CLI, told
// to raise a timeout that had never been reached.
func TestACanceledHealthCheckIsNotReportedAsATimeout(t *testing.T) {
	// Never healthy, so the only ways out are the cancel and the timeout, and
	// the timeout is far enough away that reaching it would be the bug. An
	// empty handler will not do: net/http writes a 200 for one, which is
	// exactly what CheckHealth is looking for.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := CheckHealth(ctx, portOf(srv.Listener.Addr().String()), time.Hour, HealthCheckConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("CheckHealth() = %v, want a context.Canceled", err)
	}
	if errors.Is(err, ErrHealthTimeout) {
		t.Errorf("CheckHealth() = %v, which claims a timeout that never happened", err)
	}
}

// And a real timeout still is one, by the sentinel and not by its wording.
//
// The sentinel is what the CLI branches on to name the variable that lengthens
// the wait, so an error that stops matching stops carrying the only advice a
// reader gets at the moment they need it.
func TestATimedOutHealthCheckCarriesTheSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := CheckHealth(context.Background(), portOf(srv.Listener.Addr().String()), 50*time.Millisecond, HealthCheckConfig{})
	if !errors.Is(err, ErrHealthTimeout) {
		t.Errorf("CheckHealth() = %v, want ErrHealthTimeout", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("CheckHealth() = %v, want a deadline rather than a cancellation", err)
	}
}

// portOf is the port half of a host:port address.
func portOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}
