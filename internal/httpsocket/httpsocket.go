// Package httpsocket sends the CLI's requests to Astronomer's hosts through a
// Unix domain socket instead of over the network, when ASTRO_HTTP_UNIX_SOCKET
// names one.
//
// Whatever listens on the socket stands in for the network: it reads each
// request, decides what to do with it, and forwards it upstream itself. The
// usual reason to run one is to keep credentials out of the CLI's hands — a
// local broker holds the real token, applies its own rules to each request,
// and adds the token on the way out, so a process driving the CLI (an agent,
// a script) never holds anything it could use elsewhere.
//
// Requests go over the socket as plain HTTP/1.1, the same way GitHub's gh
// does with its http_unix_socket setting: the URL keeps its https scheme and
// the Host header keeps the real host, and the CLI does no TLS of its own on
// the socket. Without that, the listener could not read the request it is
// meant to judge. TLS to the real host is the listener's job.
//
// Only the hosts in the list go through the socket; every other request
// leaves the way it always did. The default list is the Astro API and the
// Airflow of Astro Deployments, which take the same Astro token. Local
// Airflow, Docker, the CDN and other services stay off it.
package httpsocket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	// SocketEnv names the socket. Unset or empty, nothing changes.
	SocketEnv = "ASTRO_HTTP_UNIX_SOCKET"
	// HostsEnv replaces the default host list: a comma-separated list where
	// each entry is a host name, or *.suffix for every host below suffix.
	HostsEnv = "ASTRO_HTTP_UNIX_SOCKET_HOSTS"
)

// DefaultHosts are the hosts that go through the socket when HostsEnv is not
// set: the Astro API in each Astronomer environment, PR preview APIs, and the
// Airflow of Astro Deployments, which is served under the .run domains.
var DefaultHosts = []string{
	"api.astronomer.io",
	"api.astronomer-dev.io",
	"api.astronomer-stage.io",
	"api.astronomer-perf.io",
	"*.api.astronomer-dev.io",
	"*.astronomer.run",
	"*.astronomer-dev.run",
	"*.astronomer-stage.run",
}

// Install replaces http.DefaultTransport with one that sends requests for the
// configured hosts through the socket SocketEnv names. With SocketEnv unset
// it changes nothing.
//
// Every client in the CLI that names no transport of its own reads
// http.DefaultTransport per request, so this has to run once, before any
// command does.
func Install(getenv func(string) string) error {
	socket := strings.TrimSpace(getenv(SocketEnv))
	if socket == "" {
		return nil
	}
	// Resolved once, so a relative path means the directory the command
	// started in, not wherever the process happens to be when it dials.
	socket, err := filepath.Abs(socket)
	if err != nil {
		return fmt.Errorf("%s: %w", SocketEnv, err)
	}
	hosts := DefaultHosts
	// Empty counts as unset, so ASTRO_HTTP_UNIX_SOCKET_HOSTS= in a wrapper's
	// environment means the default rather than "no hosts".
	if raw := strings.TrimSpace(getenv(HostsEnv)); raw != "" {
		hosts = strings.Split(raw, ",")
	}
	rt, err := NewRouter(socket, hosts, http.DefaultTransport)
	if err != nil {
		return err
	}
	http.DefaultTransport = rt
	return nil
}

// Router sends a request to the socket when its host matches, and to the
// direct transport otherwise.
type Router struct {
	socket  http.RoundTripper
	direct  http.RoundTripper
	matcher matcher
}

// NewRouter builds a Router over socketPath for the given host patterns.
// direct carries every request whose host is not in the list.
func NewRouter(socketPath string, hosts []string, direct http.RoundTripper) (*Router, error) {
	m, err := parseHosts(hosts)
	if err != nil {
		return nil, err
	}
	return &Router{socket: socketTransport(socketPath), direct: direct, matcher: m}, nil
}

// RoundTrip implements http.RoundTripper.
func (r *Router) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.matcher.match(req.URL.Hostname()) {
		return r.socket.RoundTrip(req)
	}
	return r.direct.RoundTrip(req)
}

// CloseIdleConnections passes through to the direct transport, which holds
// the only pooled connections: the socket transport keeps none.
func (r *Router) CloseIdleConnections() {
	type closeIdler interface{ CloseIdleConnections() }
	if c, ok := r.direct.(closeIdler); ok {
		c.CloseIdleConnections()
	}
}

// socketTransport dials socketPath for every connection, plain or "TLS". The
// TLS dialer returning the raw connection is what keeps the socket plaintext:
// net/http then writes the https request over it as HTTP/1.1, unencrypted.
// No proxy applies to it, and keep-alives are off so each request is judged
// on a connection of its own, as gh does.
func socketTransport(socketPath string) *http.Transport {
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "unix", socketPath)
		if err != nil {
			return nil, fmt.Errorf("connect to %s set by %s: %w", socketPath, SocketEnv, err)
		}
		return conn, nil
	}
	return &http.Transport{
		DialContext:       dial,
		DialTLSContext:    dial,
		DisableKeepAlives: true,
	}
}

// matcher holds host patterns: exact names, and suffixes from *.suffix
// entries, which match hosts strictly below the suffix.
type matcher struct {
	exact    map[string]bool
	suffixes []string
}

func (m matcher) match(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	if m.exact[host] {
		return true
	}
	for _, s := range m.suffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}

func parseHosts(hosts []string) (matcher, error) {
	m := matcher{exact: map[string]bool{}}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(h, "*."); ok {
			if rest == "" || strings.Contains(rest, "*") {
				return matcher{}, fmt.Errorf("%s: %q is not a host or *.suffix", HostsEnv, h)
			}
			m.suffixes = append(m.suffixes, "."+rest)
			continue
		}
		if strings.ContainsAny(h, "*/:") {
			return matcher{}, fmt.Errorf("%s: %q is not a host or *.suffix", HostsEnv, h)
		}
		m.exact[h] = true
	}
	if len(m.exact) == 0 && len(m.suffixes) == 0 {
		return matcher{}, errors.New(HostsEnv + " names no hosts")
	}
	return m, nil
}
