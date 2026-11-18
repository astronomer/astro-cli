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
	// rateLimitDrainBytes is how much of a 429's body is read before it is closed, so its connection can be reused.
	rateLimitDrainBytes = 4096
)

// The wait before each retry is picked between these, times the attempt number.
// The API counts each token's requests per second, so a wait of at least a
// second lands in a new window.
var (
	rateLimitWaitMin = time.Second
	rateLimitWaitMax = 2 * time.Second
)

// WithRateLimitRetries returns a client that resends a request the Astro API
// refuses with 429 Too Many Requests. The API limits the requests each token
// makes per second, which CI jobs sharing a token can briefly exceed.
func WithRateLimitRetries(c *http.Client) *http.Client {
	retrying := retryablehttp.NewClient()
	retrying.HTTPClient = c
	retrying.RetryMax = rateLimitRetries
	retrying.RetryWaitMin = rateLimitWaitMin
	retrying.RetryWaitMax = rateLimitWaitMax
	retrying.Backoff = retryablehttp.RateLimitLinearJitterBackoff
	retrying.CheckRetry = retryRateLimited
	retrying.ErrorHandler = retryablehttp.PassthroughErrorHandler
	retrying.Logger = nil
	return retrying.StandardClient()
}

// RetryRateLimited calls send, and calls it again after the same waits as
// WithRateLimitRetries each time the API answers 429 Too Many Requests. It is
// for a request whose body send builds afresh on each call, which
// WithRateLimitRetries would read whole to be able to resend it.
func RetryRateLimited(ctx httpContext.Context, send func() (*http.Response, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := send()
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt == rateLimitRetries {
			return resp, err
		}
		wait := retryablehttp.RateLimitLinearJitterBackoff(rateLimitWaitMin, rateLimitWaitMax, attempt, resp)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, rateLimitDrainBytes))
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

// retryRateLimited resends only a 429, which the API refuses before acting on
// the request, so a write is never repeated.
func retryRateLimited(ctx httpContext.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return err == nil && resp.StatusCode == http.StatusTooManyRequests, nil
}
