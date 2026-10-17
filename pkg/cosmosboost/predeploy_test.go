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

// TestPreDeployWritesSlimManifest pins that PreDeploy writes a slim manifest
// next to a discovered manifest.json's hash sidecar. It is on by default: the
// only switch is the step itself (cosmos_boost.pre_deploy), with a per-feature
// env var below.
func TestPreDeployWritesSlimManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))

	require.NoError(t, PreDeploy(dir))

	require.FileExists(t, filepath.Join(dir, ".astro", "manifest.slim.json"))
}

// TestPreDeployRespectsSlimManifestEnvVar: the env var disables this one
// optimization without disabling the step, so the hash sidecar still lands.
func TestPreDeployRespectsSlimManifestEnvVar(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))
	t.Setenv(slimManifestEnvVar, "false")

	require.NoError(t, PreDeploy(dir))

	require.FileExists(t, filepath.Join(dir, ".astro", "dbt_metadata.json"))
	require.NoFileExists(t, filepath.Join(dir, ".astro", "manifest.slim.json"))
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

// TestEnsureCleanFailsOnForeignSidecar pins the strict-ownership contract for
// deploys: cleanup never deletes a file it does not own, but the plugin would
// consume its version.hash all the same - so an enabled deploy must stop and
// ask for manual removal instead of shipping it.
func TestEnsureCleanFailsOnForeignSidecar(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))

	foreign := filepath.Join(dir, "other", ".astro", "dbt_metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0o755))
	require.NoError(t, os.WriteFile(foreign, []byte(`{"generated_by": {"application": "someone-else"}}`), 0o644))

	err := EnsureClean(dir)

	require.Error(t, err, "an unrecognized sidecar must fail the clean")
	require.ErrorContains(t, err, foreign)
	require.ErrorContains(t, err, "remove them manually")
	_, statErr := os.Stat(filepath.Join(dir, artifactRelPath))
	require.True(t, os.IsNotExist(statErr), "our artifact must still be removed")
	require.FileExists(t, foreign, "a file another tool wrote must never be deleted")
}

// TestCleanupKeepsForeignSidecarWithoutError: the explicit command keeps
// preserving unrecognized files and reports success, unlike EnsureClean.
func TestCleanupKeepsForeignSidecarWithoutError(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "other", ".astro", "dbt_metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0o755))
	require.NoError(t, os.WriteFile(foreign, []byte(`{"generated_by": {"application": "someone-else"}}`), 0o644))

	require.NoError(t, Cleanup(dir))
	require.FileExists(t, foreign)
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

// TestPreDeployReportsFailedUnits: a unit that cannot be hashed turns into an
// error naming the failure count, so the hook can warn with substance.
func TestPreDeployReportsFailedUnits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission semantics differ on windows")
	}
	dir := t.TempDir()
	writeDbtProject(t, dir)
	model := filepath.Join(dir, "models", "a.sql")
	require.NoError(t, os.Chmod(model, 0o000))
	t.Cleanup(func() { _ = os.Chmod(model, 0o644) })

	err := PreDeploy(dir)

	require.Error(t, err)
	require.ErrorContains(t, err, "failed for 1 unit(s)")
}

// TestBestEffortPreDeployWarnsAndContinues: stamping failures never propagate.
func TestBestEffortPreDeployWarnsAndContinues(t *testing.T) {
	BestEffortPreDeploy(filepath.Join(t.TempDir(), "does-not-exist"))
}
