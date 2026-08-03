package precompute

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeFiles creates files (relative path -> content) under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHashProjectDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml":  "name: shop\n",
		"models/a.sql":     "select 1",
		"models/sub/b.sql": "select 2",
	})

	h1, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	h2, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s != %s", h1, h2)
	}
}

func TestHashProjectExcludesGeneratedContent(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\n",
		"models/a.sql":    "select 1",
	})
	want, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}

	// Adding generated / installed / secret content must NOT change the hash.
	writeFiles(t, dir, map[string]string{
		"target/manifest.json":     `{"x":1}`,
		"logs/dbt.log":             "noise",
		"dbt_packages/dep/x.sql":   "select 9",
		".astro/dbt_metadata.json": `{"hash":"stale"}`,
		"package-lock.yml":         "sha: abc",
		"profiles.yml":             "secret",
	})
	got, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("excluded content changed the hash:\n want %s\n got  %s", want, got)
	}
}

// TestHashProjectExcludesGit pins that VCS metadata never affects project
// identity: .git is not deployed, so commits, fetches, and gc must not change
// the hash. Both the .git directory form and the pointer-file form (linked
// worktrees, submodules) are covered.
func TestHashProjectExcludesGit(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\n",
		"models/a.sql":    "select 1",
	})
	before := mustHashProject(t, dir)

	writeFiles(t, dir, map[string]string{
		".git/HEAD":              "ref: refs/heads/main\n",
		".git/objects/ab/cdef01": "packed",
		"vendored/.git":          "gitdir: ../../.git/modules/vendored\n",
	})
	if got := mustHashProject(t, dir); got != before {
		t.Fatalf("git metadata changed the hash:\n want %s\n got  %s", before, got)
	}
}

// mustHashProject hashes dir and fails the test on error, for tests that only
// care about the hash value.
func mustHashProject(t *testing.T, dir string) string {
	t.Helper()
	hash, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatalf("hashProject: %v", err)
	}
	return hash
}

func TestHashProjectSensitiveToModelChange(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\n",
		"models/a.sql":    "select 1",
	})
	before := mustHashProject(t, dir)

	writeFiles(t, dir, map[string]string{"models/a.sql": "select 2"})
	after := mustHashProject(t, dir)

	if before == after {
		t.Fatal("hash did not change after editing a model")
	}
}

// TestHashProjectHonorsPackagesInstallPath verifies the dbt `packages-install-path`
// override is excluded from the hash, not just the default dbt_packages/.
func TestHashProjectHonorsPackagesInstallPath(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\npackages-install-path: my_packages\n",
		"models/a.sql":    "select 1",
	})
	want, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}

	// Installing packages into the custom dir must NOT change the hash.
	writeFiles(t, dir, map[string]string{"my_packages/dep/x.sql": "select 999"})
	got, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("custom packages-install-path content changed the hash:\n want %s\n got  %s", want, got)
	}
}

// TestHashProjectGolden pins the exact algorithm output for a fixed input, so the
// Cosmos Boost plugin's read-side can reproduce the same value. If this constant
// has to change, the algorithm changed and `algoProjectTree` must be bumped.
func TestHashProjectGolden(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: jaffle\n",
		"models/x.sql":    "select 1\n",
	})

	got, files, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if files != 2 {
		t.Fatalf("want 2 files hashed, got %d", files)
	}

	const want = "f8c7f8d2f8060e63e6ecbe5d2e817cdbb5c50bd4f020da28a6a1305f96be5d10"
	if got != want {
		t.Fatalf("golden hash mismatch (update the constant only if the algorithm intentionally changed):\n want %s\n got  %s", want, got)
	}
}

