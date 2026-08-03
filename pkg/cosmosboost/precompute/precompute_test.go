package precompute

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunDiscoversProjectsAndWritesSidecars(t *testing.T) {
	root := t.TempDir()
	// Two dbt projects at different depths (one under dags/, one under include/),
	// plus a non-project file that must be ignored.
	writeFiles(t, root, map[string]string{
		"dags/dbt/shop/dbt_project.yml":   "name: shop\n",
		"dags/dbt/shop/models/a.sql":      "select 1",
		"include/finance/dbt_project.yml": "name: finance\n",
		"include/finance/models/f.sql":    "select 2",
		"dags/plain_dag.py":               "print('hi')",
	})

	summary, err := Run([]string{root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(summary.Results), summary.Results)
	}
	if summary.CountFailed() != 0 {
		t.Fatalf("unexpected failures: %+v", summary.Results)
	}

	for _, r := range summary.Results {
		if r.Kind != "project" {
			t.Fatalf("want kind=project, got %q for %s", r.Kind, r.Path)
		}
		sidecar := filepath.Join(r.Path, sidecarDir, sidecarName)
		var m Metadata
		readJSON(t, sidecar, &m)
		if m.Schema != schemaVersion || m.Version.Algo != algoProjectTree || m.Version.Hash != r.Hash {
			t.Fatalf("sidecar mismatch for %s: %+v (result hash %s)", r.Path, m, r.Hash)
		}
		if m.GeneratedBy.Application != application || m.GeneratedBy.Version != "test" {
			t.Fatalf("generated_by mismatch for %s: %+v", r.Path, m.GeneratedBy)
		}
	}
}

// TestRunManifestOnly covers a manifest-only deployment: a shipped manifest.json
// with no surrounding dbt project still gets a sidecar (DBT_MANIFEST path).
func TestRunManifestOnly(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"shipped/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","generated_at":"t"},"nodes":{"model.x":{"name":"x"}}}`,
	})

	summary, err := Run([]string{root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || summary.Results[0].Kind != "manifest" {
		t.Fatalf("want 1 manifest result, got %+v", summary.Results)
	}

	var m Metadata
	readJSON(t, filepath.Join(root, "shipped", sidecarDir, sidecarName), &m)
	if m.Version.Algo != algoManifestJSON || m.Version.Hash == "" {
		t.Fatalf("manifest sidecar mismatch: %+v", m)
	}
}

// TestRunProjectWithTargetManifest covers the common full-project case: the
// project gets a folder-hash sidecar, and its target/manifest.json gets its own
// manifest sidecar (target/ is excluded from the folder hash).
func TestRunProjectWithTargetManifest(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml":      "name: p\n",
		"proj/models/a.sql":         "select 1",
		"proj/target/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","generated_at":"t"},"nodes":{}}`,
	})

	summary, err := Run([]string{root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, r := range summary.Results {
		if r.Err != nil {
			t.Fatalf("unexpected error for %s: %v", r.Path, r.Err)
		}
		kinds[r.Kind]++
	}
	if kinds["project"] != 1 || kinds["manifest"] != 1 {
		t.Fatalf("want 1 project + 1 manifest, got %+v (%+v)", kinds, summary.Results)
	}
	mustExist(t, filepath.Join(root, "proj", sidecarDir, sidecarName))
	mustExist(t, filepath.Join(root, "proj", "target", sidecarDir, sidecarName))
}

// TestRunSkipsNonDBTManifest verifies an unrelated manifest.json (no dbt shape)
// is discovered but skipped — no sidecar is written beside it.
func TestRunSkipsNonDBTManifest(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"webapp/manifest.json": `{"name":"My App","icons":[]}`,
	})

	summary, err := Run([]string{root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || !summary.Results[0].Skipped {
		t.Fatalf("want 1 skipped manifest, got %+v", summary.Results)
	}
	if summary.CountSkipped() != 1 {
		t.Fatalf("Skipped() = %d, want 1", summary.CountSkipped())
	}
	if _, err := os.Stat(filepath.Join(root, "webapp", sidecarDir, sidecarName)); !os.IsNotExist(err) {
		t.Fatalf("a sidecar was written beside a non-dbt manifest (err=%v)", err)
	}
}

// TestRunWarnsOnTemplatedPackagesPath verifies a project whose packages-install-path
// is a Jinja template still gets stamped, but the Result carries a non-fatal warning.
func TestRunWarnsOnTemplatedPackagesPath(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml": "name: p\npackages-install-path: \"{{ env_var('DBT_PKG_DIR') }}\"\n",
		"proj/models/a.sql":    "select 1",
	})

	summary, err := Run([]string{root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("want 1 result, got %+v", summary.Results)
	}
	r := summary.Results[0]
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Warning == "" {
		t.Fatal("expected a warning for a templated packages-install-path")
	}
	mustExist(t, filepath.Join(root, "proj", sidecarDir, sidecarName))
}

