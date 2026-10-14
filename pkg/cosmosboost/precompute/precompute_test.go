package precompute

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
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
		readJSON(t, summary, sidecar, &m)
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

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || summary.Results[0].Kind != "manifest" {
		t.Fatalf("want 1 manifest result, got %+v", summary.Results)
	}

	var m Metadata
	readJSON(t, summary, filepath.Join(root, "shipped", sidecarDir, sidecarName), &m)
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

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
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
	mustProduce(t, summary, filepath.Join(root, "proj", sidecarDir, sidecarName))
	mustProduce(t, summary, filepath.Join(root, "proj", "target", sidecarDir, sidecarName))
}

// TestRunSkipsNonDBTManifest verifies an unrelated manifest.json (no dbt shape)
// is discovered but skipped — no sidecar is written beside it.
func TestRunSkipsNonDBTManifest(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"webapp/manifest.json": `{"name":"My App","icons":[]}`,
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || !summary.Results[0].Skipped {
		t.Fatalf("want 1 skipped manifest, got %+v", summary.Results)
	}
	if summary.CountSkipped() != 1 {
		t.Fatalf("Skipped() = %d, want 1", summary.CountSkipped())
	}
	mustNotProduce(t, summary, filepath.Join(root, "webapp", sidecarDir, sidecarName))
}

// TestRunWarnsOnTemplatedPackagesPath verifies a project whose packages-install-path
// is a Jinja template still gets stamped, but the Result carries a non-fatal warning.
func TestRunWarnsOnTemplatedPackagesPath(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml": "name: p\npackages-install-path: \"{{ env_var('DBT_PKG_DIR') }}\"\n",
		"proj/models/a.sql":    "select 1",
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
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
	mustProduce(t, summary, filepath.Join(root, "proj", sidecarDir, sidecarName))
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
	s, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]string, len(s.Results))
	for i := range s.Results {
		r := &s.Results[i]
		if r.Err != nil {
			t.Fatalf("unexpected error for %s: %v", r.Path, r.Err)
		}
		m[r.Path] = r.Hash
	}
	return m
}

// artifact returns the bytes s produced for path, and whether it produced any.
func artifact(s Summary, path string) ([]byte, bool) {
	for _, a := range s.Artifacts() {
		if a.Path == path {
			return a.Data, true
		}
	}
	return nil, false
}

