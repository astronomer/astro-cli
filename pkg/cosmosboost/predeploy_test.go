package cosmosboost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/version"
)

const artifactRelPath = ".astro/dbt_metadata.json"

func writeDbtProject(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "models"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models", "a.sql"), []byte("select 1"), 0o644))
}

func TestPreDeployWritesArtifact(t *testing.T) {
	origVersion := version.CurrVersion
	version.CurrVersion = "1.2.3"
	t.Cleanup(func() { version.CurrVersion = origVersion })

	dir := t.TempDir()
	writeDbtProject(t, dir)

	require.NoError(t, PreDeploy(dir))

	data, err := os.ReadFile(filepath.Join(dir, artifactRelPath))
	require.NoError(t, err)
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
	require.NoError(t, json.Unmarshal(data, &meta))
	require.Equal(t, 1, meta.Schema, "schema is the plugin's compatibility gate and must stay 1")
	require.NotEmpty(t, meta.Version.Hash, "version.hash is what the plugin consumes")
	require.NotEmpty(t, meta.Version.Algo)
	require.Equal(t, "astro", meta.GeneratedBy.Application)
	require.Equal(t, "1.2.3", meta.GeneratedBy.Version, "provenance records the CLI version now that the step runs in-process")
}

func TestPreDeployNoDbtContentIsANoOp(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('hi')"), 0o644))

	require.NoError(t, PreDeploy(dir))

	_, err := os.Stat(filepath.Join(dir, ".astro"))
	require.True(t, os.IsNotExist(err), "nothing dbt-shaped means nothing written")
}

func TestPreDeployNonexistentPath(t *testing.T) {
	require.Error(t, PreDeploy(filepath.Join(t.TempDir(), "does-not-exist")))
}

// TestEnsureCleanRemovesStaleArtifacts covers the sequence the deploy hooks
// run: EnsureClean first, then (opt-in) BestEffortPreDeploy — so an artifact
// from an earlier deploy, even one whose project no longer exists, cannot
// survive into the payload.
func TestEnsureCleanRemovesStaleArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)

	// A stale artifact from an earlier deploy, in a spot the current tree no
	// longer produces one for (its project was deleted since).
	stale := filepath.Join(dir, "removed-project", ".astro", "dbt_metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte(`{"generated_by": {"application": "astro"}}`), 0o644))

	require.NoError(t, EnsureClean(dir))
	BestEffortPreDeploy(dir)

	_, err := os.Stat(stale)
	require.True(t, os.IsNotExist(err), "stale artifacts must not survive a pre-deploy run")
	require.FileExists(t, filepath.Join(dir, artifactRelPath), "the current project must be stamped")
}

func TestEnsureCleanRemovesOursKeepsForeign(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))

	foreign := filepath.Join(dir, "other", ".astro", "dbt_metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0o755))
	require.NoError(t, os.WriteFile(foreign, []byte(`{"generated_by": {"application": "someone-else"}}`), 0o644))

	require.NoError(t, EnsureClean(dir))

	_, err := os.Stat(filepath.Join(dir, artifactRelPath))
	require.True(t, os.IsNotExist(err), "our artifact must be removed")
	require.FileExists(t, foreign, "a file another tool wrote must never be deleted")
}

// TestEnsureCleanFailsWhenArtifactUnremovable pins the safety property behind
// the hooks: when a stale artifact's absence cannot be guaranteed, EnsureClean
// errors so the deploy stops instead of shipping it.
func TestEnsureCleanFailsWhenArtifactUnremovable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission semantics differ on windows")
	}
	dir := t.TempDir()
	astroDir := filepath.Join(dir, "proj", ".astro")
	require.NoError(t, os.MkdirAll(astroDir, 0o755))
	artifact := filepath.Join(astroDir, "dbt_metadata.json")
	require.NoError(t, os.WriteFile(artifact, []byte(`{"generated_by": {"application": "astro"}}`), 0o644))
	require.NoError(t, os.Chmod(astroDir, 0o555)) // file cannot be unlinked
	t.Cleanup(func() { _ = os.Chmod(astroDir, 0o755) })

	err := EnsureClean(dir)

	require.Error(t, err, "an artifact that cannot be removed must fail the clean")
	require.ErrorContains(t, err, "astro dbt cleanup")
	require.FileExists(t, artifact)
}
