package airflowapi

// Plain stdlib testing on purpose: pkg/airflowapi is a shared sub-module and
// keeps its dependency list near-empty (docs/architecture.md), so no
// testify here. httptest stands in for both Airflow generations.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// recordedRequest is one call the stub received.
type recordedRequest struct {
	Method string
	Path   string
	// RequestURI is the request line as it arrived, before Go decoded any
	// escapes — the only place to see whether an id stayed escaped.
	RequestURI string
	Query      url.Values
	Header     http.Header
	Body       string
}

// stubRoute is one canned answer.
type stubRoute struct {
	status   int
	body     string
	location string
}

// airflowStub is an httptest server standing in for an Airflow. Routes are
// keyed by "METHOD /full/path", so a test says which generation's path it
// expects and an unregistered path answers 404 the way Airflow does.
type airflowStub struct {
	*httptest.Server
	t *testing.T

	mu       sync.Mutex
	routes   map[string]stubRoute
	received []recordedRequest
}

func newStub(t *testing.T) *airflowStub {
	t.Helper()
	stub := &airflowStub{t: t, routes: map[string]stubRoute{}}
	stub.Server = httptest.NewServer(stub)
	t.Cleanup(stub.Close)
	return stub
}

// newAF3Stub is a stub that answers version detection as Airflow 3.
func newAF3Stub(t *testing.T) *airflowStub {
	t.Helper()
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/version", `{"version":"3.0.3","git_version":"abc"}`)
	return stub
}

// newAF2Stub is a stub that answers version detection as Airflow 2: no
// /api/v2 at all, so detection falls back.
func newAF2Stub(t *testing.T) *airflowStub {
	t.Helper()
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v1/version", `{"version":"2.10.5"}`)
	return stub
}

func (s *airflowStub) route(method, path, body string) {
	s.routeStatus(method, path, http.StatusOK, body)
}

func (s *airflowStub) routeStatus(method, path string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[method+" "+path] = stubRoute{status: status, body: body}
}

// redirect answers a path with a 302 elsewhere, the way an Airflow 2 under
// session auth bounces an unauthenticated API call to its login page.
func (s *airflowStub) redirect(method, path, location string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[method+" "+path] = stubRoute{status: http.StatusFound, location: location}
}

func (s *airflowStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("read stub request body: %v", err)
	}
	s.mu.Lock()
	s.received = append(s.received, recordedRequest{
		Method:     r.Method,
		Path:       r.URL.Path,
		RequestURI: r.RequestURI,
		Query:      r.URL.Query(),
		Header:     r.Header.Clone(),
		Body:       string(body),
	})
	route, ok := s.routes[r.Method+" "+r.URL.Path]
	s.mu.Unlock()

	if ok && route.location != "" {
		http.Redirect(w, r, route.location, route.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"detail":"Not Found"}`)
		return
	}
	w.WriteHeader(route.status)
	_, _ = io.WriteString(w, route.body)
}

// requests is everything the stub received, in order.
func (s *airflowStub) requests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.received...)
}

// lastRequest is the most recent call, for asserting a path, query, or body.
func (s *airflowStub) lastRequest() recordedRequest {
	s.t.Helper()
	received := s.requests()
	if len(received) == 0 {
		s.t.Fatal("stub received no requests")
	}
	return received[len(received)-1]
}

// countRequests is how many calls hit one method and path.
func (s *airflowStub) countRequests(method, path string) int {
	count := 0
	for _, req := range s.requests() {
		if req.Method == method && req.Path == path {
			count++
		}
	}
	return count
}

// client builds a client against the stub over an HTTP transport.
func (s *airflowStub) client() *Client {
	s.t.Helper()
	transport, err := NewHTTPTransport(s.URL)
	if err != nil {
		s.t.Fatalf("build transport: %v", err)
	}
	return New(transport)
}
