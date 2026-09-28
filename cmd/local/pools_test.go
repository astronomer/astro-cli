package local

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// startedAirflow is a runtime whose start succeeds and reports the stub as
// the Airflow it brought up.
type startedAirflow struct {
	fakeAirflow
	status localrt.Status
}

func (a startedAirflow) Status() (localrt.Status, error) { return a.status, nil }

type poolsRuntime struct {
	fakeRuntime
	af startedAirflow
}

func (r poolsRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return r.af, nil
}

const poolsManifest = `
[tool.astro.pools]
etl = {slots = 4, description = 'ETL loads'}
ml = {slots = 1, include_deferred = true}
`

func startWithPools(t *testing.T, stub *airflowStub) (stdout string, err error) {
	t.Helper()
	dir := instanceProject(t, poolsManifest)
	d, out, _ := queryDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Runtime = poolsRuntime{af: startedAirflow{status: localrt.Status{
		ProjectPath:  dir,
		State:        localrt.StateRunning,
		Port:         stubPort(t, stub),
		AirflowMajor: "3",
	}}}
	err = execute(t, d, "local", "start")
	return out.String(), err
}

func TestStartCreatesTheManifestPools(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/pools", `{}`)

	out, err := startWithPools(t, stub)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "warning") {
		t.Errorf("a start that created every pool warned:\n%s", out)
	}
	var bodies []map[string]any
	for _, req := range stub.requests() {
		if req.Method != http.MethodPost {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
	}
	want := []map[string]any{
		{"name": "etl", "slots": float64(4), "description": "ETL loads"},
		{"name": "ml", "slots": float64(1), "include_deferred": true},
	}
	if !reflect.DeepEqual(bodies, want) {
		t.Errorf("created %v, want %v", bodies, want)
	}
}

// One pool failing is one warning naming it; the start still succeeds and the
// next pool still goes in.
func TestStartWarnsForAPoolItCouldNotApply(t *testing.T) {
	stub := newAirflowStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/pools/etl", http.StatusForbidden, `{"detail":"Forbidden"}`)
	stub.route(http.MethodPost, "/api/v2/pools", `{}`)

	out, err := startWithPools(t, stub)
	if err != nil {
		t.Fatalf("a pool that failed failed the start: %v", err)
	}
	if n := strings.Count(out, "warning:"); n != 1 {
		t.Errorf("want one warning, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "warning: pool etl was not created or updated") {
		t.Errorf("the warning does not name the pool:\n%s", out)
	}
	if !stub.sawRequest(http.MethodPost, "/api/v2/pools") {
		t.Error("the pool after the failed one was not created")
	}
}
