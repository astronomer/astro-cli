package awsauth

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/astronomer/astro-cli/pkg/instances"
)

// One Provider serves many calls without carrying anything between them.
//
// The bug this holds off: the closure captures the Options variable, not a
// snapshot, so an assignment inside it latches the first caller's values for
// the life of the Provider and races between concurrent calls. A CLI never
// sees it, because it builds Deps and the provider set per command. A
// long-lived process builds one set and serves concurrent requests from it, so
// project B's web-login exchange would ride project A's client — its timeout,
// its TLS config, its cookie jar.
func TestOneProviderDoesNotCarryOptionsBetweenCalls(t *testing.T) {
	p := Provider(Options{Config: stubAWSConfig(t, newAWSStub(t).URL)})
	i := link(t, mwaaLink)

	first := &http.Client{}
	second := &http.Client{}
	seen := make(chan *http.Client, 2)

	// The transport is built with the client the door resolved, so asking the
	// door twice with different Deps is what exposes a latched value.
	for _, client := range []*http.Client{first, second} {
		transport, err := p.Transport(context.Background(), i, instances.Deps{HTTPClient: client})
		if err != nil {
			t.Fatalf("transport: %v", err)
		}
		got, ok := transport.(*awsTransport)
		if !ok {
			t.Fatalf("transport = %T, want the AWS door", transport)
		}
		seen <- got.base
	}
	close(seen)

	if got := <-seen; got != first {
		t.Errorf("first call used %p, want its own client %p", got, first)
	}
	if got := <-seen; got != second {
		t.Errorf("second call used %p, want its own client %p — the first call's was latched", got, second)
	}
}

// And concurrently, which is what the race detector is for.
func TestOneProviderIsSafeForConcurrentCalls(t *testing.T) {
	p := Provider(Options{Config: stubAWSConfig(t, newAWSStub(t).URL)})
	i := link(t, mwaaLink)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Transport(context.Background(), i, instances.Deps{HTTPClient: &http.Client{}}); err != nil {
				t.Errorf("transport: %v", err)
			}
		}()
	}
	wg.Wait()
}
