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

// retryRateLimited resends only a 429, which the API refuses before acting on
// the request, so a write is never repeated.
func retryRateLimited(ctx httpContext.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return err == nil && resp.StatusCode == http.StatusTooManyRequests, nil
}
