package httputil

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRateLimitRetries(t *testing.T) {
	origMin, origMax := rateLimitWaitMin, rateLimitWaitMax
	rateLimitWaitMin, rateLimitWaitMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { rateLimitWaitMin, rateLimitWaitMax = origMin, origMax })

	// serve answers with the given statuses in turn, recording each request body
	serve := func(t *testing.T, statuses ...int) (*httptest.Server, *[]string) {
		t.Helper()
		var calls atomic.Int32
		var bodies []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(body))
			n := int(calls.Add(1)) - 1
			w.WriteHeader(statuses[min(n, len(statuses)-1)])
		}))
		t.Cleanup(srv.Close)
		return srv, &bodies
	}

	t.Run("resends a rate-limited request with its body", func(t *testing.T) {
		srv, bodies := serve(t, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusOK)
		resp, err := WithRateLimitRetries(&http.Client{}).Post(srv.URL, "application/json", bytes.NewReader([]byte(`{"a":1}`)))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{`{"a":1}`, `{"a":1}`, `{"a":1}`}, *bodies)
	})

	t.Run("returns the 429 once the retries run out", func(t *testing.T) {
		srv, bodies := serve(t, http.StatusTooManyRequests)
		resp, err := WithRateLimitRetries(&http.Client{}).Get(srv.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.Len(t, *bodies, rateLimitRetries+1)
	})

	t.Run("does not resend other errors", func(t *testing.T) {
		srv, bodies := serve(t, http.StatusServiceUnavailable, http.StatusOK)
		resp, err := WithRateLimitRetries(&http.Client{}).Post(srv.URL, "application/json", bytes.NewReader([]byte(`{}`)))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.Len(t, *bodies, 1)
	})

	t.Run("stops waiting when the request is canceled", func(t *testing.T) {
		rateLimitWaitMin, rateLimitWaitMax = time.Hour, 2*time.Hour
		t.Cleanup(func() { rateLimitWaitMin, rateLimitWaitMax = time.Millisecond, 2*time.Millisecond })
		srv, _ := serve(t, http.StatusTooManyRequests)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
		require.NoError(t, err)
		_, err = WithRateLimitRetries(&http.Client{}).Do(req) //nolint:bodyclose
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestRetryRateLimited(t *testing.T) {
	origMin, origMax := rateLimitWaitMin, rateLimitWaitMax
	rateLimitWaitMin, rateLimitWaitMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { rateLimitWaitMin, rateLimitWaitMax = origMin, origMax })

	// sends answers with the given statuses in turn, counting the calls
	sends := func(calls *int, statuses ...int) func() (*http.Response, error) {
		return func() (*http.Response, error) {
			status := statuses[min(*calls, len(statuses)-1)]
			*calls++
			return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}
	}

	t.Run("calls send again for a rate-limited request", func(t *testing.T) {
		calls := 0
		resp, err := RetryRateLimited(context.Background(), sends(&calls, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusOK))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 3, calls)
	})

	t.Run("returns the 429 once the retries run out", func(t *testing.T) {
		calls := 0
		resp, err := RetryRateLimited(context.Background(), sends(&calls, http.StatusTooManyRequests))
		require.NoError(t, err)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.Equal(t, rateLimitRetries+1, calls)
	})

	t.Run("does not call send again for other answers or errors", func(t *testing.T) {
		calls := 0
		resp, err := RetryRateLimited(context.Background(), sends(&calls, http.StatusServiceUnavailable, http.StatusOK))
		require.NoError(t, err)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.Equal(t, 1, calls)

		calls = 0
		_, err = RetryRateLimited(context.Background(), func() (*http.Response, error) { //nolint:bodyclose
			calls++
			return nil, io.ErrUnexpectedEOF
		})
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		assert.Equal(t, 1, calls)
	})

	t.Run("stops waiting when the context is done", func(t *testing.T) {
		rateLimitWaitMin, rateLimitWaitMax = time.Hour, 2*time.Hour
		t.Cleanup(func() { rateLimitWaitMin, rateLimitWaitMax = time.Millisecond, 2*time.Millisecond })
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		calls := 0
		_, err := RetryRateLimited(ctx, sends(&calls, http.StatusTooManyRequests)) //nolint:bodyclose
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 1, calls)
	})
}
