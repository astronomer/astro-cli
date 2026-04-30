package airflowrt

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// HealthCheckConfig holds version-specific options for CheckHealth.
// Using a struct lets callers add new fields without changing the function signature.
type HealthCheckConfig struct {
	// AirflowMajorVersion selects the health endpoint: "2" uses /health (AF2),
	// anything else (including empty string) uses /api/v2/monitor/health (AF3).
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
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out after %s — Airflow may still be starting. Check logs for details", timeout)
		case <-ticker.C:
			for _, path := range healthPaths {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
				if err != nil {
					continue
				}
				resp, err := client.Do(req)
				if err != nil {
					continue
				}
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}
