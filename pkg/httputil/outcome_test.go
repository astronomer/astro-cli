package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func respFor(method, path string, status int) *http.Response {
	req, _ := http.NewRequest(method, "https://api.astronomer.io"+path, http.NoBody)
	return &http.Response{StatusCode: status, Request: req}
}

// A 5xx to a write leaves its outcome unknown, and the error says so and names
// the command that shows the state; a 5xx to a read is only a failure.
func TestA5xxToAWriteIsAnUnknownOutcome(t *testing.T) {
	const path = "/v1/organizations/org1/deployments/dep1"
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		err := NormalizeAPIError(respFor(method, path, http.StatusBadGateway), []byte(`{"message":"upstream"}`))
		assert.True(t, OutcomeUnknown(err), method)
		assert.True(t, OutcomeUnknown(fmt.Errorf("updating: %w", err)), method)
		assert.Contains(t, err.Error(), "upstream; the change may have been applied; check it with astro deployment inspect dep1")
		assert.NotContains(t, err.Error(), "try again")
	}

	err := NormalizeAPIError(respFor(http.MethodPost, "/v1/organizations/org1/deployments", http.StatusInternalServerError), []byte("<html>"))
	assert.Contains(t, err.Error(), "astro deployment list")
	assert.True(t, errors.Is(err, ErrorRequest))

	for _, resp := range []*http.Response{
		respFor(http.MethodGet, path, http.StatusBadGateway),
		respFor(http.MethodPost, path, http.StatusBadRequest),
		respFor(http.MethodPost, path, http.StatusConflict),
		{StatusCode: http.StatusInternalServerError},
	} {
		err := NormalizeAPIError(resp, []byte(`{"message":"no"}`))
		assert.False(t, OutcomeUnknown(err))
		assert.NotContains(t, err.Error(), "may have been applied")
	}
}

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

// A write whose connection broke after it was sent is an unknown outcome; one
// that never connected is not.
func TestATransportFailureOfAWriteIsAnUnknownOutcomeOnlyOnceSent(t *testing.T) {
	post := func(err error) error {
		c := WithRateLimitRetries(&http.Client{Transport: failingTransport{err}})
		resp, e := c.Post("https://api.astronomer.io/v1/organizations/o/workspaces", "application/json", strings.NewReader("{}"))
		if resp != nil {
			resp.Body.Close()
		}
		return e
	}

	err := post(io.ErrUnexpectedEOF)
	assert.True(t, OutcomeUnknown(err), "%v", err)
	assert.True(t, IsTransportError(err), "the wrapped failure is still a transport error")
	assert.Contains(t, err.Error(), "astro workspace list")

	refused := post(&net.OpError{Op: "dial", Err: errors.New("refused")})
	assert.False(t, OutcomeUnknown(refused), "a request that never connected changed nothing")
}

func TestAReadTransportFailureIsNeverAnUnknownOutcome(t *testing.T) {
	origMin, origMax := rateLimitWaitMin, rateLimitWaitMax
	rateLimitWaitMin, rateLimitWaitMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { rateLimitWaitMin, rateLimitWaitMax = origMin, origMax })
	c := WithRateLimitRetries(&http.Client{Transport: failingTransport{io.ErrUnexpectedEOF}})
	resp, err := c.Get("https://api.astronomer.io/v1/organizations/o/workspaces")
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.False(t, OutcomeUnknown(err))
}

func TestReadsRetryTransientFailuresAndWritesDoNot(t *testing.T) {
	origMin, origMax := rateLimitWaitMin, rateLimitWaitMax
	rateLimitWaitMin, rateLimitWaitMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { rateLimitWaitMin, rateLimitWaitMax = origMin, origMax })

	serve := func(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32) {
		t.Helper()
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := int(calls.Add(1)) - 1
			w.WriteHeader(statuses[min(n, len(statuses)-1)])
		}))
		t.Cleanup(srv.Close)
		return srv, &calls
	}
	do := func(t *testing.T, method, url string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), method, url, http.NoBody)
		require.NoError(t, err)
		resp, err := WithRateLimitRetries(&http.Client{}).Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}

	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			srv, calls := serve(t, status, http.StatusOK)
			assert.Equal(t, http.StatusOK, do(t, method, srv.URL).StatusCode, "%s after %d", method, status)
			assert.EqualValues(t, 2, calls.Load())
		}
		srv, calls := serve(t, status, http.StatusOK)
		assert.Equal(t, status, do(t, http.MethodPost, srv.URL).StatusCode, "a write is not resent after %d", status)
		assert.EqualValues(t, 1, calls.Load())
	}

	t.Run("a read gives up after the retries", func(t *testing.T) {
		srv, calls := serve(t, http.StatusServiceUnavailable)
		assert.Equal(t, http.StatusServiceUnavailable, do(t, http.MethodGet, srv.URL).StatusCode)
		assert.EqualValues(t, rateLimitRetries+1, calls.Load())
	})

	t.Run("a read is not resent for other statuses", func(t *testing.T) {
		srv, calls := serve(t, http.StatusInternalServerError, http.StatusOK)
		assert.Equal(t, http.StatusInternalServerError, do(t, http.MethodGet, srv.URL).StatusCode)
		assert.EqualValues(t, 1, calls.Load())
	})

	t.Run("a read is resent after a dropped connection", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		assert.Equal(t, http.StatusOK, do(t, http.MethodGet, srv.URL).StatusCode)
		assert.EqualValues(t, 2, calls.Load())
	})

	t.Run("a canceled read is not resent", func(t *testing.T) {
		srv, calls := serve(t, http.StatusServiceUnavailable)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
		require.NoError(t, err)
		_, err = WithRateLimitRetries(&http.Client{}).Do(req) //nolint:bodyclose // canceled: no response
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, calls.Load())
	})
}