// TestHashManifestIgnoresVolatileMetadata verifies a recompile that only changes
// volatile metadata (generated_at, invocation_id) does NOT change the hash, while
// a real node change does.
func TestHashManifestIgnoresVolatileMetadata(t *testing.T) {
	dir := t.TempDir()
	const base = `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json","dbt_version":"1.8.0","generated_at":%q,"invocation_id":%q},"nodes":{"model.x":{"name":%q}}}`

	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	hash := func(path string) string {
		h, _, isDbt, err := hashManifest(path)
		if err != nil {
			t.Fatal(err)
		}
		if !isDbt {
			t.Fatalf("expected %s to be recognized as a dbt manifest", path)
		}
		return h
	}

	a := hash(write("a.json", fmt.Sprintf(base, "2026-01-01T00:00:00Z", "aaaa", "x")))
	// Same source, different volatile metadata only:
	b := hash(write("b.json", fmt.Sprintf(base, "2026-06-30T12:34:56Z", "zzzz", "x")))
	if a != b {
		t.Fatalf("manifest hash changed when only volatile metadata differed:\n %s\n %s", a, b)
	}
	// A real content change (a node) must change the hash:
	c := hash(write("c.json", fmt.Sprintf(base, "2026-01-01T00:00:00Z", "aaaa", "y")))
	if a == c {
		t.Fatal("manifest hash did not change when a node changed")
	}
}

// TestHashManifestSkipsNonDBT verifies that a manifest.json lacking the dbt shape
// (e.g. a web-app/PWA manifest) or invalid JSON is not treated as a dbt manifest,
// so it won't be stamped.
func TestHashManifestSkipsNonDBT(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"webapp.json":  `{"name":"My App","short_name":"App","icons":[]}`, // no metadata.dbt_schema_version
		"nometa.json":  `{"nodes":{"model.x":{}}}`,                        // nodes but no metadata
		"invalid.json": `{not json`,                                       // not JSON at all
	}
	for name, content := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, isDbt, err := hashManifest(p)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if isDbt {
			t.Fatalf("%s: should not be treated as a dbt manifest", name)
		}
	}
}

// TestHashProjectPackagesPathIsPathSpecific verifies the packages-install-path
// override is excluded by its exact project-root-relative path, not by directory
// basename — so a same-named source directory elsewhere is still hashed.
func TestHashProjectPackagesPathIsPathSpecific(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml":         "name: shop\npackages-install-path: vendor/packages\n",
		"vendor/packages/dep.sql": "select 9", // installed deps — excluded
		"models/packages/m.sql":   "select 1", // real source, same basename "packages" — must be hashed
	})
	base, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}

	// Changing the installed-packages dir must NOT change the hash.
	writeFiles(t, dir, map[string]string{"vendor/packages/dep.sql": "select 99"})
	if got, _, _, _ := hashProject(dir, readDbtConfig(dir)); got != base {
		t.Fatal("content under the packages-install-path changed the hash")
	}

	// Changing the same-basename source dir MUST change the hash (no over-exclusion).
	writeFiles(t, dir, map[string]string{"models/packages/m.sql": "select 2"})
	if got, _, _, _ := hashProject(dir, readDbtConfig(dir)); got == base {
		t.Fatal("models/packages/ was wrongly excluded by basename")
	}
}

// TestReadDbtConfig covers the dbt_project.yml settings the hasher relies on: an
// unset key yields "" (the caller applies the dbt default), a plain packages/target/
// log path is used verbatim, a Jinja-templated packages-install-path is reported as
// unset + templated (NOT rendered), a path escaping the project root is dropped, and
// a parse error degrades to the zero value.
func TestReadDbtConfig(t *testing.T) {
	cases := []struct {
		name          string
		yml           string
		wantPackages  string
		wantTarget    string
		wantLog       string
		wantTemplated bool
	}{
		{"unset", "name: shop\n", "", "", "", false},
		{"plain packages", "name: shop\npackages-install-path: vendor/packages\n", "vendor/packages", "", "", false},
		{"custom target and log", "name: shop\ntarget-path: build\nlog-path: mylogs\n", "", "build", "mylogs", false},
		{"templated packages", "name: shop\npackages-install-path: \"{{ env_var('DBT_PKG_DIR') }}\"\n", "", "", "", true},
		{"escaping path dropped", "name: shop\ntarget-path: ../outside\n", "", "", "", false},
		{"invalid yaml", "- not\n- a mapping\n", "", "", "", false}, // best-effort: parse error → zero value
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"dbt_project.yml": tc.yml})
			cfg := readDbtConfig(dir)
			if cfg.packagesInstallPath != tc.wantPackages || cfg.targetPath != tc.wantTarget ||
				cfg.logPath != tc.wantLog || cfg.templatedPackages != tc.wantTemplated {
				t.Fatalf("readDbtConfig = %+v, want packages=%q target=%q log=%q templated=%v",
					cfg, tc.wantPackages, tc.wantTarget, tc.wantLog, tc.wantTemplated)
			}
		})
	}
}

