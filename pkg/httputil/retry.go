package httputil

import (
	httpContext "context"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	rateLimitRetries = 3
	// rateLimitDrainBytes is how much of a 429's body is read before it is
	// closed, so its connection can be reused.
	rateLimitDrainBytes = 4096
)

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
// refuses with 429 Too Many Requests, and a read (GET or HEAD) that failed in
// transit or got a 502, 503 or 504, a few times with a growing wait that
// honors Retry-After. The API limits the requests each token makes per second,
// which CI jobs sharing a token can briefly exceed.
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
	retrying.CheckRetry = retryTransient
	retrying.ErrorHandler = retryablehttp.PassthroughErrorHandler
	retrying.Logger = nil
	std := retrying.StandardClient()
	// A write that fails in transit may have been applied; its error says so.
	std.Transport = outcomeTransport{next: std.Transport}
	return std
}

// RetryRateLimited calls send, and calls it again after the same waits as
// WithRateLimitRetries (Retry-After honored, capped at rateLimitWaitCap) each
// time the API answers 429 Too Many Requests. It is for a request whose body
// send builds afresh on each call, which WithRateLimitRetries would read whole
// to be able to resend it. It resends nothing else, and stops waiting when ctx
// ends.
func RetryRateLimited(ctx httpContext.Context, send func() (*http.Response, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := send()
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt == rateLimitRetries {
			return resp, err
		}
		wait := min(retryablehttp.RateLimitLinearJitterBackoff(rateLimitWaitMin, rateLimitWaitMax, attempt, resp), rateLimitWaitCap)
		io.Copy(io.Discard, io.LimitReader(resp.Body, rateLimitDrainBytes)) //nolint:errcheck // draining a body to reuse its connection
		resp.Body.Close()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// retryTransient decides whether to resend a request.
//
// A 429 is resent for every method: the API refuses it before acting on the
// request, so a write is never repeated. A read (GET or HEAD) is also resent
// when it failed in transit or the server answered 502, 503 or 504, which
// say only that it could not take the request now. Any other method is never
// resent for those: the server may have acted on it before failing, so the
// outcome is unknown and repeating it could apply the change twice.
//
// A request whose context ends needs nothing more here: retryablehttp stops
// waiting for a retry when it does, and an error alongside a response would
// leave the response unclosed.
func retryTransient(ctx httpContext.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, nil
	}
	if err == nil && resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}
	if !isRead(resp, err) {
		return false, nil
	}
	if err != nil {
		return IsTransportError(err), nil
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true, nil
	}
	return false, nil
}

// isRead reports whether the request that produced resp, or err, was a GET or
// HEAD. A transport error carries its method in the *url.Error's Op.
func isRead(resp *http.Response, err error) bool {
	if resp != nil && resp.Request != nil {
		return resp.Request.Method == http.MethodGet || resp.Request.Method == http.MethodHead
	}
	if m, ok := methodOfTransportError(err); ok {
		return m == http.MethodGet || m == http.MethodHead
	}
	return false
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
