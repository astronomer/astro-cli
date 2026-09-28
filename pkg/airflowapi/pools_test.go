package airflowapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func decodeBody(t *testing.T, req recordedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		t.Fatalf("decode %s %s body %q: %v", req.Method, req.Path, req.Body, err)
	}
	return body
}

func TestUpsertPoolCreatesAMissingPool(t *testing.T) {
	yes := true
	for _, tc := range []struct {
		name  string
		stub  func(*testing.T) *airflowStub
		pools string
	}{
		{"airflow 3", newAF3Stub, "/api/v2/pools"},
		{"airflow 2", newAF2Stub, "/api/v1/pools"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodPost, tc.pools, `{"name":"etl","slots":4}`)

			if err := stub.client().UpsertPool(t.Context(), PoolSpec{Name: "etl", Slots: 4, Description: "ETL loads", IncludeDeferred: &yes}); err != nil {
				t.Fatal(err)
			}
			req := stub.lastRequest()
			if req.Method != http.MethodPost || req.Path != tc.pools {
				t.Fatalf("last request = %s %s, want POST %s", req.Method, req.Path, tc.pools)
			}
			want := map[string]any{"name": "etl", "slots": float64(4), "description": "ETL loads", "include_deferred": true}
			if body := decodeBody(t, req); !reflect.DeepEqual(body, want) {
				t.Errorf("body = %v, want %v", body, want)
			}
		})
	}
}

func TestUpsertPoolLeavesAMatchingPoolAlone(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/pools/etl", `{"name":"etl","slots":4,"description":"set by hand","include_deferred":true}`)

	if err := stub.client().UpsertPool(t.Context(), PoolSpec{Name: "etl", Slots: 4}); err != nil {
		t.Fatal(err)
	}
	for _, req := range stub.requests() {
		if req.Method != http.MethodGet {
			t.Errorf("a pool that already matches was written: %s %s", req.Method, req.Path)
		}
	}
}

// Airflow 3 validates a patch as a whole pool, so the include_deferred the
// spec leaves unset rides along at its current value rather than being
// dropped or reset.
func TestUpsertPoolUpdatesOnAirflow3WithTheCurrentIncludeDeferred(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/pools/etl", `{"name":"etl","slots":4,"include_deferred":true}`)
	stub.route(http.MethodPatch, "/api/v2/pools/etl", `{"name":"etl","slots":5}`)

	if err := stub.client().UpsertPool(t.Context(), PoolSpec{Name: "etl", Slots: 5}); err != nil {
		t.Fatal(err)
	}
	req := stub.lastRequest()
	if req.Method != http.MethodPatch {
		t.Fatalf("last request = %s %s, want a PATCH", req.Method, req.Path)
	}
	if len(req.Query) != 0 {
		t.Errorf("query = %v, want no update mask", req.Query)
	}
	want := map[string]any{"name": "etl", "slots": float64(5), "include_deferred": true}
	if body := decodeBody(t, req); !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
}

func TestUpsertPoolUpdatesDefaultPoolOnAirflow3ThroughAMask(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/pools/default_pool", `{"name":"default_pool","slots":128,"include_deferred":false}`)
	stub.route(http.MethodPatch, "/api/v2/pools/default_pool", `{"name":"default_pool","slots":16}`)
	yes := true

	if err := stub.client().UpsertPool(t.Context(), PoolSpec{Name: "default_pool", Slots: 16, IncludeDeferred: &yes}); err != nil {
		t.Fatal(err)
	}
	req := stub.lastRequest()
	if mask := req.Query["update_mask"]; !reflect.DeepEqual(mask, []string{"slots", "include_deferred"}) {
		t.Errorf("update_mask = %v, want one value per field", mask)
	}
}

// Airflow 2 fills an absent include_deferred with false on a maskless patch,
// so every patch there names its fields, in the comma-separated form its API
// reads.
func TestUpsertPoolUpdatesOnAirflow2ThroughAMask(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/pools/etl", `{"name":"etl","slots":4,"description":"old","include_deferred":true}`)
	stub.route(http.MethodPatch, "/api/v1/pools/etl", `{"name":"etl","slots":5}`)

	if err := stub.client().UpsertPool(t.Context(), PoolSpec{Name: "etl", Slots: 5, Description: "new"}); err != nil {
		t.Fatal(err)
	}
	req := stub.lastRequest()
	if req.Method != http.MethodPatch {
		t.Fatalf("last request = %s %s, want a PATCH", req.Method, req.Path)
	}
	if mask := req.Query["update_mask"]; !reflect.DeepEqual(mask, []string{"slots,description"}) {
		t.Errorf("update_mask = %v, want slots,description", mask)
	}
	want := map[string]any{"name": "etl", "slots": float64(5), "description": "new"}
	if body := decodeBody(t, req); !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
}

func TestUpsertPoolReportsAFailedRead(t *testing.T) {
	stub := newAF3Stub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/pools/etl", http.StatusForbidden, `{"detail":"Forbidden"}`)

	if err := stub.client().UpsertPool(t.Context(), PoolSpec{Name: "etl", Slots: 4}); err == nil {
		t.Fatal("a pool that could not be read was reported as upserted")
	}
	if n := stub.countRequests(http.MethodPost, "/api/v2/pools"); n != 0 {
		t.Errorf("created the pool %d times after a failed read", n)
	}
}
