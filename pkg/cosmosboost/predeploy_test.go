package cosmosboost

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/cosmosboost/precompute"
	"github.com/astronomer/astro-cli/version"
)

const (
	sidecarRel  = ".astro/dbt_metadata.json"
	oursSidecar = `{"schema":1,"version":{"algo":"sha256-tree-v2","hash":"stale"},"generated_by":{"application":"astro","version":"1.0.0"}}`
	foreignFile = `{"schema":1,"version":{"hash":"abc"},"generated_by":{"application":"someone-else"}}`
	dbtManifest = `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

func writeDbtProject(t *testing.T, dir string) {
	t.Helper()
	writeFiles(t, dir, map[string]string{"dbt_project.yml": "name: shop\n", "models/a.sql": "select 1"})
}

// snapshot maps every file under root to its contents, to prove a call left
// the tree exactly as it found it.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		out[path] = string(data)
		return err
	}))
	return out
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.CurrVersion
	version.CurrVersion = v
	t.Cleanup(func() { version.CurrVersion = orig })
}

// TestBundleOverlayEnabledAddsSidecars pins the plugin contract: the same
// files at the same paths with the same contents earlier versions wrote into
// the project, now only in the bundle - the tree is untouched.
func TestBundleOverlayEnabledAddsSidecars(t *testing.T) {
	setVersion(t, "1.2.3")
	dir := t.TempDir()
	writeDbtProject(t, dir)
	writeFiles(t, dir, map[string]string{"target/manifest.json": dbtManifest})
	before := snapshot(t, dir)

	overlay, err := BundleOverlay(dir, true)
	require.NoError(t, err)

	require.Equal(t, before, snapshot(t, dir), "the tree must not be written")
	require.Empty(t, overlay.Skip)
	require.ElementsMatch(t, []string{sidecarRel, "target/" + sidecarRel, "target/.astro/manifest.slim.json"}, keys(overlay.Extra))

	// The contents are exactly what the pre-deploy step produces for the tree.
	summary, err := precompute.Run([]string{dir}, "1.2.3", precompute.Options{SlimManifest: true})
	require.NoError(t, err)
	for _, a := range summary.Artifacts() {
		rel, err := filepath.Rel(dir, a.Path)
		require.NoError(t, err)
		require.Equal(t, string(a.Data), string(overlay.Extra[filepath.ToSlash(rel)]), rel)
	}

	var meta struct {
		Schema  int `json:"schema"`
		Version struct {
			Algo string `json:"algo"`
			Hash string `json:"hash"`
		} `json:"version"`
		GeneratedBy struct {
			Application string `json:"application"`
			Version     string `json:"version"`
		} `json:"generated_by"`
	}
	require.NoError(t, json.Unmarshal(overlay.Extra[sidecarRel], &meta))
	require.Equal(t, 1, meta.Schema, "schema is the plugin's compatibility gate and must stay 1")
	require.Equal(t, "sha256-tree-v2", meta.Version.Algo)
	for _, r := range summary.Results {
		if r.Path == dir {
			require.Equal(t, r.Hash, meta.Version.Hash, "version.hash is what the plugin consumes")
		}
	}
	require.Equal(t, "astro", meta.GeneratedBy.Application)
	require.Equal(t, "1.2.3", meta.GeneratedBy.Version)
}

// TestBundleOverlayRespectsSlimManifestEnvVar: the env var disables this one
// optimization without disabling the step, so the hash sidecar still ships.
func TestBundleOverlayRespectsSlimManifestEnvVar(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": dbtManifest})
	t.Setenv(slimManifestEnvVar, "false")

	overlay, err := BundleOverlay(dir, true)
	require.NoError(t, err)
	require.Equal(t, []string{sidecarRel}, keys(overlay.Extra))
}

// TestSlimManifestEnabled: unset leaves the optimization on; otherwise only a
// value util.CheckEnvBool reads as true keeps it on.
func TestSlimManifestEnabled(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{value: "", want: true}, // unset: on by default under the master switch
		{value: "true", want: true},
		{value: "TRUE", want: true}, // case-insensitive
		{value: "1", want: true},
		{value: "yes", want: true},
		{value: "false"},
		{value: "0"},
		{value: "off"},
	} {
		t.Setenv(slimManifestEnvVar, tc.value)
		require.Equal(t, tc.want, slimManifestEnabled(), "value %q", tc.value)
	}
}

// TestBundleOverlayLeavesOutStaleSidecars: whether the step is on or off, a
// sidecar an earlier astro-cli wrote into the project never ships - including
// one whose project no longer exists - and the tree keeps it.
func TestBundleOverlayLeavesOutStaleSidecars(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		dir := t.TempDir()
		writeDbtProject(t, dir)
		writeFiles(t, dir, map[string]string{
			sidecarRel:                                  oursSidecar,
			"removed-project/" + sidecarRel:             oursSidecar,
			"removed-project/.astro/manifest.slim.json": `{"_generated_by":{"application":"astro"}}`,
		})
		before := snapshot(t, dir)

		overlay, err := BundleOverlay(dir, enabled)
		require.NoError(t, err)

		require.Equal(t, before, snapshot(t, dir), "enabled=%v: the tree must not be written", enabled)
		require.ElementsMatch(t, []string{sidecarRel, "removed-project/" + sidecarRel, "removed-project/.astro/manifest.slim.json"}, keys(overlay.Skip), "enabled=%v", enabled)
		if enabled {
			require.Equal(t, []string{sidecarRel}, keys(overlay.Extra), "the current project gets a fresh sidecar")
			require.NotContains(t, string(overlay.Extra[sidecarRel]), "stale")
		} else {
			require.Empty(t, overlay.Extra, "a disabled deploy adds nothing")
		}
	}
}

// TestBundleOverlayForeignSidecar: a file from an unrecognized producer is
// the user's. Disabled, it ships untouched; enabled, the deploy stops and asks
// for its removal, because the plugin would consume its version.hash.
func TestBundleOverlayForeignSidecar(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	writeFiles(t, dir, map[string]string{"other/" + sidecarRel: foreignFile})
	foreign := filepath.Join(dir, "other", ".astro", "dbt_metadata.json")

	overlay, err := BundleOverlay(dir, false)
	require.NoError(t, err)
	require.Empty(t, overlay.Skip, "disabled: a foreign file ships")
	require.Empty(t, overlay.Extra)

	_, err = BundleOverlay(dir, true)
	require.Error(t, err)
	require.ErrorContains(t, err, "unrecognized producer")
	require.ErrorContains(t, err, foreign)
	require.ErrorContains(t, err, "Remove them and retry")
	require.FileExists(t, foreign, "a file another tool wrote must never be deleted")
}

func TestBundleOverlayNoDbtContentIsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"app.py": "print('hi')"})

	overlay, err := BundleOverlay(dir, true)
	require.NoError(t, err)
	require.Empty(t, overlay.Skip)
	require.Empty(t, overlay.Extra)
}

// TestBundleOverlayMissingRoot: a root that is not there cannot be checked
// for stale files, so the deploy fails rather than guessing.
func TestBundleOverlayMissingRoot(t *testing.T) {
	_, err := BundleOverlay(filepath.Join(t.TempDir(), "does-not-exist"), false)
	require.Error(t, err)
}

// TestBundleOverlayStepFailureIsAWarning: stamping is best-effort - a unit
// that cannot be hashed ships no sidecar, the others still do, and the
// deploy goes on.
func TestBundleOverlayStepFailureIsAWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeDbtProject(t, filepath.Join(dir, "broken"))
	writeDbtProject(t, filepath.Join(dir, "fine"))
	model := filepath.Join(dir, "broken", "models", "a.sql")
	require.NoError(t, os.Chmod(model, 0o000))
	t.Cleanup(func() { _ = os.Chmod(model, 0o644) })

	overlay, err := BundleOverlay(dir, true)
	require.NoError(t, err)
	require.Equal(t, []string{"fine/" + sidecarRel}, keys(overlay.Extra))

	_, err = preDeploy(dir)
	require.ErrorContains(t, err, "failed for 1 unit(s)")
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
