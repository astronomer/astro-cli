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
// cfg.AirflowMajorVersion selects the correct endpoint: "/api/v2/monitor/health" for AF3,
// "/health" for AF2. An empty AirflowMajorVersion defaults to AF3.
// The provided context can be used to cancel the health check before the timeout expires.
var CheckHealth = func(ctx context.Context, port string, timeout time.Duration, cfg HealthCheckConfig) error {
	healthPath := "/api/v2/monitor/health"
	if cfg.AirflowMajorVersion == "2" {
		healthPath = "/health"
	}
	url := fmt.Sprintf("http://localhost:%s%s", port, healthPath)

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
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
