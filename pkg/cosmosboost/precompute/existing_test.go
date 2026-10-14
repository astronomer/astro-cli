package precompute

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

const (
	oursSidecar  = `{"schema":1,"version":{"algo":"sha256-tree-v2","hash":"old"},"generated_by":{"application":"astro","version":"1.0.0"}}`
	foreignSlim  = `{"_schema":1,"_generated_by":{"application":"someone-else"}}`
	oursSlim     = `{"_schema":1,"_generated_by":{"application":"astro","version":"1.0.0"}}`
	foreignNoTag = `{"schema":1,"version":{"hash":"abc"}}`
)

// existingByRel is FindExisting's result keyed by root-relative slash path.
func existingByRel(t *testing.T, root string) map[string]bool {
	t.Helper()
	found, err := FindExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range found {
		rel, err := filepath.Rel(root, e.Path)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.ToSlash(rel)] = e.Ours
	}
	return out
}

// TestFindExistingJudgesEachFileByItsOwnMarker: provenance is read per file,
// never inferred from a neighbor, and a file with no marker is not ours.
func TestFindExistingJudgesEachFileByItsOwnMarker(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/.astro/dbt_metadata.json":         oursSidecar,
		"proj/.astro/manifest.slim.json":        foreignSlim,
		"proj/target/.astro/dbt_metadata.json":  foreignNoTag,
		"proj/target/.astro/manifest.slim.json": oursSlim,
		"orphan/.astro/manifest.slim.json":      oursSlim,
		"proj/.astro/config.yaml":               "not ours to judge",
		"proj/dbt_metadata.json":                oursSidecar, // not under .astro
		"proj/.astro/nested/dbt_metadata.json":  oursSidecar, // not directly under .astro
	})

	got := existingByRel(t, root)
	want := map[string]bool{
		"proj/.astro/dbt_metadata.json":         true,
		"proj/.astro/manifest.slim.json":        false,
		"proj/target/.astro/dbt_metadata.json":  false,
		"proj/target/.astro/manifest.slim.json": true,
		"orphan/.astro/manifest.slim.json":      true,
	}
	if len(got) != len(want) {
		t.Fatalf("found %v, want %v", got, want)
	}
	for path, ours := range want {
		if g, ok := got[path]; !ok || g != ours {
			t.Fatalf("%s: found=%v ours=%v, want ours=%v (all: %v)", path, ok, g, ours, got)
		}
	}
}

// TestFindExistingSkipsGitAndGeneratedDirs: .git is never walked, and logs/
// and dbt_packages/ below the root are skipped even when unreadable, while
// target/ is visited and a root named like a skipped directory is still
// scanned.
func TestFindExistingSkipsGitAndGeneratedDirs(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "logs")
	writeFiles(t, root, map[string]string{
		".git/x/.astro/dbt_metadata.json":            oursSidecar,
		"proj/logs/.astro/dbt_metadata.json":         oursSidecar,
		"proj/dbt_packages/.astro/dbt_metadata.json": oursSidecar,
		"proj/target/.astro/dbt_metadata.json":       oursSidecar,
		".astro/dbt_metadata.json":                   oursSidecar,
	})
	if runtime.GOOS != "windows" {
		locked := filepath.Join(root, "proj", "logs")
		if err := os.Chmod(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	}

	got := existingByRel(t, root)
	var paths []string
	for p := range got {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	want := []string{".astro/dbt_metadata.json", "proj/target/.astro/dbt_metadata.json"}
	if len(paths) != len(want) || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("found %v, want %v", paths, want)
	}
}

// TestFindExistingFailsOnUnreadableArtifact: a file nobody can read cannot be
// attributed, so the scan fails rather than guessing.
func TestFindExistingFailsOnUnreadableArtifact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission semantics differ on windows")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"proj/.astro/dbt_metadata.json": oursSidecar})
	path := filepath.Join(root, "proj", ".astro", "dbt_metadata.json")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	if _, err := FindExisting(root); err == nil {
		t.Fatal("FindExisting over an unreadable sidecar: error = nil, want non-nil")
	}
}

func TestFindExistingNonexistentRoot(t *testing.T) {
	if _, err := FindExisting(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("FindExisting on a missing root: error = nil, want non-nil")
	}
}
