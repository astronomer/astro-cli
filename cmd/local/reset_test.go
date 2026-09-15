package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// resetRuntime records the path reset was asked about and answers with a canned
// report, standing in for the engines.
//
// Everything in this file is the command's side of reset: the confirmation, the
// path it asks about, and how each field of the report renders. What reset
// actually deletes is pkg/localrt's, and pkg/localrt/reset_test.go covers it
// against the real engines rather than a stub.
type resetRuntime struct {
	fakeRuntime
	report localrt.ResetReport
	err    error
	called string
}

func (s *resetRuntime) Reset(_ context.Context, projectPath string) (localrt.ResetReport, error) {
	s.called = projectPath
	return s.report, s.err
}

// resetDeps is testDeps in a real project directory, with the runtime replaced
// by one that records what reset was asked and answers with a canned report.
func resetDeps(t *testing.T) (Deps, *resetRuntime, *bytes.Buffer) {
	t.Helper()
	d, out := testDeps(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(validManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	rt := &resetRuntime{}
	d.Runtime = rt
	return d, rt, out
}

// The command reaches the runtime and does not refuse a project with no
// running Airflow.
//
// This is the command's half only. That reset genuinely wipes a stopped
// project — the case it exists for, which used to answer `no local Airflow is
// recorded for this project` — is the runtime's, and is tested there against
// the real engines: see TestResetWipesAStoppedProjectWithNoRecord in
// pkg/localrt. A stub cannot prove a file was deleted.
func TestResetReachesTheRuntimeWithNothingRunning(t *testing.T) {
	d, rt, _ := resetDeps(t)
	rt.report = localrt.ResetReport{} // nothing running, nothing to stop

	if err := execute(t, d, "local", "reset", "-y"); err != nil {
		t.Fatalf("reset on a stopped project must succeed: %v", err)
	}
	if rt.called == "" {
		t.Error("reset did not reach the runtime")
	}
}

func TestResetReportsWhatItDid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report localrt.ResetReport
		want   []string
		absent []string
	}{
		{
			name:   "stopped project, nothing running",
			report: localrt.ResetReport{},
			want:   []string{"state: wiped"},
			absent: []string{"airflow: stopped", "no container engine"},
		},
		{
			name:   "running project is stopped first",
			report: localrt.ResetReport{Stopped: true},
			want:   []string{"airflow: stopped", "state: wiped"},
		},
		{
			name:   "docker volumes named when taken down",
			report: localrt.ResetReport{ComposeProject: "astro-demo-ab12cd"},
			want:   []string{"state: wiped", "astro-demo-ab12cd", "volumes"},
		},
		{
			// The honest half. A docker-mode project keeps its metadata
			// database in a volume, so reporting a clean wipe when the engine
			// never answered would be a lie — and the database is exactly what
			// the user was trying to be rid of.
			name:   "unreachable docker is said, not swallowed",
			report: localrt.ResetReport{DockerUnreachable: true},
			want:   []string{"state: wiped", "no container engine answered", "still there"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, rt, out := resetDeps(t)
			rt.report = tc.report

			if err := execute(t, d, "local", "reset", "-y"); err != nil {
				t.Fatalf("reset: %v", err)
			}
			got := out.String()
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q:\n%s", w, got)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("output should not contain %q:\n%s", a, got)
				}
			}
		})
	}
}

func TestResetJSONCarriesTheReport(t *testing.T) {
	d, rt, out := resetDeps(t)
	rt.report = localrt.ResetReport{Stopped: true, ComposeProject: "astro-demo-ab12cd"}

	if err := execute(t, d, "local", "reset", "-y", "--output", "json"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	var got localrt.ResetReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &got); err != nil {
		t.Fatalf("stdout is not one json object: %v: %q", err, out.String())
	}
	if !got.Stopped || got.ComposeProject != "astro-demo-ab12cd" {
		t.Errorf("report = %+v, want the runtime's own", got)
	}
}

// A failure to wipe is reported rather than claimed as success.
func TestResetSurfacesARuntimeFailure(t *testing.T) {
	d, rt, _ := resetDeps(t)
	rt.err = errors.New("permission denied removing .venv")

	err := execute(t, d, "local", "reset", "-y")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("want the runtime's error, got %v", err)
	}
}
