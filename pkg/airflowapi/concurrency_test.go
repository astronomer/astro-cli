package airflowapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Detection runs on the network, so it must never happen behind a mutex: a
// mutex ignores the waiter's deadline. These cover the shape that replaced
// one — a shared probe callers wait on by channel.

func TestDetectionDoesNotOutlastAWaitersDeadline(t *testing.T) {
	const probeDelay = 1500 * time.Millisecond
	const deadline = 200 * time.Millisecond

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v2/version" {
			time.Sleep(probeDelay)
			_, _ = w.Write([]byte(`{"version":"3.0.3"}`))
			return
		}
		_, _ = w.Write([]byte(`{"dags":[],"total_entries":0}`))
	}))
	t.Cleanup(server.Close)

	transport, err := NewHTTPTransport(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := New(transport)

	var slow sync.WaitGroup
	slow.Add(1)
	go func() {
		defer slow.Done()
		_, _ = client.Generation(context.Background())
	}()
	time.Sleep(100 * time.Millisecond) // let the slow probe start

	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	start := time.Now()
	_, err = client.ListDAGs(ctx, ListDAGsOptions{})
	elapsed := time.Since(start)
	slow.Wait()

	if err == nil {
		t.Error("want the waiter's deadline to end its call")
	}
	if elapsed > probeDelay/2 {
		t.Errorf("a call with a %v deadline took %v: it waited on the probe rather than its own context", deadline, elapsed)
	}
}

func TestAFailedDetectionIsSharedRatherThanRepeatedPerCaller(t *testing.T) {
	var probes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":"upstream unavailable"}`))
	}))
	t.Cleanup(server.Close)

	transport, err := NewHTTPTransport(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := New(transport)

	var callers sync.WaitGroup
	for range 10 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			_, _ = client.ListDAGs(context.Background(), ListDAGsOptions{})
		}()
	}
	callers.Wait()

	// One round is two probes. A few rounds can start before the first
	// finishes, but ten callers must not mean ten rounds.
	if got := probes.Load(); got > 4 {
		t.Errorf("ten concurrent calls fired %d probes against a server that is down", got)
	}
}

func TestClientIsRaceFreeUnderLoad(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[{"dag_id":"etl"}],"total_entries":1}`)
	client := stub.client()

	var callers sync.WaitGroup
	for range 20 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if _, err := client.ListDAGs(context.Background(), ListDAGsOptions{}); err != nil {
				t.Error(err)
			}
			if _, err := client.Version(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	callers.Wait()
}
