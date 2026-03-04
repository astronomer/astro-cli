package proxy

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/pkg/logger"
)

const (
	DefaultPort        = "6563"
	readHeaderTimeout  = 10 * time.Second
	writeTimeout       = 60 * time.Second
	idleTimeout        = 120 * time.Second
	shutdownGraceTime  = 5 * time.Second
)

// Proxy is an HTTP reverse proxy that routes requests based on the Host header.
type Proxy struct {
	port      string
	mu        sync.RWMutex
	proxies   map[string]*httputil.ReverseProxy // keyed by backend "host:port"
	transport *http.Transport
}

// NewProxy creates a new Proxy listening on the given port.
func NewProxy(port string) *Proxy {
	if port == "" {
		port = DefaultPort
	}
	return &Proxy{
		port:      port,
		proxies:   make(map[string]*httputil.ReverseProxy),
		transport: &http.Transport{},
	}
}

// Serve starts the proxy HTTP server. It handles SIGTERM/SIGINT for graceful shutdown.
func (p *Proxy) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handler)

	addr := "127.0.0.1:" + p.port
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Graceful shutdown on SIGTERM/SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		logger.Debugf("proxy received shutdown signal")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownGraceTime)
		defer cancel()
		srv.Shutdown(ctx) //nolint:errcheck
	}()

	logger.Debugf("proxy listening on %s", addr)
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
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
	rp.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		logger.Debugf("proxy error for %s: %v", req.Host, proxyErr)
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
	// Strip port from host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Bare localhost → landing page
	if host == "localhost" || host == "127.0.0.1" {
		p.landingPage(w, r)
		return
	}

	// Look up route for this hostname
	route, err := GetRoute(host)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if route == nil {
		p.notFoundPage(w, r, host)
		return
	}

	rp := p.getOrCreateProxy(route.Port)

	// Set forwarding headers
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", "http")
	if r.Header.Get("X-Forwarded-For") == "" {
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	rp.ServeHTTP(w, r)
}

// landingPage shows a table of active routes.
func (p *Proxy) landingPage(w http.ResponseWriter, _ *http.Request) {
	routes, err := ListRoutes()
	if err != nil {
		http.Error(w, "Error reading routes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Astro Dev Proxy</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 800px; margin: 50px auto; padding: 0 20px; color: #333; }
h1 { color: #1a1a2e; }
table { width: 100%; border-collapse: collapse; margin-top: 20px; }
th, td { text-align: left; padding: 12px 16px; border-bottom: 1px solid #eee; }
th { background: #f8f9fa; font-weight: 600; }
a { color: #0066cc; text-decoration: none; }
a:hover { text-decoration: underline; }
.empty { color: #666; font-style: italic; padding: 40px 0; text-align: center; }
</style></head><body>
<h1>Astro Dev Proxy</h1>`)

	if len(routes) == 0 {
		fmt.Fprint(w, `<p class="empty">No active projects. Run <code>astro dev start</code> to get started.</p>`)
	} else {
		fmt.Fprint(w, `<table><tr><th>Project</th><th>URL</th><th>Backend Port</th><th>Project Dir</th></tr>`)
		for _, r := range routes {
			projectURL := fmt.Sprintf("http://%s:%s", r.Hostname, p.port)
			fmt.Fprintf(w, `<tr><td>%s</td><td><a href="%s">%s</a></td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(strings.TrimSuffix(r.Hostname, localhostSuffix)),
				html.EscapeString(projectURL),
				html.EscapeString(projectURL),
				html.EscapeString(r.Port),
				html.EscapeString(r.ProjectDir))
		}
		fmt.Fprint(w, `</table>`)
	}

	fmt.Fprint(w, `</body></html>`)
}

// notFoundPage shows a helpful 404 page for unknown hostnames.
func (p *Proxy) notFoundPage(w http.ResponseWriter, _ *http.Request, hostname string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Not Found</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 600px; margin: 50px auto; padding: 0 20px; color: #333; }
h1 { color: #cc3300; }
code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }
a { color: #0066cc; }
</style></head><body>
<h1>Project Not Found</h1>
<p>No project is registered for hostname <code>%s</code>.</p>
<p>Check <a href="http://localhost:%s">active projects</a> or start a project with <code>astro dev start</code>.</p>
</body></html>`, html.EscapeString(hostname), p.port)
}
