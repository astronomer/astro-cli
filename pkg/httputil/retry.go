package httputil

import (
	httpContext "context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const rateLimitRetries = 3

// The wait before each retry is picked between these, times the attempt number.
// The API counts each token's requests per second, so a wait of at least a
// second lands in a new window.
var (
	rateLimitWaitMin = time.Second
	rateLimitWaitMax = 2 * time.Second
)

// rateLimitWaitCap bounds a wait the API asks for in Retry-After, which is
// otherwise taken as given: retries print nothing, so a header of minutes
// would hang the command with no word of why.
var rateLimitWaitCap = 10 * time.Second

// WithRateLimitRetries returns a client that resends a request the Astro API
// refuses with 429 Too Many Requests. The API limits the requests each token
// makes per second, which CI jobs sharing a token can briefly exceed.
func WithRateLimitRetries(c *http.Client) *http.Client {
	retrying := retryablehttp.NewClient()
	// Not c itself: retryablehttp closes its client's idle connections after
	// any request that fails, and c's are usually http.DefaultTransport's,
	// which every other client in the process shares.
	retrying.HTTPClient = &http.Client{
		Transport:     keepIdleTransport{c},
		CheckRedirect: c.CheckRedirect,
		Jar:           c.Jar,
		Timeout:       c.Timeout,
	}
	retrying.RetryMax = rateLimitRetries
	retrying.RetryWaitMin = rateLimitWaitMin
	retrying.RetryWaitMax = rateLimitWaitMax
	retrying.Backoff = func(lo, hi time.Duration, attempt int, resp *http.Response) time.Duration {
		return min(retryablehttp.RateLimitLinearJitterBackoff(lo, hi, attempt, resp), rateLimitWaitCap)
	}
	retrying.CheckRetry = retryRateLimited
	retrying.ErrorHandler = retryablehttp.PassthroughErrorHandler
	retrying.Logger = nil
	return retrying.StandardClient()
}

// retryRateLimited resends only a 429, which the API refuses before acting on
// the request, so a write is never repeated. A request whose context ends
// needs nothing here: retryablehttp stops waiting for a retry when it does,
// and an error alongside a response would leave the response unclosed.
func retryRateLimited(_ httpContext.Context, resp *http.Response, err error) (bool, error) {
	return err == nil && resp.StatusCode == http.StatusTooManyRequests, nil
}

// keepIdleTransport sends through the wrapped client's transport, read per
// request as the client would, and has no CloseIdleConnections of its own, so
// closing the retrying client's idle connections leaves that transport's alone.
type keepIdleTransport struct{ c *http.Client }

func (t keepIdleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.c.Transport != nil {
		return t.c.Transport.RoundTrip(r)
	}
	return http.DefaultTransport.RoundTrip(r)
}
