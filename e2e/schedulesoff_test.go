//go:build e2e && !windows

package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// schedulesOffWait is how long a start is given to create a run on its own. The
// scheduler loops about once a second and the DAG is already parsed, so a run it
// meant to create would be there well before this.
const schedulesOffWait = 15 * time.Second

const catchupDAG = `from datetime import datetime, timezone

from airflow.sdk import dag, task


@dag(schedule="@daily", start_date=datetime(2026, 1, 1, tzinfo=timezone.utc), catchup=True)
def catchup_dag():
    @task
    def hello():
        return 1

    hello()


catchup_dag()
`

// A local start runs no schedules: a daily DAG with catchup and a past start
// date gets no run of its own, and a run someone triggers still runs.
func TestLocalStartRunsOnlyTriggeredRuns(t *testing.T) {
	tier(t, 2)

	p := airflowProject(t)
	write(t, filepath.Join(p.Dir, "dags", "catchup_dag.py"), catchupDAG)
	t.Cleanup(func() { p.runSlow("local", "stop") })
	p.runSlow("local", "start").requireSuccess().requireStderr("Schedules are off locally")

	waitFor(t, "catchup_dag to be parsed", func() bool {
		var d struct {
			IsPaused bool `json:"is_paused"`
		}
		r := p.run("local", "af", "dags", "get", "catchup_dag", "--output", "json")
		return r.ExitCode == 0 && json.Unmarshal([]byte(r.Stdout), &d) == nil && !d.IsPaused
	})
	time.Sleep(schedulesOffWait)
	if runs := p.runs(); len(runs) != 0 {
		t.Fatalf("the scheduler created %d runs on its own, the first %+v", len(runs), runs[0])
	}

	p.run("local", "af", "runs", "trigger", "catchup_dag", "--output", "json").requireSuccess()
	waitFor(t, "the triggered run to start", func() bool {
		for _, r := range p.runs() {
			if r.State == "running" || r.State == "success" {
				return true
			}
		}
		return false
	})
	if runs := p.runs(); len(runs) != 1 || runs[0].RunType != "manual" {
		t.Errorf("runs = %+v, want the one manual run", runs)
	}
}

type runRow struct {
	DAGID   string `json:"dag_id"`
	State   string `json:"state"`
	RunType string `json:"run_type"`
}

// runs lists every run on the project's Airflow, one JSON row per line.
func (p *project) runs() []runRow {
	p.t.Helper()
	out := p.run("local", "af", "runs", "list", "--output", "json").requireSuccess().Stdout
	var rows []runRow
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var r runRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			p.t.Fatalf("runs list row is not JSON: %v\n%s", err, line)
		}
		rows = append(rows, r)
	}
	return rows
}
