package airflowapi

import (
	"errors"
	"net/http"
	"testing"
)

func TestConfigReadsTheSections(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/config",
		`{"sections":[{"name":"core","options":[{"key":"dags_folder","value":"/usr/local/airflow/dags"}]}]}`)
	client := stub.client()

	config, err := client.Config(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Sections) != 1 || config.Sections[0].Options[0].Key != "dags_folder" {
		t.Fatalf("config = %+v, want the section read", config)
	}
}

func TestConfigReportsAHiddenConfigAsForbidden(t *testing.T) {
	// Airflow refuses this unless expose_config is on. It reaches the caller
	// as a typed error rather than an empty answer.
	stub := newAF2Stub(t)
	stub.routeStatus(http.MethodGet, "/api/v1/config", http.StatusForbidden,
		`{"detail":"Your Airflow administrator chose not to expose the configuration"}`)
	client := stub.client()

	_, err := client.Config(t.Context())
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want it to read as forbidden", err)
	}
	var status *StatusError
	if !errors.As(err, &status) || status.Detail() == "" {
		t.Errorf("err = %v, want airflow's own explanation kept", err)
	}
}

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