func readJSON(t *testing.T, s Summary, path string, v any) {
	t.Helper()
	data, ok := artifact(s, path)
	if !ok {
		t.Fatalf("no artifact produced for %s", path)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}

func mustProduce(t *testing.T, s Summary, path string) {
	t.Helper()
	if _, ok := artifact(s, path); !ok {
		t.Fatalf("expected an artifact for %s", path)
	}
}

func mustNotProduce(t *testing.T, s Summary, path string) {
	t.Helper()
	if _, ok := artifact(s, path); ok {
		t.Fatalf("unexpected artifact for %s", path)
	}
}

// TestRunProjectHashStableWithNestedManifestSidecar guards the hash-stability
// concern: a dbt manifest.json in a subdirectory (e.g. docs/) gets its own sidecar
// at docs/.astro/ in the bundle. The enclosing project's hash must not depend on
// that sidecar, so it must be identical whether or not one exists on disk.
func TestRunProjectHashStableWithNestedManifestSidecar(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml":    "name: p\n",
		"proj/models/a.sql":       "select 1",
		"proj/docs/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","generated_at":"t"},"nodes":{}}`,
	})

	projectHash := func() string {
		s, err := Run([]string{root}, "test", Options{SlimManifest: true})
		if err != nil {
			t.Fatal(err)
		}
		for i := range s.Results {
			r := &s.Results[i]
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

	first := projectHash()
	// Put the nested sidecar on disk, as a stale one from an earlier astro-cli
	// (or the unpacked bundle) would be.
	writeFiles(t, root, map[string]string{"proj/docs/.astro/dbt_metadata.json": `{"generated_by":{"application":"astro"}}`})
	second := projectHash()
	if first != second {
		t.Fatalf("project hash changed once the nested docs/.astro sidecar existed:\n first  %s\n second %s", first, second)
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash("abcdef"); got != "abcdef" { // <= 12 chars: returned as-is
		t.Fatalf("shortHash(short) = %q, want abcdef", got)
	}
	if got := shortHash("0123456789abcdef0123"); got != "0123456789ab" { // truncated to 12
		t.Fatalf("shortHash(long) = %q, want 0123456789ab", got)
	}
}

// TestRunSkipsGitInternals pins that discovery never descends into .git: a
// dbt_project.yml or manifest.json inside VCS internals is not a deployable
// unit, and stamping it would write into (and later delete from) .git.
func TestRunSkipsGitInternals(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"proj/dbt_project.yml":      "name: shop\n",
		"proj/models/a.sql":         "select 1",
		".git/trap/dbt_project.yml": "name: trap\n",
		".git/deep/manifest.json":   `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`,
	})

	summary, err := Run([]string{dir}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(summary.Results); got != 1 {
		t.Fatalf("results = %d, want 1 (only the real project)", got)
	}
	for _, a := range summary.Artifacts() {
		if strings.Contains(filepath.ToSlash(a.Path), "/.git/") {
			t.Fatalf("produced an artifact inside VCS internals: %s", a.Path)
		}
	}
}

func TestReadDbtConfigMissingFile(t *testing.T) {
	got := readDbtConfig(t.TempDir())
	if got.packagesInstallPath != "" || got.targetPath != "" || got.logPath != "" || len(got.templatedSettings) != 0 {
		t.Fatalf("readDbtConfig on a dir without dbt_project.yml = %+v, want zero value", got)
	}
}

// TestRunWarnsOnAllTemplatedDirSettings: the Jinja guard covers target-path
// and log-path the same way it covers packages-install-path, and the warning
// names every templated setting.
func TestRunWarnsOnAllTemplatedDirSettings(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"proj/dbt_project.yml": "name: shop\n" +
			"packages-install-path: \"{{ env_var('P') }}\"\n" +
			"target-path: \"{{ env_var('T') }}\"\n" +
			"log-path: \"{{ env_var('L') }}\"\n",
		"proj/models/a.sql": "select 1",
	})

	summary, err := Run([]string{dir}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(summary.Results))
	}
	w := summary.Results[0].Warning
	for _, setting := range []string{"packages-install-path", "target-path", "log-path"} {
		if !strings.Contains(w, setting) {
			t.Fatalf("warning %q does not name templated setting %s", w, setting)
		}
	}
}

// TestReadDbtConfigTemplatedValuesFallBackToDefaults: a templated setting is
// treated as unset, so the dbt default directory stays excluded rather than a
// meaningless literal.
func TestReadDbtConfigTemplatedValuesFallBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\ntarget-path: \"{{ var('t') }}\"\nlog-path: logs_custom\n",
	})
	cfg := readDbtConfig(dir)
	if cfg.targetPath != "" {
		t.Fatalf("templated target-path must resolve to unset, got %q", cfg.targetPath)
	}
	if cfg.logPath != "logs_custom" {
		t.Fatalf("literal log-path must survive, got %q", cfg.logPath)
	}
	if len(cfg.templatedSettings) != 1 || cfg.templatedSettings[0] != "target-path" {
		t.Fatalf("templatedSettings = %v, want [target-path]", cfg.templatedSettings)
	}
}

func TestRunErrorsOnUnreadableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission semantics differ on windows")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := Run([]string{dir}, "test", Options{SlimManifest: true}); err == nil {
		t.Fatal("Run over a tree with an unreadable directory: error = nil, want non-nil")
	}
}

// TestRunErrorsOnUnreadableTargetDir hits the manifest walk's error path:
// project discovery skips target/ by name and never descends into a found
// project, so only the manifest walk reaches the unreadable directory.
func TestRunErrorsOnUnreadableTargetDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"proj/dbt_project.yml": "name: shop\n"})
	target := filepath.Join(dir, "proj", "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o755) })

	if _, err := Run([]string{dir}, "test", Options{SlimManifest: true}); err == nil {
		t.Fatal("Run: error = nil, want manifest-scan error")
	}
}

// TestRunReportsFailedUnits: per-unit failures land in the unit's Result and
// do not abort the run.
func TestRunReportsFailedUnits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"broken/dbt_project.yml":   "name: broken\n",
		"broken/models/a.sql":      "select 1",
		"lockdir/dbt_project.yml":  "name: lockdir\n",
		"standalone/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`,
	})
	// An unreadable file fails hashing mid-walk.
	if err := os.Chmod(filepath.Join(dir, "broken", "models", "a.sql"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "broken", "models", "a.sql"), 0o644) })
	// An unreadable directory inside a project fails the hash walk itself.
	// models/logs is skipped by BOTH discovery walks (name-based) but visited
	// by the hash walk (whose logs exclusion is root-relative only), so the
	// failure surfaces in the unit, not in discovery.
	locked := filepath.Join(dir, "lockdir", "models", "logs")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	// An unreadable manifest fails its unit.
	if err := os.Chmod(filepath.Join(dir, "standalone", "manifest.json"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "standalone", "manifest.json"), 0o644) })

	summary, err := Run([]string{dir}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := summary.CountFailed(); got != 3 {
		t.Fatalf("failed = %d, want 3 (broken project, locked project, unreadable manifest)", got)
	}
}

