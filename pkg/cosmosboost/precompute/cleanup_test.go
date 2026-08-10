package precompute

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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

// TestCleanupIgnoresLookalikes: only dbt_metadata.json directly inside a
// .astro directory is a sidecar; same-named files elsewhere are untouched.
func TestCleanupIgnoresLookalikes(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"data/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`,
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

func TestCanonicalPathFallsBackOnMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if got := canonicalPath(missing); got != filepath.Clean(missing) {
		t.Fatalf("canonicalPath(%q) = %q, want the cleaned path", missing, got)
	}
}

// TestCleanupReportsUnreadableSidecar: a sidecar whose content cannot be read
// cannot be provenance-checked, so it is a failure (not silently kept).
func TestCleanupReportsUnreadableSidecar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"proj/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`})
	sidecar := filepath.Join(dir, "proj", ".astro", "dbt_metadata.json")
	if err := os.Chmod(sidecar, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sidecar, 0o644) })

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if summary.CountFailed() != 1 {
		t.Fatalf("failed = %d, want 1", summary.CountFailed())
	}
	var out bytes.Buffer
	summary.WriteReport(&out)
	if !strings.Contains(out.String(), "✗") {
		t.Fatalf("failure missing from report: %q", out.String())
	}
}

// TestCleanupReportsUnremovableSidecar: a sidecar that passes the provenance
// check but cannot be unlinked is recorded as failed, not dropped.
func TestCleanupReportsUnremovableSidecar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"proj/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`})
	astroDir := filepath.Join(dir, "proj", ".astro")
	if err := os.Chmod(astroDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(astroDir, 0o755) })

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if summary.CountFailed() != 1 {
		t.Fatalf("failed = %d, want 1", summary.CountFailed())
	}
}

// TestCleanupSkipsUnreadableGeneratedDirs: logs/ and dbt_packages/ never hold
// sidecars (discovery skips them by name), so cleanup must not fail over
// them even when they are unreadable, e.g. a root-owned bind-mount leftover.
func TestCleanupSkipsUnreadableGeneratedDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission semantics differ on windows")
	}
	dir := t.TempDir()
	for _, name := range []string{"logs", "dbt_packages"} {
		locked := filepath.Join(dir, "proj", name)
		if err := os.MkdirAll(locked, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	}

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup must not fail over generated dirs it never writes to: %v", err)
	}
	if len(summary.Results) != 0 {
		t.Fatalf("results = %d, want 0", len(summary.Results))
	}
}

// TestCleanupStillVisitsTarget: target/ can legitimately hold a compiled
// manifest's sidecar at target/.astro/, so it is NOT among the skipped names.
func TestCleanupStillVisitsTarget(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"proj/target/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`,
	})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(summary.Results) != 1 || summary.CountFailed() != 0 || summary.CountKept() != 0 {
		t.Fatalf("results = %+v, want the target/.astro sidecar removed", summary.Results)
	}
}

// TestCleanupCleansRootsNamedLikeSkipDirs mirrors discovery's root exemption:
// a project living in a directory named logs or dbt_packages can be stamped,
// so cleanup of that same root must not skip it - otherwise a sidecar could
// survive EnsureClean and ship stale. .git stays skipped even as the root.
func TestCleanupCleansRootsNamedLikeSkipDirs(t *testing.T) {
	for _, name := range []string{"logs", "dbt_packages"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), name)
			writeFiles(t, root, map[string]string{
				"proj/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`,
			})

			summary, err := Cleanup([]string{root})
			if err != nil {
				t.Fatalf("Cleanup: %v", err)
			}
			if len(summary.Results) != 1 || summary.CountFailed() != 0 || summary.CountKept() != 0 {
				t.Fatalf("results = %+v, want the sidecar under the %s root removed", summary.Results, name)
			}
		})
	}

	t.Run(".git stays protected", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), ".git")
		writeFiles(t, root, map[string]string{
			"trap/.astro/dbt_metadata.json": `{"generated_by": {"application": "astro"}}`,
		})

		summary, err := Cleanup([]string{root})
		if err != nil {
			t.Fatalf("Cleanup: %v", err)
		}
		if len(summary.Results) != 0 {
			t.Fatalf("results = %+v, want nothing visited under a .git root", summary.Results)
		}
		if _, err := os.Stat(filepath.Join(root, "trap", ".astro", "dbt_metadata.json")); err != nil {
			t.Fatalf("file under .git was touched: %v", err)
		}
	})
}
