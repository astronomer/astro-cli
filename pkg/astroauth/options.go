package astroauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// requestTimeout bounds one astroauth request, from dialing to the end of the
// body. A login or a refresh is a handful of small requests; one that has not
// finished in this long is a stalled network or proxy, and waiting longer only
// leaves a deploy or a login hanging with nothing on the screen.
const requestTimeout = 30 * time.Second

// maxResponseBytes caps what a response body is read to. The auth config, a
// token response and a userinfo document are each well under a kilobyte.
const maxResponseBytes = 1 << 20

var errResponseTooLarge = fmt.Errorf("response is larger than %d bytes", maxResponseBytes)

// errAuthRedirect is a token or userinfo endpoint answering with a redirect.
// Both answer for themselves; one that redirects is misconfigured, and
// following it would send what the request carries (the code, the refresh
// token, the access token) on to wherever it points.
var errAuthRedirect = errors.New("auth endpoint redirected; not following it")

// defaultHTTPClient is the client a request uses when the caller gives none.
// It names no Transport, so it reads http.DefaultTransport per request, as
// http.DefaultClient did: whatever the process installed there (the CLI's
// ASTRO_HTTP_UNIX_SOCKET routing) still applies.
var defaultHTTPClient = &http.Client{Timeout: requestTimeout}

// requestOptions holds the network settings for an astroauth request. The zero
// value reaches the live Astronomer API with defaultHTTPClient.
type requestOptions struct {
	httpClient *http.Client
	baseURL    string // overrides the API base URL FetchAuthConfig derives from the domain
}

// RequestOption overrides a network default. Production callers pass none and
// get the live defaults; the tests' WithHTTPClient and WithBaseURL
// (options_test.go) point a request at an httptest server.
type RequestOption func(*requestOptions)

func resolveOptions(opts []RequestOption) requestOptions {
	o := requestOptions{httpClient: defaultHTTPClient}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// decodeJSON decodes a response body into v, reading at most maxResponseBytes:
// a body past that is an error rather than memory spent on it.
func decodeJSON(body io.Reader, v any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponseBytes {
		return errResponseTooLarge
	}
	return json.Unmarshal(data, v)
}

// withoutRedirects is a copy of c that refuses to follow a redirect, for the
// requests that carry a credential (the token exchange and refresh, and
// userinfo's bearer token), meant for the endpoint they name and no other. The copy keeps c's Transport and Timeout, and c
// itself is left as it was.
func withoutRedirects(c *http.Client) *http.Client {
	cc := *c
	cc.CheckRedirect = func(*http.Request, []*http.Request) error { return errAuthRedirect }
	return &cc
}
