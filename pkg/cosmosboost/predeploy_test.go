package cosmosboost

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/cosmosboost/precompute"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/version"
)

const (
	sidecarRel  = ".astro/dbt_metadata.json"
	slimRel     = ".astro/manifest.slim.json"
	oursSidecar = `{"schema":1,"version":{"algo":"sha256-tree-v2","hash":"stale"},"generated_by":{"application":"astro","version":"1.0.0"}}`
	oursSlim    = `{"_schema":1,"_generated_by":{"application":"astro","version":"1.0.0"}}`
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
// the tree exactly as it found it. A root that is not there is empty.
func snapshot(t *testing.T, root string) map[string]string {
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
	if !errors.Is(err, fs.ErrNotExist) {
		require.NoError(t, err)
	}
	return out
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.CurrVersion
	version.CurrVersion = v
	t.Cleanup(func() { version.CurrVersion = orig })
}

// bundle tars dir as a deploy does, through BundleOverlay, and returns the
// tarball's entries by name, or the error the deploy would stop on.
func bundle(t *testing.T, dir string, enabled bool) (map[string]string, error) {
	t.Helper()
	overlay, err := BundleOverlay(dir, enabled)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := fileutil.TarGz(dir, target, false, []string{".git/"}, overlay); err != nil {
		return nil, err
	}
	file, err := os.Open(target)
	require.NoError(t, err)
	defer file.Close()
	gz, err := gzip.NewReader(file)
	require.NoError(t, err)
	entries := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		entries[header.Name] = string(data)
	}
	return entries, nil
}

// TestBundleEnabledAddsSidecars pins the plugin contract: the same files at
// the same paths with the same contents earlier versions wrote into the
// project, now only in the bundle - the tree is untouched.
func TestBundleEnabledAddsSidecars(t *testing.T) {
	setVersion(t, "1.2.3")
	dir := t.TempDir()
	writeDbtProject(t, dir)
	writeFiles(t, dir, map[string]string{"target/manifest.json": dbtManifest})
	before := snapshot(t, dir)

	entries, err := bundle(t, dir, true)
	require.NoError(t, err)

	require.Equal(t, before, snapshot(t, dir), "the tree must not be written")
	require.ElementsMatch(t, []string{"dbt_project.yml", "models/a.sql", "target/manifest.json", sidecarRel, "target/" + sidecarRel, "target/" + slimRel}, keys(entries))

	// The contents are exactly what the pre-deploy step produces for the tree.
	summary, err := precompute.Run([]string{dir}, "1.2.3", precompute.Options{SlimManifest: true})
	require.NoError(t, err)
	for _, a := range summary.Artifacts() {
		rel, err := filepath.Rel(dir, a.Path)
		require.NoError(t, err)
		require.Equal(t, string(a.Data), entries[filepath.ToSlash(rel)], rel)
		// What it generates is what every later deploy recognizes as its own.
		require.True(t, airflowrt.CosmosBoostFileWrittenByAstro(filepath.ToSlash(rel), a.Data), rel)
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
	require.NoError(t, json.Unmarshal([]byte(entries[sidecarRel]), &meta))
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

// TestBundleRespectsSlimManifestEnvVar: the env var disables this one
// optimization without disabling the step, so the hash sidecar still ships.
func TestBundleRespectsSlimManifestEnvVar(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": dbtManifest})
	t.Setenv(slimManifestEnvVar, "false")

	entries, err := bundle(t, dir, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"manifest.json", sidecarRel}, keys(entries))
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

// TestBundleLeavesOutStaleSidecars: whether the step is on or off, a sidecar
// an earlier CLI wrote into the project never ships - including one whose
// project no longer exists, and one under logs/, which discovery never visits
// but the tarball would carry - and the tree keeps it.
func TestBundleLeavesOutStaleSidecars(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		dir := t.TempDir()
		writeDbtProject(t, dir)
		writeFiles(t, dir, map[string]string{
			sidecarRel:                      oursSidecar,
			"removed-project/" + sidecarRel: oursSidecar,
			"removed-project/" + slimRel:    oursSlim,
			"logs/" + sidecarRel:            oursSidecar,
		})
		before := snapshot(t, dir)

		entries, err := bundle(t, dir, enabled)
		require.NoError(t, err)

		require.Equal(t, before, snapshot(t, dir), "enabled=%v: the tree must not be written", enabled)
		for _, gone := range []string{"removed-project/" + sidecarRel, "removed-project/" + slimRel, "logs/" + sidecarRel} {
			require.NotContains(t, entries, gone, "enabled=%v", enabled)
		}
		if enabled {
			require.Contains(t, entries, sidecarRel, "the current project gets a fresh sidecar")
			require.NotContains(t, entries[sidecarRel], "stale")
		} else {
			require.NotContains(t, entries, sidecarRel, "a disabled deploy ships no sidecar, stale or fresh")
		}
	}
}

// TestBundleForeignSidecar: a file from an unrecognized producer is the
// user's. Disabled, it ships untouched. Enabled, where the step puts a fresh
// sidecar, the deploy stops and asks for its removal, before saying the step
// is complete: the plugin reads that path, and the replacement would hide it.
func TestBundleForeignSidecar(t *testing.T) {
	for _, rel := range []string{sidecarRel, "target/" + sidecarRel, "target/" + slimRel} {
		dir := t.TempDir()
		writeDbtProject(t, dir)
		writeFiles(t, dir, map[string]string{"target/manifest.json": dbtManifest, rel: foreignFile})
		foreign := filepath.Join(dir, filepath.FromSlash(rel))

		entries, err := bundle(t, dir, false)
		require.NoError(t, err)
		require.Equal(t, foreignFile, entries[rel], "disabled: a foreign file ships")

		out := captureStdout(t, func() { _, err = bundle(t, dir, true) })
		require.Error(t, err, rel)
		require.ErrorContains(t, err, "unrecognized producer")
		require.ErrorContains(t, err, foreign)
		require.ErrorContains(t, err, "Remove them and retry")
		require.NotContains(t, out, "pre-deploy step complete", "the refusal comes before the step says it is done")
		require.FileExists(t, foreign, "a file another tool wrote must never be deleted")
	}
}

// TestBundleShipsForeignSidecarsTheStepDoesNotReplace: where the step writes
// nothing - an installed package's own sidecar under dbt_packages/, logs/,
// beside a manifest.json that is not dbt's, a slim manifest with the slim
// optimization off - another producer's file is the project's, and an enabled
// deploy ships it.
func TestBundleShipsForeignSidecarsTheStepDoesNotReplace(t *testing.T) {
	t.Setenv(slimManifestEnvVar, "false")
	dir := t.TempDir()
	writeDbtProject(t, dir)
	shipped := map[string]string{
		"dbt_packages/utils/" + sidecarRel: foreignFile,
		"logs/" + sidecarRel:               foreignFile,
		"webapp/" + sidecarRel:             foreignFile,
		slimRel:                            foreignFile,
	}
	writeFiles(t, dir, shipped)
	writeFiles(t, dir, map[string]string{
		"dbt_packages/utils/dbt_project.yml": "name: utils\n",
		"webapp/manifest.json":               `{"name":"My App"}`,
	})

	entries, err := bundle(t, dir, true)
	require.NoError(t, err)
	for rel, content := range shipped {
		require.Equal(t, content, entries[rel], rel)
	}
	require.Contains(t, entries, sidecarRel, "the project itself is stamped")
}

// TestBundleLinksAndDirectoriesAtSidecarPaths: disabled, a link at a
// sidecar's path is the user's and ships as it is. Enabled, a link or a
// directory where the step puts a sidecar is left out and the fresh sidecar
// takes its place.
func TestBundleLinksAndDirectoriesAtSidecarPaths(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		dir := t.TempDir()
		writeDbtProject(t, filepath.Join(dir, "linked"))
		writeDbtProject(t, filepath.Join(dir, "blocked"))
		writeFiles(t, dir, map[string]string{
			"target.json":                  oursSidecar,
			"blocked/" + sidecarRel + "/x": "x",
		})
		link := filepath.Join(dir, "linked", ".astro", "dbt_metadata.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
		if err := os.Symlink(filepath.Join(dir, "target.json"), link); err != nil {
			t.Skipf("cannot create a symlink here: %v", err)
		}

		entries, err := bundle(t, dir, enabled)
		require.NoError(t, err, "enabled=%v", enabled)
		if enabled {
			require.NotContains(t, entries, "blocked/"+sidecarRel+"/x")
			for _, rel := range []string{"linked/" + sidecarRel, "blocked/" + sidecarRel} {
				require.Contains(t, entries, rel)
				require.NotContains(t, entries[rel], "stale", "the fresh sidecar replaces it")
			}
		} else {
			require.Contains(t, entries, "linked/"+sidecarRel, "a link ships as it is")
			require.Empty(t, entries["linked/"+sidecarRel], "as a link, not as what it points at")
			require.Contains(t, entries, "blocked/"+sidecarRel+"/x")
		}
	}
}

// captureStdout returns what fn prints to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	defer func() { os.Stdout = orig }()
	fn()
	os.Stdout = orig
	require.NoError(t, w.Close())
	return <-done
}

