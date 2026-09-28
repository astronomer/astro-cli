package airflowapi

import (
	"context"
	"errors"
	"net/http"
)

// HealthComponent is one Airflow component's health.
type HealthComponent struct {
	Status string `json:"status"`
}

// Health is what Airflow reports about itself.
type Health struct {
	MetaDatabase HealthComponent `json:"metadatabase"`
	Scheduler    HealthComponent `json:"scheduler"`
	Triggerer    HealthComponent `json:"triggerer"`
	DAGProcessor HealthComponent `json:"dag_processor"`
}

// Where health lives moved between releases: Airflow 3 and Astronomer's
// patched Airflow 2 runtime images serve monitor/health under the Airflow 3
// API base, vanilla Apache Airflow 2 serves /health at the server root. Both
// are asked, newest first — the order pkg/airflowrt probes them in — and
// which build this is comes from the answer rather than from a version.
//
// Naming the generation on the first request rather than writing out
// /api/v2/monitor/health keeps it a request a transport composes: a door
// that reaches only one generation's API still addresses it, and a door with
// no server root answers the second with a 404 the client reads as
// ErrNotServed.
var healthProbes = []Request{
	{Method: http.MethodGet, Path: "/monitor/health", Generation: Airflow3},
	{Method: http.MethodGet, Path: "/health", Generation: GenerationNone},
}

// Health reports Airflow's own view of its components.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var lastErr error
	for _, probe := range healthProbes {
		resp, err := c.send(ctx, probe)
		if err != nil {
			// A missing path means this build serves health elsewhere; any
			// other refusal is the answer.
			if !errors.Is(err, ErrNotFound) {
				return Health{}, err
			}
			lastErr = err
			continue
		}
		var health Health
		if err := resp.Decode(&health); err != nil {
			return Health{}, err
		}
		return health, nil
	}
	// Neither path answered: this build serves health somewhere else, or not
	// at all, which is a refusal about the endpoint rather than about
	// anything the caller named.
	var status *StatusError
	if errors.As(lastErr, &status) {
		notServed := *status
		notServed.Collection = true
		return Health{}, &notServed
	}
	return Health{}, lastErr
}
