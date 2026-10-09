package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	DefaultPort       = "6563"
	readHeaderTimeout = 10 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownGraceTime = 5 * time.Second
	startFailWindow   = 100 * time.Millisecond

	// maxLoggedHost caps a client-supplied Host in a log line. Long enough for
	// any hostname this proxy routes, short enough that nothing can flood one.
	maxLoggedHost = 253

	// SignatureHeader and SignatureValue mark responses the proxy generates
	// itself (its landing and not-found pages). A caller can probe the proxy's
	// port and check for this header to confirm it's really the astro proxy on
	// the other end, not an unrelated process that recycled its PID/port.
	SignatureHeader = "X-Astro-Proxy"
	SignatureValue  = "astro-local-proxy"
)

// Proxy is an HTTP reverse proxy that routes requests based on the Host
// header, using a Store to resolve hostnames to backend ports.
type Proxy struct {
	// ModifyResponse hooks run in order on every proxied response, after the
	// proxy has put the Astronomer theme into it (InjectAirflowTheme). Set them
	// before calling Start.
	ModifyResponse []func(*http.Response) error

	// ModifyRequest hooks run in order on every proxied request, after the
	// URL and the forwarding headers are set. Set them before calling Start.
	//
	// This is where a host authenticates. It does NOT belong in a Transport:
	// http.RoundTripper's contract says RoundTrip "should not modify the
	// request" and "should not attempt to handle higher-level protocol details
	// such as ... authentication, or cookies", so a transport that injects
	// credentials is working against the interface it implements.
	ModifyRequest []func(*httputil.ProxyRequest)

	// Transport supplies the RoundTripper for one backend. Nil, or a nil
	// return, uses the shared default. Set it before calling Start.
	//
	// Per backend rather than one value for the proxy, because a host's
	// transport may hold state for a particular backend — a session to renew, a
	// connection pool to keep separate — which it cannot do without knowing
	// which backend it is for.
	//
	// Called once per backend, under the lock that caches that backend's
	// reverse proxy, so a burst of first requests yields one transport.
	Transport func(backendPort string) http.RoundTripper

	// ErrorHandler writes the response when a proxied request fails. Nil serves
	// a 502: the unavailable page to a browser navigating to a backend that is
	// not answering, plain text otherwise (see defaultErrorHandler). Set it
	// before calling Start.
	//
	// A host with somewhere better to send people wants this: the desktop
	// serves a page explaining that the project is not running, which is a more
	// useful answer than "Bad Gateway" to someone who has an app in front of
	// them.
	//
	// "Fails" is wider than "the backend is down". httputil routes three things
	// here: a transport error, a ModifyResponse hook returning an error, and its
	// own Director/Rewrite misconfiguration. A host that renders "the project is
	// not running" for all three will say that about a project which is running
	// fine, and hide its own bug. The error is passed through so the host can
	// tell them apart.
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

	// RenderLanding and RenderNotFound write the BODY of the pages the proxy
	// generates itself. Nil uses the built-in templates. Set them before
	// calling Start.
	//
	// The body only, and the writer they are handed cannot be turned back into
	// the ResponseWriter. Status and headers are committed before either runs,
	// because one of those headers is the proxy's signature — how a daemon
	// decides whether the process holding a recorded port is really a proxy
	// (airflow/proxy's probeProxySignature GETs the landing page for exactly
	// this). A hook that reached the headers could drop it, and the failure
	// would surface as an unrelated tool concluding the proxy is not running
	// and starting a second one.
	RenderLanding  func(w io.Writer, routes []LandingRoute)
	RenderNotFound func(w io.Writer, hostname, port string)

	// Pages is the copy on the built-in pages that differs between hosts: the
	// proxy's name and how to start a project. The zero value is the CLI's. Set
	// it before calling Start.
	Pages Pages

	// FallbackPort is the port Start tries when the configured one is taken,
	// before it asks the OS for any free port. Empty skips it. Set it before
	// calling Start.
	//
	// A host that remembers the port it fell back to last time and passes it
	// here keeps its URLs across restarts while something else holds the
	// configured port.
	FallbackPort string

	store     *Store
	mu        sync.RWMutex
	proxies   map[string]*httputil.ReverseProxy // keyed by backend port
	transport *http.Transport
	srv       *http.Server

	// portMu guards port/running: Start rewrites port at bind time (the
	// configured port is not necessarily the one bound — see the fallback
	// in Start), and Port/Running read from other goroutines.
	portMu  sync.RWMutex
	port    string
	running bool
}