// TestRunWritesSlimManifestAlongsideSidecar: with the option set, every
// discovered manifest.json gets a slim copy next to its hash sidecar, and the
// sidecar carries the filtered_manifest pointer at it. The pointer hashes the
// slim file's own bytes, so a consumer can confirm the two are a matched pair -
// which adjacency alone does not imply, since each file on disk is judged by
// its own producer marker.
func TestRunWritesSlimManifestAlongsideSidecar(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"shipped/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","project_name":"shop"},"nodes":{"model.shop.orders":{"original_file_path":"models/orders.sql","package_name":"shop","resource_type":"model","fqn":["shop","orders"]}}}`,
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || summary.Results[0].Err != nil {
		t.Fatalf("want 1 successful manifest result, got %+v", summary.Results)
	}

	astroDir := filepath.Join(root, "shipped", sidecarDir)
	var slim map[string]any
	readJSON(t, summary, filepath.Join(astroDir, slimManifestName), &slim)
	if _, ok := slim["nodes"].(map[string]any)["model.shop.orders"]; !ok {
		t.Fatalf("slim manifest missing expected node: %+v", slim)
	}

	var meta Metadata
	readJSON(t, summary, filepath.Join(astroDir, sidecarName), &meta)
	fm := meta.FilteredManifest
	if fm == nil {
		t.Fatalf("sidecar has no filtered_manifest section: %+v", meta)
	}
	if fm.Path != slimManifestName || fm.Schema != slimSchemaVersion || fm.Version.Algo != algoFilteredManifest {
		t.Fatalf("filtered_manifest = %+v", fm)
	}
	slimData, _ := artifact(summary, filepath.Join(astroDir, slimManifestName))
	if want := sha256Hex(slimData); fm.Version.Hash != want {
		t.Fatalf("filtered_manifest hash = %q, want the slim file's own hash %q", fm.Version.Hash, want)
	}
	if fm.Version.Hash == meta.Version.Hash {
		t.Fatal("filtered_manifest hash must not be the full manifest's hash")
	}
}

// TestRunSkipsSlimManifestWhenNotRequested is the other side of the switch: the
// hash sidecar is still written, with no slim manifest and no pointer key - so
// a consumer's presence check is enough to fall back to the full manifest.
func TestRunSkipsSlimManifestWhenNotRequested(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"shipped/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","project_name":"shop"},"nodes":{}}`,
	})

	summary, err := Run([]string{root}, "test", Options{})
	if err != nil {
		t.Fatal(err)
	}

	astroDir := filepath.Join(root, "shipped", sidecarDir)
	mustNotProduce(t, summary, filepath.Join(astroDir, slimManifestName))
	raw, ok := artifact(summary, filepath.Join(astroDir, sidecarName))
	if !ok {
		t.Fatal("no sidecar produced")
	}
	if strings.Contains(string(raw), "filtered_manifest") {
		t.Fatalf("filtered_manifest emitted with the slim manifest disabled: %s", raw)
	}
}

// TestRunSlimsManifestInProjectRoot: a manifest.json beside dbt_project.yml is
// not a discovery unit (its .astro/ is the project's own), so it used to get no
// slim copy at all - the projects most likely to have one got nothing from this
// feature. It is slimmed into the project's .astro/, with the project sidecar
// carrying the pointer and keeping its own tree hash.
func TestRunSlimsManifestInProjectRoot(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml": "name: shop\n",
		"proj/models/a.sql":    "select 1",
		"proj/manifest.json":   `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","project_name":"shop"},"nodes":{"model.shop.orders":{"original_file_path":"models/orders.sql","package_name":"shop","resource_type":"model","fqn":["shop","orders"],"raw_code":"select 1"}}}`,
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	// One unit only: the manifest must not also be discovered on its own.
	if len(summary.Results) != 1 || summary.Results[0].Kind != kindProject || summary.Results[0].Err != nil {
		t.Fatalf("want 1 successful project result, got %+v", summary.Results)
	}

	astroDir := filepath.Join(root, "proj", sidecarDir)
	var slim map[string]any
	readJSON(t, summary, filepath.Join(astroDir, slimManifestName), &slim)
	if _, ok := slim["nodes"].(map[string]any)["model.shop.orders"]; !ok {
		t.Fatalf("slim manifest missing expected node: %+v", slim)
	}

	var meta Metadata
	readJSON(t, summary, filepath.Join(astroDir, sidecarName), &meta)
	if meta.Version.Algo != algoProjectTree {
		t.Fatalf("project sidecar algo = %q, want %q", meta.Version.Algo, algoProjectTree)
	}
	if meta.FilteredManifest == nil || meta.FilteredManifest.Path != slimManifestName {
		t.Fatalf("project sidecar does not point at the slim manifest: %+v", meta.FilteredManifest)
	}
}

