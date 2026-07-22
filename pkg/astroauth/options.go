package astroauth

import "net/http"

// requestOptions holds the network settings for an astroauth request. The zero
// value reaches the live Astronomer API with http.DefaultClient.
type requestOptions struct {
	httpClient *http.Client
	baseURL    string // overrides the API base URL FetchAuthConfig derives from the domain
}

// RequestOption overrides a network default. Production callers pass none and
// get the live defaults; tests use these to point at an httptest server.
type RequestOption func(*requestOptions)

// WithHTTPClient sets the HTTP client used for the request.
func WithHTTPClient(c *http.Client) RequestOption {
	return func(o *requestOptions) { o.httpClient = c }
}

// WithBaseURL overrides the base URL FetchAuthConfig builds from the domain,
// for example "http://127.0.0.1:port" instead of "https://api.<domain>".
func WithBaseURL(u string) RequestOption {
	return func(o *requestOptions) { o.baseURL = u }
}

func resolveOptions(opts []RequestOption) requestOptions {
	o := requestOptions{httpClient: http.DefaultClient}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
