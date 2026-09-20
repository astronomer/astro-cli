//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// seedVenv fabricates the interpreter a hot install needs to find.
func seedVenv(t *testing.T, project string) {
	t.Helper()
	bin := filepath.Join(project, ".venv", "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "python"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func engineWithUV(t *testing.T, f fakeUV) *Engine {
	t.Helper()
	e := New(t.TempDir(), nil, UVOptions{})
	e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) { return f, nil }
	return e
}

// Additive, not a re-sync. EnsureSynced resolves the whole declared set and can
// remove what the manifest no longer names — out from under a scheduler that is
// mid-parse. A hot install is asked for when something was ADDED.
func TestHotInstallAddsRatherThanReconciling(t *testing.T) {
	project := t.TempDir()
	seedVenv(t, project)
	var installed []string
	e := engineWithUV(t, fakeUV{pipInstalled: &installed})

	if err := e.HotInstall(t.Context(), project, []string{"pandas==2.2.0", "requests"}, rt.Callbacks{}); err != nil {
		t.Fatalf("HotInstall() = %v", err)
	}

	want := []string{"pandas==2.2.0", "requests"}
	if len(installed) != len(want) {
		t.Fatalf("installed %v, want %v", installed, want)
	}
	for i := range want {
		if installed[i] != want[i] {
			t.Errorf("installed[%d] = %q, want %q", i, installed[i], want[i])
		}
	}
}

// Airflow re-parses a Dag when its mtime moves, so touching them is what makes
// a new package visible without a restart. It is the reason this lives in the
// engine instead of being a bare `uv pip install` at the call site.
func TestHotInstallNudgesTheSchedulerToRereadDags(t *testing.T) {
	project := t.TempDir()
	seedVenv(t, project)
	dags := filepath.Join(project, "dags")
	if err := os.MkdirAll(dags, 0o750); err != nil {
		t.Fatal(err)
	}
	dag := filepath.Join(dags, "etl.py")
	if err := os.WriteFile(dag, []byte("# dag\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dag, stale, stale); err != nil {
		t.Fatal(err)
	}

	if err := e2HotInstall(t, project); err != nil {
		t.Fatalf("HotInstall() = %v", err)
	}

	info, err := os.Stat(dag)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(stale) {
		t.Error("the Dag's mtime did not move, so the scheduler will not re-parse it and the new package stays invisible until a restart")
	}
}

// dags/<subdir>/*.py is the ordinary layout for anything past a scaffold, and a
// top-level-only walk nudged none of it while still reporting success — which
// is exactly the outcome the nudge exists to prevent.
func TestHotInstallRereadsDagsInSubdirectories(t *testing.T) {
	project := t.TempDir()
	seedVenv(t, project)
	nested := filepath.Join(project, "dags", "etl", "daily")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	dag := filepath.Join(nested, "load.py")
	if err := os.WriteFile(dag, []byte("# dag\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dag, stale, stale); err != nil {
		t.Fatal(err)
	}

	if err := e2HotInstall(t, project); err != nil {
		t.Fatalf("HotInstall() = %v", err)
	}

	info, err := os.Stat(dag)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(stale) {
		t.Error("a Dag two directories down was not re-parsed, so the install is invisible to it until a restart")
	}
}

// The completion marker is the claim that this venv finished installing. While
// one is in progress that claim is false, and an interrupted install has to
// leave it false so the next start repairs the venv instead of trusting it.
func TestHotInstallLeavesTheMarkerDownWhenItFails(t *testing.T) {
	project := t.TempDir()
	seedVenv(t, project)
	marker := filepath.Join(project, ".venv", uv.MarkerName)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	e := engineWithUV(t, fakeUV{pipErr: errors.New("resolution failed")})
	if err := e.HotInstall(t.Context(), project, []string{"pandas"}, rt.Callbacks{}); err == nil {
		t.Fatal("HotInstall() = nil, want the install failure")
	}

	if _, err := os.Stat(marker); err == nil {
		t.Error("the marker survived a failed install, so the next start will trust a venv that was written into halfway")
	}
}

// And it goes back when the install works, or every later start pays for a
// resync it does not need.
func TestHotInstallPutsTheMarkerBackWhenItSucceeds(t *testing.T) {
	project := t.TempDir()
	seedVenv(t, project)
	marker := filepath.Join(project, ".venv", uv.MarkerName)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := e2HotInstall(t, project); err != nil {
		t.Fatalf("HotInstall() = %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the marker was not restored after a successful install: %v", err)
	}
}

// A project with no dags directory is ordinary — a brand new one, or one whose
// Dags live where the manifest points. Failing the install for it would undo
// work that already succeeded.
func TestHotInstallSucceedsWithNoDagsDirectory(t *testing.T) {
	project := t.TempDir()
	seedVenv(t, project)

	if err := e2HotInstall(t, project); err != nil {
		t.Errorf("HotInstall() = %v, want nil for a project with no dags/", err)
	}
}

// Nothing declared is not a failure: a caller watching a manifest cannot know
// the project has no dependencies before it asks.
func TestHotInstallWithNothingDeclaredDoesNothing(t *testing.T) {
	project := t.TempDir()
	var installed []string
	e := engineWithUV(t, fakeUV{pipInstalled: &installed})

	// No venv either, to prove it returns before looking for one.
	if err := e.HotInstall(t.Context(), project, nil, rt.Callbacks{}); err != nil {
		t.Errorf("HotInstall() = %v, want nil", err)
	}
	if len(installed) != 0 {
		t.Errorf("installed %v for an empty dependency set", installed)
	}
}

// uv would report a missing interpreter, which does not tell the user the
// answer is "start it first".
func TestHotInstallRefusesAProjectWithNoEnvironment(t *testing.T) {
	e := engineWithUV(t, fakeUV{})

	err := e.HotInstall(t.Context(), t.TempDir(), []string{"pandas"}, rt.Callbacks{})
	if err == nil {
		t.Fatal("HotInstall() = nil for a project with no venv")
	}
	if !errors.Is(err, ErrNoEnvironment) {
		t.Errorf("error = %v, want ErrNoEnvironment so a consumer can branch without reading the message", err)
	}
	if !strings.Contains(err.Error(), "start it first") {
		t.Errorf("error = %q, want it to say what to do about it", err)
	}
}

func e2HotInstall(t *testing.T, project string) error {
	t.Helper()
	var installed []string
	return engineWithUV(t, fakeUV{pipInstalled: &installed}).
		HotInstall(t.Context(), project, []string{"pandas"}, rt.Callbacks{})
}