// TestRunProjectHashIgnoresRootSlimManifest: the slim manifest lands inside the
// hashed tree, so the project hash must not move because of it - otherwise
// enabling the optimization would invalidate every cache key once.
func TestRunProjectHashIgnoresRootSlimManifest(t *testing.T) {
	hashWith := func(t *testing.T, opts Options) string {
		t.Helper()
		root := t.TempDir()
		writeFiles(t, root, map[string]string{
			"proj/dbt_project.yml": "name: shop\n",
			"proj/models/a.sql":    "select 1",
			"proj/manifest.json":   `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`,
		})
		summary, err := Run([]string{root}, "test", opts)
		if err != nil || len(summary.Results) != 1 {
			t.Fatalf("Run: err=%v results=%+v", err, summary.Results)
		}
		return summary.Results[0].Hash
	}

	if with, without := hashWith(t, Options{SlimManifest: true}), hashWith(t, Options{}); with != without {
		t.Fatalf("writing the slim manifest changed the project hash:\n with    %s\n without %s", with, without)
	}
}

// TestRunSkipsNonDbtManifestInProjectRoot: a project root can hold an unrelated
// manifest.json (a web app's PWA manifest), which must not be slimmed or
// pointed at.
func TestRunSkipsNonDbtManifestInProjectRoot(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml": "name: shop\n",
		"proj/manifest.json":   `{"name":"My App","short_name":"App","icons":[]}`,
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}

	astroDir := filepath.Join(root, "proj", sidecarDir)
	mustNotProduce(t, summary, filepath.Join(astroDir, slimManifestName))
	var meta Metadata
	readJSON(t, summary, filepath.Join(astroDir, sidecarName), &meta)
	if meta.FilteredManifest != nil {
		t.Fatalf("project sidecar points at a slim manifest that was never written: %+v", meta.FilteredManifest)
	}
}

// TestRunLeavesManifestUnchanged: the slim manifest is a copy for .astro/,
// never an edit of what dbt produced. Worth pinning by bytes, since
// processManifest parses the manifest and hashDocument then mutates that doc.
func TestRunLeavesManifestUnchanged(t *testing.T) {
	root := t.TempDir()
	const manifest = `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","project_name":"shop","generated_at":"2026-08-13T00:00:00Z"},` +
		`"nodes":{"model.shop.orders":{"original_file_path":"models/orders.sql","package_name":"shop","resource_type":"model","fqn":["shop","orders"],"created_at":1754300000.1}},` +
		`"selectors":{"my_selector":{"definition":{},"created_at":1754300000.1}}}`
	writeFiles(t, root, map[string]string{"shipped/manifest.json": manifest})
	manifestPath := filepath.Join(root, "shipped", "manifest.json")

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	// Both artifacts must exist, or this would pass trivially.
	mustProduce(t, summary, filepath.Join(root, "shipped", sidecarDir, sidecarName))
	mustProduce(t, summary, filepath.Join(root, "shipped", sidecarDir, slimManifestName))

	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != manifest {
		t.Fatalf("manifest.json was modified:\n before %s\n after  %s", manifest, after)
	}
}