// TestHashProjectExcludesCustomTargetAndLogPaths verifies a custom target-path /
// log-path from dbt_project.yml is excluded from the hash, so generated artifacts
// under a renamed output dir don't churn the version key on every compile.
func TestHashProjectExcludesCustomTargetAndLogPaths(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\ntarget-path: build\nlog-path: mylogs\n",
		"models/a.sql":    "select 1",
	})
	want, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}

	// Generated artifacts under the custom target/log dirs must NOT change the hash.
	writeFiles(t, dir, map[string]string{
		"build/manifest.json": `{"x":1}`,
		"mylogs/dbt.log":      "noise",
	})
	got, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("custom target-path/log-path content changed the hash:\n want %s\n got  %s", want, got)
	}
}

// TestHashProjectTemplatedPackagesPathNotExcluded documents the cache-churn
// trade-off: a templated packages-install-path is not resolved, so content under
// the real packages dir is NOT excluded and does change the project hash. (We
// can't know the real dir name without a Jinja engine; see packagesInstallPath.)
func TestHashProjectTemplatedPackagesPathNotExcluded(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml": "name: shop\npackages-install-path: \"{{ env_var('DBT_PKG_DIR') }}\"\n",
		"models/a.sql":    "select 1",
	})
	base, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}

	// Installing packages into the (unknown, templated) dir changes the hash,
	// because we couldn't exclude it.
	writeFiles(t, dir, map[string]string{"custom_pkgs/dep.sql": "select 9"})
	if got, _, _, _ := hashProject(dir, readDbtConfig(dir)); got == base {
		t.Fatal("expected templated packages dir to be included in the hash (cache churn), but it was excluded")
	}
}

// TestHashProjectIgnoresNestedSidecarDir pins the stability fix: an .astro sidecar
// directory anywhere in the tree (not just at the project root) must be excluded, so
// writing a sidecar next to a nested manifest (docs/.astro/) never changes the
// project hash. This is the deterministic complement to the concurrent Run-level
// TestRunProjectHashStableWithNestedManifestSidecar.
func TestHashProjectIgnoresNestedSidecarDir(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dbt_project.yml":    "name: shop\n",
		"models/a.sql":       "select 1",
		"docs/manifest.json": `{"x":1}`,
	})
	want, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}

	// A sidecar written into a nested directory must NOT change the hash.
	writeFiles(t, dir, map[string]string{
		"docs/.astro/dbt_metadata.json": `{"hash":"nested"}`,
		".astro/dbt_metadata.json":      `{"hash":"root"}`,
	})
	got, _, _, err := hashProject(dir, readDbtConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("a nested .astro sidecar changed the project hash:\n want %s\n got  %s", want, got)
	}
}

func TestHashFileError(t *testing.T) {
	if _, _, err := hashFile(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("hashFile on a missing file should return an error")
	}
}

func TestHashManifestReadError(t *testing.T) {
	_, _, isDbt, err := hashManifest(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("hashManifest on a missing file should return an error")
	}
	if isDbt {
		t.Fatal("a missing file should not be reported as a dbt manifest")
	}
}
