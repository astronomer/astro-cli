package localdocker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
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
//
// airflowrt's, not one of this package's own, so a caller holding an error from
// either engine asks once. Which engine ran is the runtime's business, and the
// question "did the wait run out" has the same answer and the same remedy
// whichever did.
var ErrHealthTimeout = airflowrt.ErrHealthTimeout

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

// interrupted reports a health wait cut short by a canceled context.
//
// Unwraps to the context's error, so errors.Is(err, context.Canceled) still
// holds for anything deciding whether this was a cancellation.
type interrupted struct{ err error }

func (e *interrupted) Error() string {
	return "interrupted: the containers keep starting in the background — " +
		"`astro local logs` shows their progress, `astro local stop` ends them"
}

func (e *interrupted) Unwrap() error { return e.err }

// waitHealthy polls the urls until one returns 200, the timeout passes, or
// ctx is canceled. Ported from v1's checkWebserverHealth, minus the printing.
func waitHealthy(ctx context.Context, urls []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: healthRequestTimeout}
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	// Once before the loop; see the same call in airflowrt.CheckHealth. A
	// timeout shorter than the poll interval otherwise expires having issued
	// no request, which was unreachable while the wait was a hardcoded five
	// minutes and is not now that a caller can choose it.
	if anyHealthy(ctx, client, urls) {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				// No environment variable here; see airflowrt.ErrHealthTimeout
				// for why the name of one belongs to the CLI and not to a
				// module Astro Desktop also builds on.
				return fmt.Errorf("%w after %s; the containers keep starting in the background — "+
					"`astro local logs` shows their progress", ErrHealthTimeout, timeout)
			}
			// The same thing the deadline branch above says, for the same
			// reason, because the same thing is true: Start published the
			// record and the route before this wait and does not tear them
			// down on the way out, so the containers keep coming up. Returning
			// the context's error bare said "context canceled" — the name of a
			// Go value — to somebody who had just pressed Ctrl-C and could not
			// tell from it whether an Airflow was now running.
			return &interrupted{err: ctx.Err()}
		case <-ticker.C:
			if anyHealthy(ctx, client, urls) {
				return nil
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

// anyHealthy reports whether any of the urls answers.
func anyHealthy(ctx context.Context, client *http.Client, urls []string) bool {
	for _, url := range urls {
		if healthOK(ctx, client, url) {
			return true
		}
	}
	return false
}
