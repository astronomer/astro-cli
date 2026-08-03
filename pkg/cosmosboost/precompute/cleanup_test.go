package precompute

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stampProject creates a minimal dbt project under dir and runs the real
// pre-deploy over it, so cleanup tests exercise sidecars exactly as the
// producer writes them.
func stampProject(t *testing.T, dir string) {
	t.Helper()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\n",
		"models/a.sql":    "select 1",
	})
	summary, err := Run([]string{dir}, "test")
	if err != nil || summary.CountFailed() > 0 {
		t.Fatalf("stamping fixture project failed: err=%v failed=%d", err, summary.CountFailed())
	}
}

func TestCleanupRemovesSidecarAndPrunesDir(t *testing.T) {
	dir := t.TempDir()
	stampProject(t, dir)

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := len(summary.Results); got != 1 {
		t.Fatalf("results = %d, want 1", got)
	}
	if summary.CountFailed() != 0 || summary.CountKept() != 0 {
		t.Fatalf("failed=%d kept=%d, want 0/0", summary.CountFailed(), summary.CountKept())
	}
	if _, err := os.Stat(filepath.Join(dir, sidecarDir, sidecarName)); !os.IsNotExist(err) {
		t.Fatalf("sidecar still present: %v", err)
	}
	// Removal emptied .astro, so the directory itself is pruned too.
	if _, err := os.Stat(filepath.Join(dir, sidecarDir)); !os.IsNotExist(err) {
		t.Fatalf(".astro dir not pruned: %v", err)
	}
}

func TestCleanupLeavesNonEmptyAstroDir(t *testing.T) {
	dir := t.TempDir()
	stampProject(t, dir)
	writeFiles(t, dir, map[string]string{".astro/config.yaml": "project:\n  name: shop\n"})

	if _, err := Cleanup([]string{dir}); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sidecarDir, sidecarName)); !os.IsNotExist(err) {
		t.Fatalf("sidecar still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sidecarDir, "config.yaml")); err != nil {
		t.Fatalf("unrelated .astro content was touched: %v", err)
	}
}

// TestCleanupKeepsForeignSidecars pins the safety property: a
// .astro/dbt_metadata.json this tool did not write — another producer's, or
// one that isn't JSON at all — is never deleted.
func TestCleanupKeepsForeignSidecars(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"other/.astro/dbt_metadata.json":   `{"generated_by": {"application": "someone-else"}}`,
		"invalid/.astro/dbt_metadata.json": "not json",
	})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := summary.CountKept(); got != 2 {
		t.Fatalf("kept = %d, want 2", got)
	}
	for _, rel := range []string{"other", "invalid"} {
		if _, err := os.Stat(filepath.Join(dir, rel, sidecarDir, sidecarName)); err != nil {
			t.Fatalf("foreign sidecar under %s was removed: %v", rel, err)
		}
	}
}

// TestCleanupRemovesLegacySidecars: sidecars written by the retired
// standalone astro-cosmos-boost helper are still ours to remove.
func TestCleanupRemovesLegacySidecars(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"proj/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro-cosmos-boost"}}`,
	})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if summary.CountKept() != 0 || summary.CountFailed() != 0 || len(summary.Results) != 1 {
		t.Fatalf("results=%d kept=%d failed=%d, want 1/0/0", len(summary.Results), summary.CountKept(), summary.CountFailed())
	}
	if _, err := os.Stat(filepath.Join(dir, "proj", sidecarDir, sidecarName)); !os.IsNotExist(err) {
		t.Fatalf("legacy sidecar still present: %v", err)
	}
}

// TestCleanupIgnoresLookalikes: only dbt_metadata.json directly inside a
// .astro directory is a sidecar; same-named files elsewhere are untouched.
func TestCleanupIgnoresLookalikes(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"data/dbt_metadata.json": `{"generated_by": {"application": "astro-cosmos-boost"}}`,
	})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := len(summary.Results); got != 0 {
		t.Fatalf("results = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", sidecarName)); err != nil {
		t.Fatalf("lookalike outside .astro was removed: %v", err)
	}
}

func TestCleanupOverlappingRoots(t *testing.T) {
	dir := t.TempDir()
	stampProject(t, dir)

	summary, err := Cleanup([]string{dir, dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := len(summary.Results); got != 1 {
		t.Fatalf("overlapping roots reported %d results, want 1", got)
	}
}

// TestCleanupRootsSpelledDifferently covers one tree named two ways: an
// absolute path and a relative one. A sidecar we remove cannot be double-counted
// (it is gone by the second walk), but a foreign one is deliberately left in
// place, so keying the seen-set on the raw walked path finds it again under the
// other spelling and reports it kept twice.
func TestCleanupRootsSpelledDifferently(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "shop")
	writeFiles(t, project, map[string]string{
		".astro/dbt_metadata.json": `{"generated_by": {"application": "someone-else"}}`,
	})

	t.Chdir(parent) // so "shop" and the absolute path name one tree

	summary, err := Cleanup([]string{project, "shop"})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := len(summary.Results); got != 1 {
		t.Fatalf("roots spelled differently reported %d results, want 1", got)
	}
	if got := summary.CountKept(); got != 1 {
		t.Fatalf("kept = %d, want 1: one file must be reported once per run", got)
	}
}

func TestCleanupNonexistentRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := Cleanup([]string{missing}); err == nil {
		t.Fatal("Cleanup on a nonexistent root: error = nil, want non-nil")
	}
}

func TestCleanupWriteReport(t *testing.T) {
	dir := t.TempDir()
	stampProject(t, dir)
	writeFiles(t, dir, map[string]string{"other/.astro/dbt_metadata.json": "not json"})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	var out bytes.Buffer
	summary.WriteReport(&out)
	got := out.String()
	if !strings.Contains(got, "1 removed, 1 kept, 0 failed") {
		t.Fatalf("report summary line missing: %q", got)
	}
	if !strings.Contains(got, "left in place") {
		t.Fatalf("kept sidecar not explained in report: %q", got)
	}
}

// TestCleanupSkipsGitInternals pins that cleanup never traverses .git: even a
// sidecar this tool's own (buggy or interrupted) run left there is not touched,
// because mutating VCS internals is worse than leaving an undeployed file.
func TestCleanupSkipsGitInternals(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".git/trap/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`,
	})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := len(summary.Results); got != 0 {
		t.Fatalf("results = %d, want 0 (nothing under .git may be visited)", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "trap", ".astro", "dbt_metadata.json")); err != nil {
		t.Fatalf("file under .git was touched: %v", err)
	}
}
