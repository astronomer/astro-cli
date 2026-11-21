package httputil

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryRateLimitedCapsTheWaitARetryAfterAsksFor(t *testing.T) {
	origCap := rateLimitWaitCap
	rateLimitWaitCap = time.Millisecond
	t.Cleanup(func() { rateLimitWaitCap = origCap })

	calls := 0
	resp, err := RetryRateLimited(context.Background(), func() (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"3600"}},
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, calls)
}
