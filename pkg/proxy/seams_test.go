package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"
	"testing"
)

// refusedPort is a port nothing is listening on, obtained by binding one and
// letting it go.
//
// Not a hardcoded low port: a sandbox that DROPs rather than RESETs turns an
// expected connection-refused into the OS connect timeout, and the test hangs
// for a minute or two instead of failing.
func refusedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// A host supplies the RoundTripper for a backend, and gets told which backend
// it is for — once per backend, however many requests race to be first.
//
// Per backend because a host's transport may hold state for one: a session to
// renew, a pool to keep separate. Once because a discarded transport may have
// opened either, and nothing will close it.
func TestTransportIsBuiltOncePerBackend(t *testing.T) {
	p := NewProxy("0", NewStore(t.TempDir()))

	var mu sync.Mutex
	asked := map[string]int{}
	p.Transport = func(backendPort string) http.RoundTripper {
		mu.Lock()
		asked[backendPort]++
		mu.Unlock()
		return roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("not dialed in this test")
		})
	}

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.getOrCreateProxy("15001")
		}()
	}
	wg.Wait()
	p.getOrCreateProxy("15002")

	mu.Lock()
	defer mu.Unlock()
	if asked["15001"] != 1 {
		t.Errorf("built %d transports for one backend under contention, want 1", asked["15001"])
	}
	if asked["15002"] != 1 {
		t.Errorf("built %d transports for the second backend, want 1", asked["15002"])
	}
}

// Nothing supplied, a supplier that declines, and a supplier that returns a
// TYPED nil all fall back to the shared default.
//
// The typed nil is the one worth testing: a nil *T boxed in the interface is
// not == nil, so a plain check installs it and the first request panics inside
// RoundTrip. It is easy to write by accident.
func TestTransportFallsBackToTheDefault(t *testing.T) {
	p := NewProxy("0", NewStore(t.TempDir()))
	if got := p.transportFor("15001"); got != p.transport {
		t.Errorf("no supplier: got %v, want the shared transport", got)
	}

	p.Transport = func(string) http.RoundTripper { return nil }
	if got := p.transportFor("15001"); got != p.transport {
		t.Errorf("supplier returned nil: got %v, want the shared transport", got)
	}

	p.Transport = func(string) http.RoundTripper {
		var typed *roundTripperStruct
		return typed
	}
	if got := p.transportFor("15001"); got != p.transport {
		t.Errorf("supplier returned a typed nil: got %#v, want the shared transport", got)
	}
}

