package local

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// timeoutRuntime records the wait a start asked the runtime for.
type timeoutRuntime struct {
	fakeRuntime
	got *time.Duration
}

func (r timeoutRuntime) SetHealthTimeout(d time.Duration) { *r.got = d }

func TestStartTimeoutFlagReachesTheRuntime(t *testing.T) {
	var got time.Duration
	d, _ := testDeps(t)
	d.Runtime = timeoutRuntime{got: &got}
	// The start itself fails (no project here); the wait is applied first.
	_ = execute(t, d, "start", "--timeout", "90s")
	if got != 90*time.Second {
		t.Errorf("runtime was given %v, want 90s", got)
	}
}

func TestStartWithoutTimeoutLeavesTheRuntimeAlone(t *testing.T) {
	var got time.Duration
	d, _ := testDeps(t)
	d.Runtime = timeoutRuntime{got: &got}
	_ = execute(t, d, "start")
	if got != 0 {
		t.Errorf("runtime was given %v with no --timeout, want nothing", got)
	}
}

func TestStartTimeoutMustBeAPositiveDuration(t *testing.T) {
	for _, v := range []string{"0s", "-1m", "600", "soon"} {
		var got time.Duration
		d, _ := testDeps(t)
		d.Runtime = timeoutRuntime{got: &got}
		err := execute(t, d, "start", "--timeout", v)
		if err == nil || !cliout.IsUsage(err) {
			t.Errorf("--timeout %s: err = %v, want a usage error", v, err)
		}
		if got != 0 {
			t.Errorf("--timeout %s reached the runtime as %v", v, got)
		}
		if err != nil && strings.Contains(err.Error(), "no project") {
			t.Errorf("--timeout %s was not refused before the start ran: %v", v, err)
		}
	}
}

// A --limit that is not a whole number of one or more is refused as the flag
// parses, rather than quietly becoming the default page.
func TestListLimitBelowOneIsAUsageError(t *testing.T) {
	for _, v := range []string{"0", "-5", "lots"} {
		stub := newAirflowStub(t)
		stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[],"total_entries":0}`)
		_, _, err := runQuery(t, stub, "dags", "list", "--limit", v)
		if err == nil || !cliout.IsUsage(err) {
			t.Errorf("--limit %s: err = %v, want a usage error", v, err)
		}
		for _, req := range stub.requests() {
			if req.Path == "/api/v2/dags" {
				t.Errorf("--limit %s still reached the API: %s", v, req.Query)
			}
		}
	}
}
