package testing

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Every test binary that imports this package reaches only this machine
// through http.DefaultTransport: a request to any other host fails at once,
// with an error saying so, instead of reaching the network. A unit test that
// reached updates.astronomer.io passed or failed with the network, and one
// failed a combined run on a TLS handshake timeout. An httptest server, or
// anything else on a loopback address or under .localhost, is still reached.
//
// It covers every client that goes through http.DefaultTransport, which is
// what an http.Client with no Transport of its own uses, and every clone of
// it. A client that builds its own transport is not covered. It does not
// apply to a binary that does not import this package, and no code but a
// test may import it (internal/archlint holds that).
func init() {
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		guard(t)
	}
}

// guard makes t refuse any host but this machine. It checks twice: the
// request's own host, before the proxy is chosen, so a proxy on this machine
// (HTTPS_PROXY=http://127.0.0.1:…) cannot carry a request elsewhere; and the
// address dialed, which catches a proxy that is not on this machine.
func guard(t *http.Transport) {
	proxy := t.Proxy
	t.Proxy = func(req *http.Request) (*url.URL, error) {
		if !isLocalAddress(req.URL.Host) {
			return nil, refusal(req.URL.Host)
		}
		if proxy == nil {
			return nil, nil
		}
		return proxy(req)
	}
	dial := t.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !isLocalAddress(addr) {
			return nil, refusal(addr)
		}
		return dial(ctx, network, addr)
	}
}

func refusal(host string) error {
	return fmt.Errorf("a unit test may not reach %s: stub the client, or serve it with httptest (see pkg/testing/nonetwork.go)", host)
}

// isLocalAddress reports whether addr, a host or host:port, names this
// machine without asking a resolver: a loopback or unspecified IP
// (0.0.0.0 and [::] are dialed as this machine), localhost, or a name under
// .localhost, which RFC 6761 reserves for loopback.
func isLocalAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}