// The generated pages hand over their BODY and keep their headers, checked ON
// THE WIRE rather than through a recorder.
//
// A recorder returns the still-mutable header map unless WriteHeader has been
// called, so a hook that deleted a header would still be observed as having it.
// A real server is what shows the difference.
func TestRenderHooksCannotTouchTheHeaders(t *testing.T) {
	p := NewProxy("6563", NewStore(t.TempDir()))
	sabotage := func(w io.Writer) {
		if rw, ok := w.(http.ResponseWriter); ok {
			rw.Header().Del(SignatureHeader)
			rw.Header().Set("Content-Type", "text/plain")
		}
	}
	p.RenderLanding = func(w io.Writer, routes []LandingRoute) {
		sabotage(w)
		fmt.Fprintf(w, "our landing, %d routes", len(routes))
	}
	p.RenderNotFound = func(w io.Writer, hostname, port string) {
		sabotage(w)
		fmt.Fprintf(w, "our 404 for %s on %s", hostname, port)
	}

	srv := httptest.NewServer(http.HandlerFunc(p.handler))
	defer srv.Close()

	for _, tc := range []struct {
		name, host, wantBody string
		wantStatus           int
	}{
		{"landing", "localhost", "our landing, 0 routes", http.StatusOK},
		{"not found", "gone.localhost", "our 404 for gone.localhost on 6563", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+"/", http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = tc.host
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if got := resp.Header.Get(SignatureHeader); got != SignatureValue {
				t.Errorf("signature on the wire = %q, want %q", got, SignatureValue)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			// The exact string: "contains" cannot tell the hostname and the
			// port apart if they are handed over in the wrong order.
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

// The landing hook receives the routes, with their values.
//
// Asserted rather than counted: a hook handed nil instead of the mapped routes
// passes any test that only looks for its own prose.
func TestRenderLandingReceivesTheRoutes(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AddRoute(&Route{
		Hostname:   "demo" + LocalhostSuffix,
		Port:       "18080",
		ProjectDir: "/tmp/demo",
		PID:        1,
		Mode:       RouteModeDocker,
	}); err != nil {
		t.Fatal(err)
	}

	p := NewProxy("6563", store)
	var got []LandingRoute
	p.RenderLanding = func(_ io.Writer, routes []LandingRoute) { got = routes }
	p.landingPage(httptest.NewRecorder())

	if len(got) != 1 {
		t.Fatalf("hook received %d routes, want the one in the store", len(got))
	}
	want := LandingRoute{
		Name:       "demo",
		URL:        "http://demo" + LocalhostSuffix + ":6563",
		Port:       "18080",
		ProjectDir: "/tmp/demo",
	}
	if got[0] != want {
		t.Errorf("route = %+v, want %+v", got[0], want)
	}
}

// Without hooks the built-in pages still render, which is what makes this
// additive for the daemon, which sets none of them.
func TestTheBuiltInPagesStillRender(t *testing.T) {
	p := NewProxy("6563", NewStore(t.TempDir()))

	rec := httptest.NewRecorder()
	p.landingPage(rec)
	if rec.Header().Get(SignatureHeader) != SignatureValue || rec.Body.Len() == 0 {
		t.Errorf("landing: signature %q, %d bytes", rec.Header().Get(SignatureHeader), rec.Body.Len())
	}

	rec = httptest.NewRecorder()
	p.notFoundPage(rec, "gone.localhost")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "gone.localhost") {
		t.Errorf("not found: status %d, body %q", rec.Code, rec.Body.String())
	}
}

// A host with somewhere better to send people gets to. The default is a plain
// 502, right for a tool with no UI and wrong for an app that can explain the
// project is not running.
func TestErrorHandlerReplacesThePlain502(t *testing.T) {
	p := NewProxy("0", NewStore(t.TempDir()))

	var got error
	p.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		got = err
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "the project is not running")
	}

	rp := p.getOrCreateProxy(refusedPort(t))
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x.localhost/", http.NoBody))

	if got == nil {
		t.Error("the hook was not told what went wrong")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want the host's", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not running") {
		t.Errorf("body = %q, want the host's page", rec.Body.String())
	}
}

// And without one, the plain 502 stands.
func TestWithoutAnErrorHandlerThe502Stands(t *testing.T) {
	p := NewProxy("0", NewStore(t.TempDir()))
	rp := p.getOrCreateProxy(refusedPort(t))
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x.localhost/", http.NoBody))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

// The request hook runs on every proxied request, after the forwarding headers
// are set — which is what lets a host authenticate without putting request
// mutation inside a RoundTripper, where the interface says it must not go.
//
// It also pins the header stripping: a client naming its own origin is
// replaced, not believed. httputil strips those headers only on the Rewrite
// path, so this fails the moment the proxy goes back to a Director.
func TestModifyRequestRunsAndTheClientCannotForgeItsOrigin(t *testing.T) {
	var mu sync.Mutex
	var gotXFF, gotAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotXFF, gotAuth = r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Test-Auth")
		mu.Unlock()
	}))
	defer backend.Close()
	_, backendPort, err := net.SplitHostPort(backend.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	store := NewStore(t.TempDir())
	if err := store.AddRoute(&Route{
		Hostname: "demo" + LocalhostSuffix, Port: backendPort, PID: 1, Mode: RouteModeDocker,
	}); err != nil {
		t.Fatal(err)
	}

	p := NewProxy("0", store)
	p.ModifyRequest = []func(*httputil.ProxyRequest){
		func(pr *httputil.ProxyRequest) { pr.Out.Header.Set("X-Test-Auth", "from-the-hook") },
	}

	srv := httptest.NewServer(http.HandlerFunc(p.handler))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "demo" + LocalhostSuffix
	req.Header.Set("X-Forwarded-For", "10.0.0.1") // a forged origin
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "from-the-hook" {
		t.Errorf("X-Test-Auth = %q, want the hook's", gotAuth)
	}
	if strings.Contains(gotXFF, "10.0.0.1") {
		t.Errorf("X-Forwarded-For = %q, want the client's claim replaced rather than believed", gotXFF)
	}
	if gotXFF == "" {
		t.Error("X-Forwarded-For was not set at all")
	}
}

// The backend sees the Host the browser sent. SetURL replaces it with the
// backend's own address, and Airflow 3 then rejects every login redirect from a
// hostname link: its `next` must match the request's URL, which read as
// 127.0.0.1:<port> rather than the name in the address bar.
func TestTheBackendSeesTheHostTheBrowserSent(t *testing.T) {
	var mu sync.Mutex
	var gotHost string
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHost = r.Host
		mu.Unlock()
	}))
	defer backend.Close()
	_, backendPort, err := net.SplitHostPort(backend.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	store := NewStore(t.TempDir())
	if err := store.AddRoute(&Route{
		Hostname: "demo" + LocalhostSuffix, Port: backendPort, PID: 1, Mode: RouteModeDocker,
	}); err != nil {
		t.Fatal(err)
	}

	p := NewProxy("0", store)
	srv := httptest.NewServer(http.HandlerFunc(p.handler))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	// With the proxy's port, as a browser sends it.
	want := "demo" + LocalhostSuffix + ":6563"
	req.Host = want
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the request routed to the backend", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotHost != want {
		t.Errorf("backend saw Host %q, want %q: Airflow validates redirects against it", gotHost, want)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// roundTripperStruct exists so a test can produce a typed nil.
type roundTripperStruct struct{}

func (*roundTripperStruct) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }
