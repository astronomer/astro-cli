package airflowrt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// healthPollInterval is how often the wait re-asks. The first ask does not
// wait for it; see CheckHealth.
const healthPollInterval = time.Second

// ErrHealthTimeout reports that Airflow did not answer in the time allowed.
//
// A sentinel rather than a string, so the layer that owns the wait can say how
// to lengthen it. This module does not: it is also what Astro Desktop builds
// on, and desktop takes the duration from its own settings. The CLI names its
// environment variable in cmd/local; naming it here would advise a desktop
// user to set something nothing reads.
var ErrHealthTimeout = errors.New("health check timed out")

// HealthCheckConfig holds version-specific options for CheckHealth.
// Using a struct lets callers add new fields without changing the function signature.
type HealthCheckConfig struct {
	// AirflowMajorVersion selects the health endpoint(s): "2" tries both
	// /api/v2/monitor/health and /health (AF2 fallback), anything else
	// (including empty string) uses only /api/v2/monitor/health (AF3).
	AirflowMajorVersion string
}

// CheckHealth polls the Airflow health endpoint until it responds with 200 or the timeout is reached.
// For AF2 (cfg.AirflowMajorVersion == "2") it tries both /api/v2/monitor/health (Astronomer-patched
// runtime builds) and /health (vanilla Apache Airflow) on each tick. For AF3 (or empty) it only
// checks /api/v2/monitor/health.
// The provided context can be used to cancel the health check before the timeout expires.
var CheckHealth = func(ctx context.Context, port string, timeout time.Duration, cfg HealthCheckConfig) error {
	baseURL := fmt.Sprintf("http://localhost:%s", port)

	// Build list of health paths to probe on each tick.
	// Astronomer runtime AF2 builds (e.g. 2.10.5+astro.4) moved /health to
	// /api/v2/monitor/health, so we try the new path first and fall back to
	// the legacy /health for vanilla Apache AF2.
	healthPaths := []string{"/api/v2/monitor/health"}
	if cfg.AirflowMajorVersion == "2" {
		healthPaths = append(healthPaths, "/health")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	// Once before the loop, because the loop's first answer arrives a whole
	// poll interval in. That was invisible while the timeout was five minutes
	// and is not now that a caller can choose it: a timeout shorter than the
	// interval expired having issued no request at all, so a wait of 500ms
	// reported that Airflow had not come up without ever asking it. At exactly
	// one second the deadline and the tick are both ready and the choice
	// between them is random, which is worse than either.
	if healthy(ctx, client, baseURL, healthPaths) {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			// Only a deadline is a timeout. ctx is the caller's with a timeout
			// hung on it, so this fires just as readily when the caller
			// cancels — and reporting "timed out after 5m0s" to somebody who
			// pressed Ctrl-C four seconds in describes a wait that did not
			// happen. The caller's own error is the true one; localstandalone
			// turns it into a sentence about what is still running.
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ctx.Err()
			}
			return fmt.Errorf("%w after %s — Airflow may still be starting", ErrHealthTimeout, timeout)
		case <-ticker.C:
			if healthy(ctx, client, baseURL, healthPaths) {
				return nil
			}
		}
	}
}

// healthy reports whether any of the paths answers 200.
func healthy(ctx context.Context, client *http.Client, baseURL string, paths []string) bool {
	for _, path := range paths {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, http.NoBody)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return false
}
