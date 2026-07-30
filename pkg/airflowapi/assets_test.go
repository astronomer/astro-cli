package airflowapi

import (
	"errors"
	"net/http"
	"testing"
)

func TestListAssetsReadsAirflow2Datasets(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/datasets",
		`{"datasets":[{"id":1,"uri":"s3://orders","consuming_dags":[{"dag_id":"etl"}]}],"total_entries":1}`)
	client := stub.client()

	list, err := client.ListAssets(t.Context(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Assets) != 1 || list.Assets[0].URI != "s3://orders" {
		t.Fatalf("list = %+v, want the dataset read as an asset", list)
	}
	if len(list.Assets[0].ScheduledDAGs) != 1 || list.Assets[0].ScheduledDAGs[0].DAGID != "etl" {
		t.Errorf("scheduled dags = %+v, want consuming_dags mapped over", list.Assets[0].ScheduledDAGs)
	}
	if list.TotalEntries != 1 {
		t.Errorf("total = %d, want 1", list.TotalEntries)
	}
}

func TestListAssetsReadsAirflow3Assets(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/assets",
		`{"assets":[{"id":1,"name":"orders","uri":"s3://orders","scheduled_dags":[{"dag_id":"etl"}]}],"total_entries":1}`)
	client := stub.client()

	list, err := client.ListAssets(t.Context(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Assets) != 1 || list.Assets[0].Name != "orders" {
		t.Fatalf("list = %+v, want the asset", list)
	}
	if len(list.Assets[0].ScheduledDAGs) != 1 {
		t.Errorf("scheduled dags = %+v, want the one dag", list.Assets[0].ScheduledDAGs)
	}
}

func TestListAssetEventsMapsBothSpellings(t *testing.T) {
	tests := []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
		body string
	}{
		{
			name: "airflow 2 dataset events", stub: newAF2Stub, path: "/api/v1/datasets/events",
			body: `{"dataset_events":[{"id":7,"dataset_id":3,"dataset_uri":"s3://orders","source_dag_id":"etl"}],"total_entries":1}`,
		},
		{
			name: "airflow 3 asset events", stub: newAF3Stub, path: "/api/v2/assets/events",
			body: `{"asset_events":[{"id":7,"asset_id":3,"asset_uri":"s3://orders","source_dag_id":"etl"}],"total_entries":1}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := tt.stub(t)
			stub.route(http.MethodGet, tt.path, tt.body)
			client := stub.client()

			list, err := client.ListAssetEvents(t.Context(), ListAssetEventsOptions{SourceDAGID: "etl"})
			if err != nil {
				t.Fatal(err)
			}
			if len(list.AssetEvents) != 1 {
				t.Fatalf("list = %+v, want the one event", list)
			}
			event := list.AssetEvents[0]
			if event.URI != "s3://orders" || event.AssetID != 3 || event.SourceDAGID != "etl" {
				t.Errorf("event = %+v, want the fields mapped to one shape", event)
			}
			if got := stub.lastRequest().Query.Get("source_dag_id"); got != "etl" {
				t.Errorf("source_dag_id = %q, want the filter sent", got)
			}
		})
	}
}

func TestUpstreamAssetEventsUsesEachGenerationsPath(t *testing.T) {
	tests := []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
	}{
		{"airflow 2", newAF2Stub, "/api/v1/dags/etl/dagRuns/r1/upstreamDatasetEvents"},
		{"airflow 3", newAF3Stub, "/api/v2/dags/etl/dagRuns/r1/upstreamAssetEvents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := tt.stub(t)
			stub.route(http.MethodGet, tt.path, `{"asset_events":[],"total_entries":0}`)
			client := stub.client()

			if _, err := client.UpstreamAssetEvents(t.Context(), "etl", "r1"); err != nil {
				t.Fatal(err)
			}
			if got := stub.lastRequest().Path; got != tt.path {
				t.Errorf("path = %q, want %q", got, tt.path)
			}
		})
	}
}

func TestUpstreamAssetEventsReadsAMissingRunAsMissing(t *testing.T) {
	// The path names a dag run, so a 404 is that run being absent rather
	// than an Airflow without the endpoint.
	stub := newAF3Stub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/etl/dagRuns/norun/upstreamAssetEvents",
		http.StatusNotFound, `{"detail":"The DagRun was not found"}`)
	client := stub.client()

	_, err := client.UpstreamAssetEvents(t.Context(), "etl", "norun")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want it to read as not found", err)
	}
	if errors.Is(err, ErrNotServed) {
		t.Errorf("err = %v, want a missing run not to read as a missing endpoint", err)
	}
}

func TestAssetsAreNotServedByAnAirflowWithoutThem(t *testing.T) {
	// Datasets arrived mid-way through Airflow 2's life. Nothing checks a
	// minor version for that: the endpoint is asked and its refusal is read.
	stub := newAF2Stub(t)
	client := stub.client()

	_, err := client.ListAssets(t.Context(), ListOptions{})
	if !errors.Is(err, ErrNotServed) {
		t.Errorf("err = %v, want it to read as not served", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want it to keep reading as not found too", err)
	}
}
