package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultPort       = "6563"
	readHeaderTimeout = 10 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownGraceTime = 5 * time.Second
	startFailWindow   = 100 * time.Millisecond
)

// Proxy is an HTTP reverse proxy that routes requests based on the Host
// header, using a Store to resolve hostnames to backend ports.
type Proxy struct {
	// ModifyResponse hooks run in order on every proxied response.
	// Set them before calling Start.
	ModifyResponse []func(*http.Response) error

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
// down. If the configured port is taken, Start falls back to an OS-assigned
// ephemeral port; read the bound port back with Port().
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

	v4, boundPort, err := bindLoopbackWithFallback(p.Port())
	if err != nil {
		return fmt.Errorf("proxy failed to start: %w", err)
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
		return fmt.Errorf("proxy failed to start: %w", err)
	case <-time.After(startFailWindow):
		return nil
	}
}

// bindLoopbackWithFallback binds 127.0.0.1:preferredPort, falling back to an
// OS-assigned ephemeral port if preferredPort is already taken. Returns the
// listener and the port actually bound.
func bindLoopbackWithFallback(preferredPort string) (l net.Listener, actualPort string, err error) {
	if l, err = net.Listen("tcp4", "127.0.0.1:"+preferredPort); err == nil {
		return l, preferredPort, nil
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
	p.srv.Shutdown(ctx) //nolint:errcheck
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

	target, _ := url.Parse("http://127.0.0.1:" + backendPort)
	rp = httputil.NewSingleHostReverseProxy(target)
	rp.Transport = p.transport
	rp.ModifyResponse = func(resp *http.Response) error {
		for _, hook := range p.ModifyResponse {
			if err := hook(resp); err != nil {
				return err
			}
		}
		return nil
	}
	rp.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		slog.Debug("proxy error", "host", req.Host, "error", proxyErr)
		http.Error(rw, "Backend unavailable", http.StatusBadGateway)
	}

	p.mu.Lock()
	// Double-check in case another goroutine created it
	if existing, ok := p.proxies[backendPort]; ok {
		p.mu.Unlock()
		return existing
	}
	p.proxies[backendPort] = rp
	p.mu.Unlock()
	return rp
}

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

	rp := p.getOrCreateProxy(route.Port)

	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", "http")
	if r.Header.Get("X-Forwarded-For") == "" {
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	rp.ServeHTTP(w, r)
}

// landingPage shows a table of active routes.
func (p *Proxy) landingPage(w http.ResponseWriter) {
	routes, err := p.store.ListRoutes()
	if err != nil {
		http.Error(w, "Error reading routes", http.StatusInternalServerError)
		return
	}

	data := landingData{
		Routes: make([]landingRoute, len(routes)),
	}
	for i, r := range routes {
		data.Routes[i] = landingRoute{
			Name:       strings.TrimSuffix(r.Hostname, LocalhostSuffix),
			URL:        fmt.Sprintf("http://%s:%s", r.Hostname, p.Port()),
			Port:       r.Port,
			ProjectDir: r.ProjectDir,
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := landingTmpl.Execute(w, data); err != nil {
		slog.Debug("landing page template", "error", err)
	}
}

// notFoundPage shows a helpful 404 page for unknown hostnames.
func (p *Proxy) notFoundPage(w http.ResponseWriter, hostname string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := notFoundTmpl.Execute(w, notFoundData{
		Hostname: hostname,
		Port:     p.Port(),
	}); err != nil {
		slog.Debug("not found page template", "error", err)
	}
}
