package astroauth

import "net/http"

// WithHTTPClient sets the HTTP client used for the request.
func WithHTTPClient(c *http.Client) RequestOption {
	return func(o *requestOptions) { o.httpClient = c }
}

// WithBaseURL overrides the base URL FetchAuthConfig builds from the domain,
// for example "http://127.0.0.1:port" instead of "https://api.<domain>".
func WithBaseURL(u string) RequestOption {
	return func(o *requestOptions) { o.baseURL = u }
}
