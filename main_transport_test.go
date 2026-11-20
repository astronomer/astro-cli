package main

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// The transport main installs refuses a redirect from https to plain http for
// every client that names no transport of its own. The request is one a
// client builds to follow such a redirect (its Response is the redirect), so
// the refusal comes before any connection is made.
func TestInstallTransportRefusesADowngradeRedirect(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	require.NoError(t, installTransport(func(string) string { return "" }, nil))

	from := &http.Request{URL: &url.URL{Scheme: "https", Host: "redirect.invalid", Path: "/v1"}}
	req := &http.Request{
		Method:   http.MethodGet,
		URL:      &url.URL{Scheme: "http", Host: "redirect.invalid", Path: "/v1"},
		Header:   http.Header{"Authorization": {"Bearer tok"}},
		Response: &http.Response{StatusCode: http.StatusFound, Request: from},
	}
	_, err := http.DefaultTransport.RoundTrip(req) //nolint:bodyclose // it fails before a response exists
	require.ErrorIs(t, err, httputil.ErrDowngradeRedirect)
}
