package httpsocket

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seen is what the listener on the socket saw of one request.
type seen struct {
	method, host, uri, auth string
	tls                     bool
}

// listen serves HTTP on a fresh Unix socket, standing in for a broker, and
// returns its path and the requests it saw.
func listen(t *testing.T) (path string, got <-chan seen) {
	t.Helper()
	// Not t.TempDir: macOS caps a socket path at 104 bytes, and a test's temp
	// dir with its name in it runs past that.
	dir, err := os.MkdirTemp("", "hs")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	path = filepath.Join(dir, "s")

	ln, err := net.Listen("unix", path)
	require.NoError(t, err)
	requests := make(chan seen, 8)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- seen{method: r.Method, host: r.Host, uri: r.RequestURI, auth: r.Header.Get("Authorization"), tls: r.TLS != nil}
		_, _ = io.WriteString(w, "from socket")
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path, requests
}

// directRecorder stands in for the network: it records the hosts it was
// asked for and answers every request itself.
type directRecorder struct{ hosts []string }

func (d *directRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	d.hosts = append(d.hosts, r.URL.Host)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("direct")), Request: r}, nil
}

func get(t *testing.T, rt http.RoundTripper, url, auth string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := (&http.Client{Transport: rt}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

// The listener has to be able to read the request it is asked to judge: the
// real host, the path and query, and the token the CLI sent. So the request
// arrives as plain HTTP even though its URL is https.
func TestAstroHostsGoThroughTheSocketInPlaintext(t *testing.T) {
	path, got := listen(t)
	direct := &directRecorder{}
	rt, err := NewRouter(path, DefaultHosts, direct)
	require.NoError(t, err)

	cases := []struct{ url, host, uri string }{
		{"https://api.astronomer.io/v1/organizations?limit=5", "api.astronomer.io", "/v1/organizations?limit=5"},
		{"https://pr12345.api.astronomer-dev.io/v1/organizations", "pr12345.api.astronomer-dev.io", "/v1/organizations"},
		{"https://acme.astronomer.run/dabc123/api/v2/dags", "acme.astronomer.run", "/dabc123/api/v2/dags"},
		{"https://ACME.astronomer-stage.run./d1/api/v2/dags", "ACME.astronomer-stage.run.", "/d1/api/v2/dags"},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			body := get(t, rt, c.url, "Bearer placeholder")
			assert.Equal(t, "from socket", body)
			s := <-got
			assert.Equal(t, http.MethodGet, s.method)
			assert.Equal(t, c.host, s.host)
			assert.Equal(t, c.uri, s.uri)
			assert.Equal(t, "Bearer placeholder", s.auth)
			assert.False(t, s.tls, "the request reached the socket over TLS")
		})
	}
	assert.Empty(t, direct.hosts)
}

// Everything that is not an Astro API or Astro Airflow host leaves the way it
// always did: local Airflow, the CDN, the login server, other services.
func TestOtherHostsGoDirect(t *testing.T) {
	path, got := listen(t)
	direct := &directRecorder{}
	rt, err := NewRouter(path, DefaultHosts, direct)
	require.NoError(t, err)

	for _, url := range []string{
		"http://localhost:8080/api/v2/dags",
		"http://127.0.0.1:8080/health",
		"https://cdn.astronomer.io/runtime-constraints/x.txt",
		"https://auth.astronomer.io/oauth/token",
		"https://updates.astronomer.io/astronomer-runtime",
		"https://astronomer.run/",
		"https://api.github.com/",
	} {
		assert.Equal(t, "direct", get(t, rt, url, ""), url)
	}
	assert.Len(t, direct.hosts, 7)
	select {
	case s := <-got:
		t.Errorf("the socket saw a request for %s", s.host)
	default:
	}
}

func TestHostsOverride(t *testing.T) {
	path, got := listen(t)
	direct := &directRecorder{}
	rt, err := NewRouter(path, []string{" Astro.Example.com ", "", "*.corp.internal"}, direct)
	require.NoError(t, err)

	assert.Equal(t, "from socket", get(t, rt, "https://astro.example.com/v1/x", ""))
	assert.Equal(t, "astro.example.com", (<-got).host)
	assert.Equal(t, "from socket", get(t, rt, "https://airflow.corp.internal/api/v2/dags", ""))
	assert.Equal(t, "airflow.corp.internal", (<-got).host)
	// *.suffix covers hosts below the suffix, not the suffix itself.
	assert.Equal(t, "direct", get(t, rt, "https://corp.internal/", ""))
	assert.Equal(t, "direct", get(t, rt, "https://api.astronomer.io/v1/x", ""))
}

func TestBadHostLists(t *testing.T) {
	for _, hosts := range [][]string{
		{},
		{" ", ""},
		{"*."},
		{"*.a.*.b"},
		{"api*.astronomer.io"},
		{"https://api.astronomer.io"},
		{"api.astronomer.io:443"},
	} {
		_, err := NewRouter("/nowhere", hosts, http.DefaultTransport)
		assert.Error(t, err, "%q", hosts)
	}
}

// A socket nobody is listening on is a clear error naming where the path came
// from, not a silent fall back to the network.
func TestMissingSocket(t *testing.T) {
	direct := &directRecorder{}
	rt, err := NewRouter(filepath.Join(t.TempDir(), "absent"), DefaultHosts, direct)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.astronomer.io/v1/x", http.NoBody)
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: rt}).Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), SocketEnv)
	assert.Empty(t, direct.hosts)
}

func TestInstall(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	t.Run("unset changes nothing", func(t *testing.T) {
		require.NoError(t, Install(env(nil)))
		assert.Same(t, original, http.DefaultTransport)
	})

	t.Run("a bad host list is an error and changes nothing", func(t *testing.T) {
		err := Install(env(map[string]string{SocketEnv: "/tmp/s", HostsEnv: "api*.example.com"}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), HostsEnv)
		assert.Same(t, original, http.DefaultTransport)
	})

	t.Run("set routes a default client through the socket", func(t *testing.T) {
		t.Cleanup(func() { http.DefaultTransport = original })
		path, got := listen(t)
		// An empty host list means the default, as an unset one does.
		require.NoError(t, Install(env(map[string]string{SocketEnv: path, HostsEnv: " "})))
		require.IsType(t, &Router{}, http.DefaultTransport)
		assert.Same(t, original, http.DefaultTransport.(*Router).direct)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.astronomer.io/v1/organizations", http.NoBody)
		require.NoError(t, err)
		resp, err := (&http.Client{}).Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, "api.astronomer.io", (<-got).host)
	})
}
