package httputil

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ctrl-C while a write is in flight leaves its outcome unknown, and the error
// is still a cancellation, so the run exits as interrupted.
func TestAWriteCanceledAfterItWasSentIsAnUnknownOutcome(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/organizations/o/deployments/dep9", strings.NewReader("{}"))
	require.NoError(t, err)
	resp, err := WithRateLimitRetries(&http.Client{}).Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.True(t, OutcomeUnknown(err), "%v", err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "astro deployment inspect dep9")
	assert.Equal(t, 1, strings.Count(err.Error(), srv.URL), "the URL appears once: %v", err)

	// Canceled before it was sent: nothing happened.
	before, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	req, err = http.NewRequestWithContext(before, http.MethodPost, srv.URL, strings.NewReader("{}"))
	require.NoError(t, err)
	resp, err = WithRateLimitRetries(&http.Client{}).Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.False(t, OutcomeUnknown(err))
}

func TestAnUnmappedPathNamesNoCommand(t *testing.T) {
	err := NormalizeAPIError(respFor(http.MethodPost, "/v1/organizations/o/audit-logs", http.StatusBadGateway), []byte(`{"message":"x"}`))
	assert.Contains(t, err.Error(), "may have been applied; check its state before running this again")
	assert.NotContains(t, err.Error(), "matching")
}
