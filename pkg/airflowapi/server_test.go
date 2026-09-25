package airflowapi

import (
	"errors"
	"net/http"
	"testing"
)

func TestHealthPrefersTheMonitorPath(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/monitor/health",
		`{"metadatabase":{"status":"healthy"},"scheduler":{"status":"healthy"}}`)
	client := stub.client()

	health, err := client.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if health.MetaDatabase.Status != "healthy" || health.Scheduler.Status != "healthy" {
		t.Errorf("health = %+v, want both components healthy", health)
	}
	if stub.countRequests(http.MethodGet, "/health") != 0 {
		t.Error("the legacy path was tried after the monitor path answered")
	}
	if stub.countRequests(http.MethodGet, "/api/v2/version") != 0 {
		t.Error("health should not need the api generation")
	}
}

func TestHealthFallsBackToTheLegacyPath(t *testing.T) {
	// Airflow 3 and Astronomer's patched Airflow 2 runtime serve the monitor
	// path, plain Apache Airflow 2 serves /health. Which build this is comes
	// from the answer, not from a version.
	stub := newStub(t)
	stub.route(http.MethodGet, "/health", `{"metadatabase":{"status":"healthy"}}`)
	client := stub.client()

	health, err := client.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if health.MetaDatabase.Status != "healthy" {
		t.Errorf("health = %+v, want the legacy path's answer", health)
	}
	if stub.countRequests(http.MethodGet, "/api/v2/monitor/health") != 1 {
		t.Error("the monitor path should be tried first")
	}
}

func TestHealthReportsAnInstanceThatServesNeitherPath(t *testing.T) {
	stub := newStub(t)
	client := stub.client()

	_, err := client.Health(t.Context())
	if !errors.Is(err, ErrNotServed) {
		t.Errorf("err = %v, want it to read as not served", err)
	}
}

func TestHealthKeepsARefusalThatIsNotAMissingPath(t *testing.T) {
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/monitor/health", http.StatusUnauthorized, `{"detail":"no token"}`)
	client := stub.client()

	_, err := client.Health(t.Context())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want the 401 reported", err)
	}
	if stub.countRequests(http.MethodGet, "/health") != 0 {
		t.Error("a 401 is the answer, not a reason to try the other path")
	}
}
