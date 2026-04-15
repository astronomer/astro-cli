package airflowrt

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// CheckHealth polls the Airflow health endpoint until it responds with 200 or the timeout is reached.
// The provided context can be used to cancel the health check before the timeout expires.
var CheckHealth = func(ctx context.Context, port string, timeout time.Duration) error {
	url := fmt.Sprintf("http://localhost:%s/api/v2/monitor/health", port)

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
