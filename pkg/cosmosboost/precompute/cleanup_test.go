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
	summary, err := Run([]string{dir}, "test", Options{SlimManifest: true})
	if err != nil || summary.CountFailed() > 0 {
		t.Fatalf("stamping fixture project failed: err=%v failed=%d", err, summary.CountFailed())
	}
}

// stampManifest is stampProject's sibling for the other unit kind: it drops a
// minimal dbt manifest.json under dir and runs the real pre-deploy over it, so
// the tests below exercise a sidecar *and* a slim manifest exactly as the
// producer writes them. (stampProject writes dbt_project.yml, which produces a
// project sidecar and no manifest, so it can't stand in here.)
func stampManifest(t *testing.T, dir string) {
	t.Helper()
	writeFiles(t, dir, map[string]string{
		"manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`,
	})
	summary, err := Run([]string{dir}, "test", Options{SlimManifest: true})
	if err != nil || summary.CountFailed() > 0 {
		t.Fatalf("stamping fixture manifest failed: err=%v failed=%d", err, summary.CountFailed())
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

// TestCleanupRemovesSlimManifestAlongsideSidecar: a slim manifest is only
// meaningful alongside a fresh hash sidecar, so cleanup must remove a stale
// one too - otherwise a later deploy (with a changed manifest, or with
// cosmos_boost.pre_deploy disabled) could still ship an outdated
// manifest.slim.json.
func TestCleanupRemovesSlimManifestAlongsideSidecar(t *testing.T) {
	dir := t.TempDir()
	stampManifest(t, dir)
	slimPath := filepath.Join(dir, sidecarDir, slimManifestName)
	if _, err := os.Stat(slimPath); err != nil {
		t.Fatalf("fixture setup: slim manifest not written: %v", err)
	}

	cleanupSummary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	// Each artifact is removed on its own terms, so each gets its own entry.
	if got := len(cleanupSummary.Results); got != 2 {
		t.Fatalf("results = %d, want 2 (one per artifact removed)", got)
	}
	if cleanupSummary.CountFailed() != 0 || cleanupSummary.CountKept() != 0 {
		t.Fatalf("failed=%d kept=%d, want 0/0", cleanupSummary.CountFailed(), cleanupSummary.CountKept())
	}

	if _, err := os.Stat(slimPath); !os.IsNotExist(err) {
		t.Fatalf("slim manifest still present after cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sidecarDir)); !os.IsNotExist(err) {
		t.Fatalf(".astro dir not pruned after removing both files: %v", err)
	}
}

// TestCleanupJudgesEachArtifactSeparately: provenance is read per file, never
// inferred from the neighbor. Deleting on the neighbor's marker would destroy
// a file we do not own in one direction, and strand a stale artifact of ours -
// unremoved and unreported - in the other.
func TestCleanupJudgesEachArtifactSeparately(t *testing.T) {
	for _, tc := range []struct {
		name     string
		foreign  string // artifact to overwrite with another producer's marker
		marker   string
		wantKept string // artifact that must survive
		wantGone string
	}{
		{
			name:    "foreign slim manifest beside our sidecar",
			foreign: slimManifestName, marker: `{"_generated_by": {"application": "someone-else"}}`,
			wantKept: slimManifestName, wantGone: sidecarName,
		},
		{
			name:    "our slim manifest beside a foreign sidecar",
			foreign: sidecarName, marker: `{"generated_by": {"application": "someone-else"}}`,
			wantKept: sidecarName, wantGone: slimManifestName,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			stampManifest(t, dir)
			astroDir := filepath.Join(dir, sidecarDir)
			if err := os.WriteFile(filepath.Join(astroDir, tc.foreign), []byte(tc.marker), 0o644); err != nil {
				t.Fatal(err)
			}

			summary, err := Cleanup([]string{dir})
			if err != nil {
				t.Fatalf("Cleanup: %v", err)
			}
			if summary.CountKept() != 1 || summary.CountFailed() != 0 {
				t.Fatalf("kept=%d failed=%d, want 1/0", summary.CountKept(), summary.CountFailed())
			}
			if _, err := os.Stat(filepath.Join(astroDir, tc.wantKept)); err != nil {
				t.Fatalf("%s must survive: %v", tc.wantKept, err)
			}
			if _, err := os.Stat(filepath.Join(astroDir, tc.wantGone)); !os.IsNotExist(err) {
				t.Fatalf("%s must be removed: %v", tc.wantGone, err)
			}
		})
	}
}

// TestCleanupRemovesOrphanedSlimManifest: a slim manifest can end up without
// its sidecar - an older astro-cli's `cleanup`/EnsureClean deletes only the
// sidecar it knows about, or EnsureClean leaves a foreign sidecar in place -
// and nothing walking for dbt_metadata.json would ever find it again. Cleanup
// must check and remove it on its own terms, via its own _generated_by
// marker.
func TestCleanupRemovesOrphanedSlimManifest(t *testing.T) {
	dir := t.TempDir()
	stampManifest(t, dir)
	astroDir := filepath.Join(dir, sidecarDir)
	slimPath := filepath.Join(astroDir, slimManifestName)
	if _, err := os.Stat(slimPath); err != nil {
		t.Fatalf("fixture setup: slim manifest not written: %v", err)
	}
	// Simulate the sidecar being gone without this file's knowledge: an older
	// astro-cli's cleanup, or a hand removal.
	if err := os.Remove(filepath.Join(astroDir, sidecarName)); err != nil {
		t.Fatal(err)
	}

	cleanupSummary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if cleanupSummary.CountFailed() != 0 || cleanupSummary.CountKept() != 0 {
		t.Fatalf("failed=%d kept=%d, want 0/0", cleanupSummary.CountFailed(), cleanupSummary.CountKept())
	}
	if got := len(cleanupSummary.Results); got != 1 {
		t.Fatalf("results = %d, want 1 (the orphaned slim manifest)", got)
	}
	if _, err := os.Stat(slimPath); !os.IsNotExist(err) {
		t.Fatalf("orphaned slim manifest still present after cleanup: %v", err)
	}
	if _, err := os.Stat(astroDir); !os.IsNotExist(err) {
		t.Fatalf(".astro dir not pruned after removing the orphaned slim manifest: %v", err)
	}
}

// TestCleanupKeepsForeignOrphanedSlimManifest: an orphaned slim manifest
// checks its own provenance the same way a sidecar does - one this tool did
// not write (or that isn't valid JSON) must never be deleted.
func TestCleanupKeepsForeignOrphanedSlimManifest(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".astro/manifest.slim.json": `{"_generated_by": {"application": "someone-else"}}`,
	})

	summary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := summary.CountKept(); got != 1 {
		t.Fatalf("kept = %d, want 1", got)
	}
	if _, err := os.Stat(filepath.Join(dir, sidecarDir, slimManifestName)); err != nil {
		t.Fatalf("foreign orphaned slim manifest was removed: %v", err)
	}
}