// TestRunSlimManifestUnaffectedByHashDocumentMutation pins a review-flagged
// risk: buildSlimManifest's result shares doc's "selectors" map by reference
// rather than copying it, so if the slim manifest were marshaled to disk
// after hashDocument runs — which strips created_at from every top-level
// collection's entries, selectors included — the written file would lose
// data outside the allowlist that was never supposed to be touched.
// processManifest now marshals the slim manifest before calling
// hashDocument, so a selector's created_at must still reach the file that
// ships.
func TestRunSlimManifestUnaffectedByHashDocumentMutation(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"shipped/manifest.json": `{
			"metadata": {"dbt_schema_version": "https://schemas.getdbt.com/dbt/manifest/v12.json"},
			"nodes": {},
			"selectors": {"my_selector": {"created_at": 123, "definition": {"method": "tag", "value": "daily"}}}
		}`,
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil || len(summary.Results) != 1 || summary.Results[0].Err != nil {
		t.Fatalf("Run failed: err=%v results=%+v", err, summary.Results)
	}

	var slim map[string]any
	readJSON(t, summary, filepath.Join(root, "shipped", sidecarDir, slimManifestName), &slim)
	selector, ok := slim["selectors"].(map[string]any)["my_selector"].(map[string]any)
	if !ok {
		t.Fatalf("slim manifest missing expected selector: %+v", slim)
	}
	if _, ok := selector["created_at"]; !ok {
		t.Fatalf("selector lost created_at: hashDocument's mutation of doc leaked into the already-marshaled slim manifest, got %+v", selector)
	}
}

// TestRunWritesNothing pins the contract the deploy relies on: the artifacts
// live in memory only, and the walked tree is byte-for-byte what it was.
func TestRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml":      "name: p\n",
		"proj/models/a.sql":         "select 1",
		"proj/manifest.json":        `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`,
		"proj/target/manifest.json": `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`,
	})
	before := treeSnapshot(t, root)

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(summary.Artifacts()); got != 4 {
		t.Fatalf("artifacts = %d, want 4 (two sidecars, two slim manifests)", got)
	}
	if after := treeSnapshot(t, root); !maps.Equal(before, after) {
		t.Fatalf("Run changed the tree:\n before %v\n after  %v", before, after)
	}
}

// TestRunFailsUnitWhenAstroIsAFile: a file named .astro where the artifacts
// belong would make the bundle unpackable, so the unit fails and produces
// nothing rather than shipping a path collision.
func TestRunFailsUnitWhenAstroIsAFile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml": "name: p\n",
		"proj/.astro":          "x",
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.CountFailed() != 1 || len(summary.Artifacts()) != 0 {
		t.Fatalf("want one failed unit and no artifacts, got %+v", summary.Results)
	}
}

// TestRunFailsUnitWhenArtifactPathIsNotAFile: a directory where a sidecar
// goes would collide with it in the bundle, so that unit fails and produces
// nothing, while a regular file there (an old sidecar) is fine.
func TestRunFailsUnitWhenArtifactPathIsNotAFile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"blocked/dbt_project.yml":                     "name: blocked\n",
		"blocked/.astro/dbt_metadata.json/inside.txt": "x",
		"fine/dbt_project.yml":                        "name: fine\n",
		"fine/.astro/dbt_metadata.json":               "old",
	})

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.CountFailed() != 1 {
		t.Fatalf("want one failed unit, got %+v", summary.Results)
	}
	mustNotProduce(t, summary, filepath.Join(root, "blocked", sidecarDir, sidecarName))
	mustProduce(t, summary, filepath.Join(root, "fine", sidecarDir, sidecarName))
}

// TestRunReplacesALinkAtAnArtifactPath: a link where a sidecar goes is left
// out of the bundle, so the fresh sidecar takes its place and the unit
// succeeds.
func TestRunReplacesALinkAtAnArtifactPath(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"proj/dbt_project.yml": "name: p\n", "elsewhere.json": "{}"})
	if err := os.MkdirAll(filepath.Join(root, "proj", sidecarDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "elsewhere.json"), filepath.Join(root, "proj", sidecarDir, sidecarName)); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	summary, err := Run([]string{root}, "test", Options{SlimManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.CountFailed() != 0 {
		t.Fatalf("want no failures, got %+v", summary.Results)
	}
	mustProduce(t, summary, filepath.Join(root, "proj", sidecarDir, sidecarName))
}

// TestUnitsSidecarDirs: one .astro per unit, beside a project or a manifest,
// known before any unit is read.
func TestUnitsSidecarDirs(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"proj/dbt_project.yml":      "name: p\n",
		"proj/target/manifest.json": "not even json",
		"shipped/manifest.json":     "{}",
	})
	units, err := Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "proj", sidecarDir),
		filepath.Join(root, "proj", "target", sidecarDir),
		filepath.Join(root, "shipped", sidecarDir),
	}
	if got := units.SidecarDirs(); !slices.Equal(got, want) {
		t.Fatalf("SidecarDirs = %v, want %v", got, want)
	}
}

// treeSnapshot maps every file under root to its contents.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		out[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
