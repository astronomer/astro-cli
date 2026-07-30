package airflowapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestStatusErrorReadsAsTheRightCondition(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		collection bool
		is         []error
		isNot      []error
	}{
		{name: "not found", status: http.StatusNotFound, is: []error{ErrNotFound}, isNot: []error{ErrNotServed}},
		{
			name: "not found on a collection", status: http.StatusNotFound, collection: true,
			is: []error{ErrNotFound, ErrNotServed},
		},
		{name: "unauthorized", status: http.StatusUnauthorized, is: []error{ErrUnauthorized}, isNot: []error{ErrForbidden}},
		{name: "forbidden", status: http.StatusForbidden, is: []error{ErrForbidden}, isNot: []error{ErrUnauthorized}},
		{name: "method not allowed", status: http.StatusMethodNotAllowed, is: []error{ErrNotServed}},
		{
			name: "server error", status: http.StatusInternalServerError,
			isNot: []error{ErrNotFound, ErrUnauthorized, ErrForbidden, ErrNotServed},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error = &StatusError{
				Method:     http.MethodGet,
				Path:       "/dags",
				StatusCode: tt.status,
				Collection: tt.collection,
			}
			for _, want := range tt.is {
				if !errors.Is(err, want) {
					t.Errorf("errors.Is(%v, %v) = false, want true", err, want)
				}
			}
			for _, unwanted := range tt.isNot {
				if errors.Is(err, unwanted) {
					t.Errorf("errors.Is(%v, %v) = true, want false", err, unwanted)
				}
			}
		})
	}
}

func TestStatusErrorMessageCarriesAirflowsExplanation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"a sentence", `{"detail":"DAG not found"}`, "DAG not found"},
		{"a validation list", `{"detail":[{"loc":["body","dag_id"]}]}`, `"loc"`},
		{"some other shape", `<html>gateway</html>`, "gateway"},
		{"nothing at all", "", "404 Not Found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &StatusError{
				Method:     http.MethodGet,
				Path:       "/dags/etl",
				StatusCode: http.StatusNotFound,
				Body:       []byte(tt.body),
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Error() = %q, want it to contain %q", err.Error(), tt.want)
			}
			if !strings.Contains(err.Error(), "GET /dags/etl") {
				t.Errorf("Error() = %q, want it to name the call", err.Error())
			}
		})
	}
}

func TestStatusErrorMessageBoundsALongBody(t *testing.T) {
	err := &StatusError{
		Method:     http.MethodGet,
		Path:       "/dags",
		StatusCode: http.StatusBadGateway,
		Body:       []byte(strings.Repeat("x", maxBodyInError*2)),
	}
	if !strings.HasSuffix(err.Error(), "...") {
		t.Errorf("Error() = %q, want a bounded body", err.Error())
	}
}

func TestStatusErrorKeepsTheWholeBody(t *testing.T) {
	stub := newAF3Stub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/etl", http.StatusInternalServerError, `{"detail":"boom"}`)
	client := stub.client()

	_, err := client.GetDAG(t.Context(), "etl")
	var status *StatusError
	if !errors.As(err, &status) {
		t.Fatalf("err = %v, want a *StatusError", err)
	}
	if string(status.Body) != `{"detail":"boom"}` {
		t.Errorf("body = %q, want it kept whole", status.Body)
	}
	if status.Path != "/dags/etl" {
		t.Errorf("path = %q, want the api-relative path", status.Path)
	}
}

func TestDetectErrorNamesEachProbe(t *testing.T) {
	err := &DetectError{Probes: []ProbeResult{
		{Generation: Airflow3, Err: errors.New("connection refused")},
		{Generation: Airflow2, Err: errors.New("connection refused")},
	}}
	message := err.Error()
	if !strings.Contains(message, "/api/v2/version") || !strings.Contains(message, "/api/v1/version") {
		t.Errorf("Error() = %q, want both probes named", message)
	}
}