// NewProxy creates a Proxy that listens on port and resolves routes via store.
func NewProxy(port string, store *Store) *Proxy {
	if port == "" {
		port = DefaultPort
	}
	return &Proxy{
		store:     store,
		port:      port,
		proxies:   make(map[string]*httputil.ReverseProxy),
		transport: &http.Transport{},
	}
}

// Port returns the proxy's listening port. Before a successful Start it
// returns the configured port; after, the port actually bound.
func (p *Proxy) Port() string {
	p.portMu.RLock()
	defer p.portMu.RUnlock()
	return p.port
}

// Running reports whether Start has completed a successful bind.
func (p *Proxy) Running() bool {
	p.portMu.RLock()
	defer p.portMu.RUnlock()
	return p.running
}

// Start binds the listeners and serves in the background. Call Stop to shut
// down. If the configured port is taken, Start falls back to FallbackPort and
// then to an OS-assigned ephemeral port; read the bound port back with Port().
//
// Both loopback families are bound: `localhost` and `*.localhost` resolve to
// ::1 before 127.0.0.1 on macOS, and clients that don't fall back to IPv4
// would get connection-refused against an IPv4-only listener. IPv6 is
// best-effort — some environments disable it.
func (p *Proxy) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handler)

	p.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	v4, boundPort, err := bindLoopbackWithFallback(p.Port(), p.FallbackPort)
	if err != nil {
		return fmt.Errorf("starting proxy: %w", err)
	}

	p.portMu.Lock()
	p.port = boundPort
	p.running = true
	p.portMu.Unlock()

	errCh := make(chan error, 2)
	serve := func(l net.Listener) {
		slog.Debug("proxy listening", "addr", l.Addr().String())
		if serveErr := p.srv.Serve(l); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}
	go serve(v4)

	if v6, err6 := net.Listen("tcp6", "[::1]:"+boundPort); err6 == nil {
		go serve(v6)
	} else {
		slog.Debug("proxy: IPv6 loopback unavailable, serving IPv4 only", "error", err6)
	}

	// Give Serve a moment to fail fast (e.g. an error right after Listen).
	select {
	case err := <-errCh:
		return fmt.Errorf("starting proxy: %w", err)
	case <-time.After(startFailWindow):
		return nil
	}
}

// bindLoopbackWithFallback binds 127.0.0.1 on the first of preferredPort and
// fallbackPort that is free, and on an OS-assigned ephemeral port if neither
// is. Returns the listener and the port actually bound.
func bindLoopbackWithFallback(preferredPort, fallbackPort string) (l net.Listener, actualPort string, err error) {
	for _, port := range []string{preferredPort, fallbackPort} {
		if port == "" {
			continue
		}
		if l, err = net.Listen("tcp4", "127.0.0.1:"+port); err == nil {
			return l, port, nil
		}
	}
	l, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	return l, fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port), nil
}

// Stop gracefully shuts down the proxy server.
func (p *Proxy) Stop() {
	if p.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGraceTime)
	defer cancel()
	// Shutdown reports only that the grace period expired with connections
	// still open. The listener is closed either way and this returns nothing,
	// so there is no caller to tell.
	p.srv.Shutdown(ctx) //nolint:errcheck // deliberate, for the reason above
}

