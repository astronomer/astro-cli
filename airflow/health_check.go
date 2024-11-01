package airflow

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// checkWebserverHealth is a container runtime agnostic way to check if
// the webserver is healthy.
var checkWebserverHealth = func(timeout time.Duration) error {
	// Airflow webserver should be hosted at localhost
	// from the perspective of the CLI running on the host machine.
	url := "http://localhost:8080/health"

	// Create a cancellable context with the specified
	// timeout for the healthcheck.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create an HTTP client for healthcheck requests.
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// This ticker represents the interval of our healthcheck.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Run a loop that we'll exit when we get a 200 status code,
	// or when the context is cancelled due to reaching the specified timeout.
	for {
		select {
		// This means the healthcheck has reached its deadline.
		// We return an error message to the user.
		case <-ctx.Done():
			return fmt.Errorf("There might be a problem with your project starting up. "+
				"The webserver health check timed out after %s but your project will continue trying to start. "+
				"Run 'astro dev logs --webserver | --scheduler' for details.\n"+
				"Try again or use the --wait flag to increase the time out", timeout)
		// This fires on every tick of our timer to run the healthcheck.
		// We return successfully from this function when we get a 200 status code.
		case <-ticker.C:
			statusCode, _ := healthCheck(ctx, client, url)
			if statusCode == http.StatusOK {
				return nil
			}
		}
	}
}

// healthCheck is a helper function to execute an HTTP request
// and return the status code or an error.
func healthCheck(ctx context.Context, client *http.Client, url string) (int, error) {
	// Create a new HTTP GET request with the specified context.
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	// Make the request to the healthcheck endpoint.
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Return the status code integer.
	return resp.StatusCode, nil
}