func TestRunIsDeterministicAcrossRuns(t *testing.T) {
	root := t.TempDir()
	// Several projects so the worker pool runs them concurrently.
	for _, name := range []string{"a", "b", "c", "d"} {
		writeFiles(t, root, map[string]string{
			name + "/dbt_project.yml": "name: " + name + "\n",
			name + "/models/m.sql":    "select '" + name + "'",
		})
	}

	first := hashesByPath(t, root)
	for i := 0; i < 5; i++ {
		got := hashesByPath(t, root)
		for path, h := range got {
			if first[path] != h {
				t.Fatalf("non-deterministic hash for %s: %s != %s", path, first[path], h)
			}
		}
	}
}

// TestSummaryWrite exercises the human-readable report across all branches
// (stamped, skipped, failed, and a warning line) without needing to trigger a
// real failure on disk.
func TestSummaryWrite(t *testing.T) {
	s := Summary{
		Duration: 1234 * time.Microsecond,
		Results: []Result{
			{Kind: "project", Path: "/p/ok", Hash: "0123456789abcdef0123", Files: 3, Bytes: 42, Duration: time.Microsecond},
			{Kind: "project", Path: "/p/warn", Hash: "abcdef0123456789abcd", Files: 1, Bytes: 7, Duration: time.Microsecond, Warning: "packages-install-path is a Jinja template"},
			{Kind: "manifest", Path: "/p/web/manifest.json", Skipped: true},
			{Kind: "project", Path: "/p/bad", Err: errors.New("boom")},
		},
	}

	var buf bytes.Buffer
	s.WriteReport(&buf)
	out := buf.String()

	for _, want := range []string{
		"2 stamped, 1 skipped, 1 failed", // 2 ok (incl. the warned one) + 1 skipped + 1 failed
		"hash=0123456789ab",              // shortHash truncates to 12 chars
		"not a dbt manifest",
		"(boom)",
		"⚠ packages-install-path is a Jinja template",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q\n--- report ---\n%s", want, out)
		}
	}
}

func hashesByPath(t *testing.T, root string) map[string]string {
	t.Helper()
	s, err := Run([]string{root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]string, len(s.Results))
	for _, r := range s.Results {
		if r.Err != nil {
			t.Fatalf("unexpected error for %s: %v", r.Path, r.Err)
		}
		m[r.Path] = r.Hash
	}
	return m
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

// TestRunProjectHashStableWithNestedManifestSidecar guards the hash-stability
// concern: a dbt manifest.json in a subdirectory (e.g. docs/) gets its own sidecar
// written into the walked tree (docs/.astro/). Since manifest and project units run
// concurrently, the enclosing project's hash must not depend on that sidecar — so it
// must be identical before and after the sidecar exists on disk.
func TestRunProjectHashStableWithNestedManifestSidecar(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml":    "name: p\n",
		"proj/models/a.sql":       "select 1",
		"proj/docs/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","generated_at":"t"},"nodes":{}}`,
	})

	projectHash := func() string {
		s, err := Run([]string{root}, "test")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range s.Results {
			if r.Kind == "project" {
				if r.Err != nil {
					t.Fatalf("unexpected error: %v", r.Err)
				}
				return r.Hash
			}
		}
		t.Fatal("no project result")
		return ""
	}

	first := projectHash()  // writes proj/.astro and proj/docs/.astro
	second := projectHash() // the nested sidecar now exists on disk
	if first != second {
		t.Fatalf("project hash changed once the nested docs/.astro sidecar existed:\n first  %s\n second %s", first, second)
	}
	// Sanity: the nested docs/manifest.json did get its own sidecar (so the walk
	// really does encounter docs/.astro and correctly skips it).
	mustExist(t, filepath.Join(root, "proj", "docs", sidecarDir, sidecarName))
}

func TestShortHash(t *testing.T) {
	if got := shortHash("abcdef"); got != "abcdef" { // <= 12 chars: returned as-is
		t.Fatalf("shortHash(short) = %q, want abcdef", got)
	}
	if got := shortHash("0123456789abcdef0123"); got != "0123456789ab" { // truncated to 12
		t.Fatalf("shortHash(long) = %q, want 0123456789ab", got)
	}
}