func TestBundleNoDbtContentAddsNothing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"app.py": "print('hi')"})

	entries, err := bundle(t, dir, true)
	require.NoError(t, err)
	require.Equal(t, []string{"app.py"}, keys(entries))
}

// TestBundleMissingRoot: a directory that is not there (a project with no
// dags/) makes the empty tarball it always did, with the step on or off, and
// nothing is created.
func TestBundleMissingRoot(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		dir := filepath.Join(t.TempDir(), "dags")
		entries, err := bundle(t, dir, enabled)
		require.NoError(t, err, "enabled=%v", enabled)
		require.Empty(t, entries)
		require.NoDirExists(t, dir)
	}
}

// TestBundleStepFailureIsAWarning: stamping is best-effort - a unit that
// cannot be hashed ships no sidecar, the others still do, and the deploy
// goes on.
func TestBundleStepFailureIsAWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeDbtProject(t, filepath.Join(dir, "broken"))
	writeDbtProject(t, filepath.Join(dir, "fine"))
	model := filepath.Join(dir, "broken", "models", "a.sql")
	require.NoError(t, os.Chmod(model, 0o000))
	t.Cleanup(func() { _ = os.Chmod(model, 0o644) })

	var overlay fileutil.TarOverlay
	var err error
	out := captureStdout(t, func() { overlay, err = BundleOverlay(dir, true) })
	require.NoError(t, err)
	require.Contains(t, out, "pre-deploy step failed for 1 unit(s)")
	require.Equal(t, []string{"fine/" + sidecarRel}, keys(overlay.Extra))
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