// getOrCreateProxy returns a cached reverse proxy for the given backend port,
// creating one if it doesn't exist yet.
func (p *Proxy) getOrCreateProxy(backendPort string) *httputil.ReverseProxy {
	p.mu.RLock()
	rp, ok := p.proxies[backendPort]
	p.mu.RUnlock()
	if ok {
		return rp
	}

	// Built rather than parsed: the host is fixed and only the port varies, so
	// there is no input that could make this fail and no error to drop.
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", backendPort)}
	rp = &httputil.ReverseProxy{}

	// Rewrite rather than Director, and not only for the hook.
	//
	// httputil strips client-supplied Forwarded / X-Forwarded-* headers ONLY on
	// the Rewrite path. Under Director they pass through untouched, so anything
	// that can reach this proxy — a local process, an embedded page — can tell
	// the backend where it came from and be believed. SetXForwarded replaces
	// them with the real values.
	//
	// The two are also mutually exclusive: httputil errors out if both are set,
	// which is why a host cannot supply its own Rewrite from outside and why
	// ModifyRequest exists.
	rp.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		// The Host the browser sent, not the backend's address, which is what
		// SetURL leaves. Airflow 3 accepts a login redirect's `next` only when it
		// matches the request's own URL or api.base_url, so with 127.0.0.1:<port>
		// here every hostname link failed at login with "Invalid or unsafe next
		// URL"; redirects built from the request URL pointed at the bare port
		// too. Safe to pass on: the handler routed on this Host, so it is a
		// registered project hostname and nothing a client can pick freely.
		pr.Out.Host = pr.In.Host
		pr.SetXForwarded()
		for _, hook := range p.ModifyRequest {
			hook(pr)
		}
	}
	rp.ModifyResponse = func(resp *http.Response) error {
		// Every tool serving these routes themes Airflow the same way, so the
		// theme is the proxy's rather than a hook each host has to remember.
		if err := InjectAirflowTheme(resp); err != nil {
			return err
		}
		for _, hook := range p.ModifyResponse {
			if err := hook(resp); err != nil {
				return err
			}
		}
		return nil
	}
	rp.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		// req.Host is whatever the client sent, and it is the one field here a
		// caller controls. slog's own handlers quote a value carrying control
		// characters, but an embedder can install one that does not.
		slog.Debug("proxy error", "host", safeHost(req.Host), "error", proxyErr)
		if p.ErrorHandler != nil {
			p.ErrorHandler(rw, req, proxyErr)
			return
		}
		defaultErrorHandler(rw, req, proxyErr)
	}

	p.mu.Lock()
	// Double-check in case another goroutine created it
	if existing, ok := p.proxies[backendPort]; ok {
		p.mu.Unlock()
		return existing
	}
	// The transport is built HERE, under the lock, rather than beside the
	// reverse proxy above: a burst of first requests for one backend races to
	// build a proxy and all but one are discarded, and a discarded transport may
	// have opened a session or a pool that nothing will ever close.
	rp.Transport = p.transportFor(backendPort)
	p.proxies[backendPort] = rp
	p.mu.Unlock()
	return rp
}

// LandingRoute is one row of the landing page, as RenderLanding receives it.
// Exported because a host that renders its own page needs the shape.
type LandingRoute struct {
	Name       string
	URL        string
	Port       string
	ProjectDir string
}

// transportFor is the RoundTripper for one backend: the host's, when it supplies
// one, and the shared default otherwise.
//
// A typed nil counts as "none". A factory that returns a nil *T boxed in the
// interface produces a value that is not == nil, so a plain check would install
// it and every request through that backend would panic inside RoundTrip
// instead of falling back — a shape that is easy to write by accident:
//
//	var t *authTransport
//	if hasSession(port) { t = newAuth(port) }
//	return t
func (p *Proxy) transportFor(backendPort string) http.RoundTripper {
	if p.Transport == nil {
		return p.transport
	}
	rt := p.Transport(backendPort)
	if rt == nil || isNilPointer(rt) {
		return p.transport
	}
	return rt
}

// safeHost bounds a client-supplied Host for logging: control characters are
// dropped so it cannot forge a second line, and the length is capped so it
// cannot flood one.
//
// Cleaned before it is cut, not after. A Host is bytes off the wire and need
// not be UTF-8; Map turns each undecodable byte into a 3-byte replacement
// character, so cutting first and cleaning second lets 253 bytes of garbage
// come back as 759 — three times the cap, on the input most likely to be
// hostile. The cut then lands on a rune boundary, so truncation cannot
// manufacture a replacement character of its own.
func safeHost(h string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, h)
	if len(cleaned) <= maxLoggedHost {
		return cleaned
	}
	cut := maxLoggedHost
	for cut > 0 && !utf8.RuneStart(cleaned[cut]) {
		cut--
	}
	return cleaned[:cut]
}

// isNilPointer reports whether v is an interface holding a nil pointer.
func isNilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	//nolint:exhaustive // every other kind cannot be nil, which is what default answers
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

// bodyOnly hides everything but Write, so a render hook cannot reach the
// ResponseWriter it came from. See RenderLanding.
func bodyOnly(w io.Writer) io.Writer { return struct{ io.Writer }{w} }

// handler routes requests based on the Host header.
func (p *Proxy) handler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Bare localhost → landing page
	if host == "localhost" || host == "127.0.0.1" || host == "[::1]" {
		p.landingPage(w)
		return
	}

	route, err := p.store.GetRoute(host)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if route == nil {
		p.notFoundPage(w, host)
		return
	}

	// The forwarding headers are SetXForwarded's now, in the Rewrite above.
	// Setting them here as well was both redundant and worse: it preserved an
	// inbound X-Forwarded-For rather than replacing it, so a client could name
	// its own origin, and the value it did set was RemoteAddr — "ip:port",
	// where the header takes a bare address.
	p.getOrCreateProxy(route.Port).ServeHTTP(w, r)
}

