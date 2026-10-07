package testing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const refused = "a unit test may not reach"

func TestIsLocalAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8080":            true,
		"127.1.2.3:80":              true,
		"[::1]:443":                 true,
		"::1":                       true,
		"0.0.0.0:8080":              true,
		"[::]:8080":                 true,
		"localhost:80":              true,
		"LOCALHOST.:80":             true,
		"project.localhost:6563":    true,
		"localhost":                 true,
		"updates.astronomer.io:443": false,
		"203.0.113.1:8080":          false,
		"[2001:db8::1]:443":         false,
		"localhost.example.com:80":  false,
		"notlocalhost:80":           false,
		"10.0.0.1":                  false,
	} {
		if got := isLocalAddress(addr); got != want {
			t.Errorf("isLocalAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}

// Importing the package guards http.DefaultTransport.
func TestDefaultTransportIsGuarded(t *testing.T) {
	for _, u := range []string{
		"https://updates.astronomer.io/astronomer-runtime",
		"http://203.0.113.1:8080/",
		"https://[2001:db8::1]/",
	} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			res.Body.Close()
			t.Errorf("%s: answered, want refused", u)
			continue
		}
		if !strings.Contains(err.Error(), refused) {
			t.Errorf("%s: %v, want refused by the guard", u, err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("an httptest server was not reached: %v", err)
	}
	res.Body.Close()
}

// A proxy on this machine must not carry a request elsewhere: the request's
// own host is checked before the proxy is chosen. Nothing listens on the
// proxy here, so a request that got past the guard would fail differently.
func TestALocalProxyDoesNotOpenTheNetwork(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:9")
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	guard(transport)
	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://updates.astronomer.io/astronomer-runtime", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err == nil {
		res.Body.Close()
		t.Fatal("answered through the proxy, want refused")
	}
	if !strings.Contains(err.Error(), refused) {
		t.Errorf("got %v, want refused by the guard", err)
	}

	// A request for this machine still goes to the proxy it names.
	local, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:8080/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	got, err := transport.Proxy(local)
	if err != nil || got == nil || got.Host != "127.0.0.1:9" {
		t.Errorf("Proxy(local) = %v, %v; want the configured proxy", got, err)
	}

	// A proxy that is not on this machine is refused when it is dialed.
	remote := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.corp.example:3128"})}
	guard(remote)
	if _, err := remote.DialContext(context.Background(), "tcp", "proxy.corp.example:3128"); err == nil || !strings.Contains(err.Error(), refused) {
		t.Errorf("dialing a remote proxy: %v, want refused by the guard", err)
	}
}
