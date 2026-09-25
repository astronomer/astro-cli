package astroauth

import "net/http"

// requestOptions holds the network settings for an astroauth request. The zero
// value reaches the live Astronomer API with http.DefaultClient.
type requestOptions struct {
	httpClient *http.Client
	baseURL    string // overrides the API base URL FetchAuthConfig derives from the domain
}

// RequestOption overrides a network default. Production callers pass none and
// get the live defaults; the tests' WithHTTPClient and WithBaseURL
// (options_test.go) point a request at an httptest server.
type RequestOption func(*requestOptions)

func resolveOptions(opts []RequestOption) requestOptions {
	o := requestOptions{httpClient: http.DefaultClient}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