// landingPage shows a table of active routes.
//
// Reads and filters; never writes. It listed through ListRoutes, which prunes
// and then persists what it pruned — so rendering this page deleted rows. The
// daemon's store carries no liveness predicate (airflow/proxy's Routes()), so
// the verdict being persisted was the default one: a route judged by the pid
// recorded in it, which is the process that registered the route rather than
// the runtime it fronts. Those diverge whenever the owner is replaced — desktop
// restarting, or a standalone master exiting ahead of the group it leads — and
// one GET on http://localhost:6563/ then removed a live project's hostname.
//
// The filtering stays, because a page listing projects that stopped weeks ago
// would be its own bug. Only the writing goes.
//
// Reading without the routes lock, as GetRoute does on the request path and
// for the same reason: writes land by atomic rename, so a reader sees one
// version or the other. It also keeps a browser hit off a lock the CLI needs —
// the landing page holding it is what made stopping the daemon while a request
// was in flight turn a graceful shutdown into a kill.
func (p *Proxy) landingPage(w http.ResponseWriter) {
	stored, err := p.store.ReadRoutes()
	if err != nil {
		http.Error(w, "Error reading routes", http.StatusInternalServerError)
		return
	}
	routes := p.store.pruneStale(stored)

	listed := make([]LandingRoute, len(routes))
	for i, r := range routes {
		listed[i] = LandingRoute{
			Name:       strings.TrimSuffix(r.Hostname, LocalhostSuffix),
			URL:        fmt.Sprintf("http://%s:%s", r.Hostname, p.Port()),
			Port:       r.Port,
			ProjectDir: r.ProjectDir,
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set(SignatureHeader, SignatureValue)
	if p.RenderLanding != nil {
		// WriteHeader first, so the headers above are committed before the hook
		// runs; and bodyOnly, so the hook cannot assert its way back to the
		// ResponseWriter and change them. Handing over the ResponseWriter as an
		// io.Writer looks equivalent and is not — the assertion succeeds, and
		// this is the page a daemon probes for the signature.
		w.WriteHeader(http.StatusOK)
		p.RenderLanding(bodyOnly(w), listed)
		return
	}
	data := landingData{Title: p.Pages.title(), Pages: p.Pages, Routes: listed}
	if err := pageTmpl.ExecuteTemplate(w, landingPageName, data); err != nil {
		slog.Debug("landing page template", "error", err)
	}
}

// notFoundPage shows a helpful 404 page for unknown hostnames.
func (p *Proxy) notFoundPage(w http.ResponseWriter, hostname string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set(SignatureHeader, SignatureValue)
	w.WriteHeader(http.StatusNotFound)
	if p.RenderNotFound != nil {
		p.RenderNotFound(bodyOnly(w), hostname, p.Port())
		return
	}
	if err := pageTmpl.ExecuteTemplate(w, notFoundPageName, notFoundData{
		Pages:    p.Pages,
		Hostname: hostname,
		Port:     p.Port(),
	}); err != nil {
		slog.Debug("not found page template", "error", err)
	}
}

// defaultErrorHandler answers a failed proxied request when the host has not
// supplied an ErrorHandler. It is always a 502; what varies is the body.
//
// The unavailable page goes only to a browser navigation that could not reach
// the backend, which is what someone opening a stopped or still-booting project
// sees. An API client (af, curl, Airflow's own fetches) gets plain text, because
// a page of HTML is noise to a program reading the status. A failure inside the
// proxy's own handling gets plain text too: telling someone the project is
// starting up when it is running fine hides the bug.
func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if !isBackendUnreachable(err) || !wantsHTML(r) {
		http.Error(w, "Backend unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	if renderErr := RenderUnavailable(w); renderErr != nil {
		slog.Debug("unavailable page template", "error", renderErr)
	}
}

// isBackendUnreachable reports whether err is the proxy failing to reach the
// backend, as opposed to something going wrong in a ModifyResponse hook or in
// httputil's own setup.
//
// A dial that is refused, times out, or finds nothing listening is the project
// not running. An error a hook returned is not, however much it looks like one
// from here.
func isBackendUnreachable(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// wantsHTML reports whether r is a browser navigation rather than an API call.
// An empty Accept counts, since whoever sent no preference still reads the body.
func wantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}
