package local

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// stubRuntime serves canned list and prune results; the other Runtime calls
// come from fakeRuntime and fail.
type stubRuntime struct {
	fakeRuntime
	list   []localrt.Status
	pruned []localrt.Status
	// pruneErr rides along with pruned, because PruneStale is best-effort:
	// what it removed and what it could not are both true at once.
	pruneErr error
}

func (s stubRuntime) List() ([]localrt.Status, error) { return s.list, nil }
func (s stubRuntime) PruneStale() ([]localrt.Status, error) {
	return s.pruned, s.pruneErr
}

// ReadStatus answers from the same records List does, so a case that says an
// Airflow is running says it once and every reader agrees. Paths are compared
// canonically because the real runtime keys its records by the symlink-resolved
// path, and a test that pretends otherwise would pass on a lie.
func (s stubRuntime) ReadStatus(projectPath string) (localrt.Status, error) {
	for i := range s.list {
		if canonical(s.list[i].ProjectPath) == canonical(projectPath) {
			return s.list[i], nil
		}
	}
	return localrt.Status{ProjectPath: projectPath, State: localrt.StateStopped}, nil
}

// canonical resolves symlinks in a path so two spellings of one directory
// compare equal. A path that cannot be resolved keeps its original spelling.
func canonical(path string) string {
	resolved, err := localrt.CanonicalPath(path)
	if err != nil {
		return path
	}
	return resolved
}

func TestBuildListRows(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	statuses := []localrt.Status{
		{ProjectPath: "/a", Mode: localrt.ModeDocker, State: localrt.StateRunning, Port: 8080, Hostname: "a.localhost", StartedAt: now.Add(-90 * time.Minute)},
		{ProjectPath: "/b", Mode: localrt.ModeStandalone, State: localrt.StateStopped, Port: 8081},
	}

	running := buildListRows(statuses, false, now)
	if len(running) != 1 {
		t.Fatalf("without --all, want only the running row, got %d", len(running))
	}
	got := running[0]
	if got.State != "running" || got.Uptime != "1h30m" || got.URL != "http://localhost:8080" {
		t.Errorf("running row wrong: %+v", got)
	}

	all := buildListRows(statuses, true, now)
	if len(all) != 2 {
		t.Fatalf("with --all, want both rows, got %d", len(all))
	}
	if all[1].State != "stopped (stale)" {
		t.Errorf("stale row must read as stopped (stale): %+v", all[1])
	}
	if all[1].Uptime != "" || all[1].URL != "" {
		t.Errorf("stale row must carry no uptime or url: %+v", all[1])
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h30m"},
		{-time.Second, "0s"},
	}
	for _, tc := range cases {
		if got := formatUptime(tc.d); got != tc.want {
			t.Errorf("formatUptime(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestListTextTableShowsColumns(t *testing.T) {
	d, out := testDeps(t)
	d.Runtime = stubRuntime{list: []localrt.Status{
		{ProjectPath: "/proj/a", Mode: localrt.ModeDocker, State: localrt.StateRunning, Port: 8080, Hostname: "a.localhost", StartedAt: time.Now().Add(-time.Hour)},
		{ProjectPath: "/proj/b", Mode: localrt.ModeStandalone, State: localrt.StateStopped, Port: 8081},
	}}
	if err := execute(t, d, "local", "list", "--all"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"PROJECT", "HOSTNAME", "STATE", "UPTIME", "/proj/a", "a.localhost", "running", "/proj/b", "stopped (stale)"} {
		if !strings.Contains(text, want) {
			t.Errorf("table is missing %q:\n%s", want, text)
		}
	}
}

func TestListDefaultHidesStale(t *testing.T) {
	d, out := testDeps(t)
	d.Runtime = stubRuntime{list: []localrt.Status{
		{ProjectPath: "/proj/b", Mode: localrt.ModeStandalone, State: localrt.StateStopped, Port: 8081},
	}}
	if err := execute(t, d, "local", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No local Airflow found") {
		t.Errorf("default list should hide the stale record:\n%s", out.String())
	}
}

func TestListJSONIsOneObjectPerLine(t *testing.T) {
	d, out := testDeps(t)
	d.Runtime = stubRuntime{list: []localrt.Status{
		{ProjectPath: "/proj/a", Mode: localrt.ModeDocker, State: localrt.StateRunning, Port: 8080, Hostname: "a.localhost", StartedAt: time.Now().Add(-time.Hour)},
		{ProjectPath: "/proj/b", Mode: localrt.ModeStandalone, State: localrt.StateStopped, Port: 8081},
	}}
	if err := execute(t, d, "local", "list", "--all", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one JSON object per row, got %d lines:\n%s", len(lines), out.String())
	}
	var first listRow
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line is not standalone JSON: %v: %q", err, lines[0])
	}
	if first.Project != "/proj/a" || first.State != "running" {
		t.Errorf("first row decoded wrong: %+v", first)
	}
}

func TestListCleanReportsRemoved(t *testing.T) {
	d, out := testDeps(t)
	d.Runtime = stubRuntime{pruned: []localrt.Status{
		{ProjectPath: "/proj/b", Mode: localrt.ModeStandalone, State: localrt.StateStopped},
	}}
	if err := execute(t, d, "local", "list", "--clean"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "Removed 1 stale record") || !strings.Contains(text, "/proj/b") {
		t.Errorf("clean output should name what it removed:\n%s", text)
	}
}

// A prune that partly worked says so, rather than reporting only the error.
//
// PruneStale is best-effort: it removes what it can and reports what it
// cannot, so the two are not alternatives. Returning the error alone told
// someone nothing had happened when records had in fact been removed — and
// they would rerun --clean against records it had already cleaned and read the
// identical error as meaning it still did nothing.
func TestListCleanReportsWhatItRemovedAlongsideWhatItCouldNot(t *testing.T) {
	d, out := testDeps(t)
	d.Runtime = stubRuntime{
		pruned: []localrt.Status{
			{ProjectPath: "/proj/b", Mode: localrt.ModeStandalone, State: localrt.StateStopped},
		},
		pruneErr: errors.New("removing record for /proj/stuck: permission denied"),
	}

	err := execute(t, d, "local", "list", "--clean")
	if err == nil {
		t.Fatal("the record it could not remove has to reach the exit status")
	}
	if !strings.Contains(err.Error(), "/proj/stuck") {
		t.Errorf("the error should name what it could not remove: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Removed 1 stale record") || !strings.Contains(text, "/proj/b") {
		t.Errorf("what it did remove should still be reported:\n%s", text)
	}
}
