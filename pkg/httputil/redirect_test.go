package httputil

import (
	"net/http"
	"net/http/httptest"
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
