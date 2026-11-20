package httputil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewHTTPClient's client, with only its Transport swapped for one that trusts
// the test server (as Houston's swaps in its own), refuses an https-to-http
// redirect and still follows an https-to-https one, Authorization included.
func TestNewHTTPClientRefusesADowngradeRedirect(t *testing.T) {
	var plainReached atomic.Bool
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plainReached.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()

	var gotAuth atomic.Value
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downgrade":
			http.Redirect(w, r, plain.URL+"/v1", http.StatusFound)
		case "/moved":
			http.Redirect(w, r, "/v1", http.StatusFound)
		default:
			gotAuth.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tlsSrv.Close()

	c := NewHTTPClient()
	c.HTTPClient.Transport = tlsSrv.Client().Transport
	headers := map[string]string{"Authorization": "Bearer tok"}

	_, err := c.Do(&DoOptions{Method: http.MethodGet, Path: tlsSrv.URL + "/downgrade", Headers: headers})
	require.ErrorIs(t, err, ErrDowngradeRedirect)
	assert.False(t, plainReached.Load(), "the plain-http target was sent the request")

	resp, err := c.Do(&DoOptions{Method: http.MethodGet, Path: tlsSrv.URL + "/moved", Headers: headers})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "Bearer tok", gotAuth.Load())
}

// A client that starts on plain http follows plain http redirects as before:
// a local or dev endpoint is not refused.
func TestNewHTTPClientFollowsPlainRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := NewHTTPClient().Do(&DoOptions{Method: http.MethodGet, Path: srv.URL + "/old"})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "/new", resp.Request.URL.Path)
}

// RefuseDowngrades refuses the redirect for a client with the default
// redirect policy, which follows one from https to plain http and keeps
// Authorization on it: neither a GET with a token (302) nor a POST that a 307
// resends with its body reaches the plain-http server. An https-to-https
// redirect, and a plain-http one, are followed as before.
func TestRefuseDowngradesStopsAClientWithTheDefaultPolicy(t *testing.T) {
	var plainReached atomic.Bool
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		if r.URL.Path != "/new" {
			plainReached.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()

	var gotAuth atomic.Value
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downgrade":
			http.Redirect(w, r, plain.URL+"/v1", http.StatusFound)
		case "/downgrade-post":
			http.Redirect(w, r, plain.URL+"/login", http.StatusTemporaryRedirect)
		case "/moved":
			http.Redirect(w, r, "/v1", http.StatusFound)
		default:
			gotAuth.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tlsSrv.Close()

	client := &http.Client{Transport: RefuseDowngrades(tlsSrv.Client().Transport)}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tlsSrv.URL+"/downgrade", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer tok")
	_, err = client.Do(req)
	require.ErrorIs(t, err, ErrDowngradeRedirect)

	req, err = http.NewRequestWithContext(t.Context(), http.MethodPost, tlsSrv.URL+"/downgrade-post", strings.NewReader("password=pw"))
	require.NoError(t, err)
	_, err = client.Do(req)
	require.ErrorIs(t, err, ErrDowngradeRedirect)
	assert.False(t, plainReached.Load(), "the plain-http target was sent a request")

	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, tlsSrv.URL+"/moved", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "Bearer tok", gotAuth.Load())

	plainClient := &http.Client{Transport: RefuseDowngrades(http.DefaultTransport)}
	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, plain.URL+"/old", http.NoBody)
	require.NoError(t, err)
	resp, err = plainClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "/new", resp.Request.URL.Path)
}

// recordingTransport answers every request it is sent with 200, and counts
// them.
type recordingTransport struct{ sent int }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.sent++
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

// closeTracker is a request body that remembers being closed.
type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error { c.closed = true; return nil }

// The guard judges each hop by the one before it: a hop that leaves https is
// refused wherever it falls in a chain (the last of http, https, http too),
// unsent and with its body closed. A request that is not a redirect's, or
// whose redirect names no request, goes on as it is.
func TestRefuseDowngradesJudgesEachHop(t *testing.T) {
	hop := func(scheme string) *http.Request {
		return &http.Request{URL: &url.URL{Scheme: scheme, Host: "airflow.invalid", Path: "/"}}
	}
	redirectFrom := func(prev *http.Request) *http.Response {
		return &http.Response{StatusCode: http.StatusTemporaryRedirect, Request: prev}
	}
	for _, tc := range []struct {
		name    string
		from    *http.Response
		scheme  string
		refused bool
	}{
		{"https to http", redirectFrom(hop("https")), "http", true},
		{"http, https, then http", redirectFrom(&http.Request{URL: hop("https").URL, Response: redirectFrom(hop("http"))}), "http", true},
		{"https to https", redirectFrom(hop("https")), "https", false},
		{"http to http", redirectFrom(hop("http")), "http", false},
		{"a redirect that names no request", &http.Response{StatusCode: http.StatusFound}, "http", false},
		{"not a redirect's request", nil, "http", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := &recordingTransport{}
			body := &closeTracker{Reader: strings.NewReader("password=pw")}
			req := hop(tc.scheme)
			req.Method, req.Header, req.Body, req.Response = http.MethodPost, http.Header{}, body, tc.from

			resp, err := RefuseDowngrades(next).RoundTrip(req)
			if tc.refused {
				require.ErrorIs(t, err, ErrDowngradeRedirect)
				assert.Nil(t, resp)
				assert.Zero(t, next.sent, "the refused request was sent")
				assert.True(t, body.closed, "the refused request's body was left open")
				return
			}
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, 1, next.sent)
		})
	}
}
