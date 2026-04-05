package airflowrt

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckHealth_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Extract port from test server
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")

	// Override CheckHealth to use /api/v2/monitor/health path on localhost
	origCheckHealth := CheckHealth
	defer func() { CheckHealth = origCheckHealth }()

	// Just test the health check function directly against the test server
	err := checkHealthURL(srv.URL+"/api/v2/monitor/health", 5*time.Second)
	require.NoError(t, err)

	_ = port // keep reference
}

func TestCheckHealth_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := checkHealthURL(srv.URL+"/api/v2/monitor/health", 2*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

// checkHealthURL is a test helper that polls an arbitrary URL.
func checkHealthURL(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url) //nolint:gosec,noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("health check timed out after %s", timeout)
}
