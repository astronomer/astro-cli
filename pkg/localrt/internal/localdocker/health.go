package localdocker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultHealthTimeout bounds the wait for the api-server to come up.
	// First runs pull the runtime image, so this is generous.
	defaultHealthTimeout = 5 * time.Minute
	healthPollInterval   = time.Second
	healthRequestTimeout = 5 * time.Second
)

// ErrHealthTimeout reports that Airflow did not become healthy in time.
// The containers keep starting in the background: the project is still up,
// registered, and stoppable.
var ErrHealthTimeout = errors.New("timed out waiting for Airflow to become healthy")

// healthURLs is where an Airflow generation answers that it is up. Airflow 3
// serves the monitor endpoint under the v2 API. Astronomer's newer Airflow 2
// runtimes serve that same path; older ones serve /health at the root, so
// Airflow 2 is polled on both and the first 200 wins (pkg/airflowrt does the
// same for standalone).
func healthURLs(port int, major string) []string {
	base := fmt.Sprintf("http://localhost:%d", port)
	urls := []string{base + "/api/v2/monitor/health"}
	if major == airflow2 {
		urls = append(urls, base+"/health")
	}
	return urls
}

// waitHealthy polls the urls until one returns 200, the timeout passes, or
// ctx is canceled. Ported from v1's checkWebserverHealth, minus the printing.
func waitHealthy(ctx context.Context, urls []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: healthRequestTimeout}
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("%w after %s; the containers keep starting in the background — `astro local logs` shows their progress", ErrHealthTimeout, timeout)
			}
			return ctx.Err()
		case <-ticker.C:
			for _, url := range urls {
				if healthOK(ctx, client, url) {
					return nil
				}
			}
		}
	}
}

func healthOK(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