// TestCleanupAttributesRemovalFailuresPerArtifact: a failure must name the
// file that actually could not be removed. Reporting one artifact's error
// against its neighbor's path sends the user to fix the wrong file, and the
// next deploy fails on the same unnamed one.
func TestCleanupAttributesRemovalFailuresPerArtifact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission semantics differ on windows")
	}
	dir := t.TempDir()
	stampManifest(t, dir)
	astroDir := filepath.Join(dir, sidecarDir)
	sidecarPath := filepath.Join(astroDir, sidecarName)
	slimPath := filepath.Join(astroDir, slimManifestName)
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Fatalf("fixture setup: sidecar not written: %v", err)
	}

	// A read-only .astro makes both removals fail.
	if err := os.Chmod(astroDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(astroDir, 0o755) })

	cleanupSummary, err := Cleanup([]string{dir})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if cleanupSummary.CountFailed() != 2 {
		t.Fatalf("failed = %d, want 2 (neither artifact removable)", cleanupSummary.CountFailed())
	}
	failed := map[string]bool{}
	for _, r := range cleanupSummary.Results {
		if r.Err != nil {
			failed[r.Path] = true
		}
	}
	for _, want := range []string{sidecarPath, slimPath} {
		if !failed[want] {
			t.Fatalf("no failure reported against %s; got %+v", want, cleanupSummary.Results)
		}
	}
	if err := os.Chmod(astroDir, 0o755); err != nil { // restore before the stats below
		t.Fatal(err)
	}
	for _, path := range []string{sidecarPath, slimPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s must survive a failed removal so Cleanup can retry it: %v", path, err)
		}
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
