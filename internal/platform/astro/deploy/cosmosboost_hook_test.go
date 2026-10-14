package deploy

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// cosmosBoostArtifact is the sidecar the pre-deploy step generates for a dbt
// project, at the path the Cosmos Boost plugin reads it from in the bundle.
const cosmosBoostArtifact = ".astro/dbt_metadata.json"

const (
	staleOursSidecar = `{"schema":1,"version":{"algo":"sha256-tree-v2","hash":"stale"},"generated_by":{"application":"astro","version":"1.0.0"}}`
	foreignSidecar   = `{"schema":1,"version":{"hash":"abc"},"generated_by":{"application":"someone-else"}}`
)

// setupCosmosBoostEnv isolates ~/.astro for the test.
func setupCosmosBoostEnv(t *testing.T) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	origHome := config.HomeConfigPath
	config.HomeConfigPath = t.TempDir()
	t.Cleanup(func() { config.HomeConfigPath = origHome })
}

func enableCosmosBoost(t *testing.T) {
	t.Helper()
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))
}

func writeDbtBundle(t *testing.T) string {
	t.Helper()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))
	return bundleDir
}

func writeTreeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// captureUploads points azureUploader at a recorder and returns the entries of
// the last tarball uploaded, by name.
func captureUploads(t *testing.T) func() map[string]string {
	t.Helper()
	var entries map[string]string
	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		entries = map[string]string{}
		gz, err := gzip.NewReader(file)
		require.NoError(t, err)
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
		return "version-id", nil
	}
	return func() map[string]string { return entries }
}

// treeSnapshot maps every file under root to its contents.
func treeSnapshot(t *testing.T, root string) map[string]string {
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

// TestUploadBundleGeneratesSidecarsIntoTheBundle pins the change this step
// exists for: with the setting on, the sidecar ships in the tarball at the
// path the plugin reads, and the project tree is left exactly as it was.
func TestUploadBundleGeneratesSidecarsIntoTheBundle(t *testing.T) {
	setupCosmosBoostEnv(t)
	enableCosmosBoost(t)
	bundleDir := writeDbtBundle(t)
	uploaded := captureUploads(t)
	before := treeSnapshot(t, bundleDir)

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.Equal(t, before, treeSnapshot(t, bundleDir), "the deploy must not write into the project")
	sidecar, ok := uploaded()[cosmosBoostArtifact]
	require.True(t, ok, "the sidecar must ship in the bundle; got %v", uploaded())
	var meta struct {
		Version struct {
			Hash string `json:"hash"`
		} `json:"version"`
		GeneratedBy struct {
			Application string `json:"application"`
		} `json:"generated_by"`
	}
	require.NoError(t, json.Unmarshal([]byte(sidecar), &meta))
	assert.Len(t, meta.Version.Hash, 64, "a sha256 version.hash")
	assert.Equal(t, "astro", meta.GeneratedBy.Application)
}

// TestUploadBundleSidecarPathsUnderTheDagsBaseDir: a DAG deploy prepends the
// directory's own name, and the sidecars sit under it like every other file.
func TestUploadBundleSidecarPathsUnderTheDagsBaseDir(t *testing.T) {
	setupCosmosBoostEnv(t)
	enableCosmosBoost(t)
	dagsDir := filepath.Join(t.TempDir(), "dags")
	writeTreeFile(t, dagsDir, "dbt/shop/dbt_project.yml", "name: shop\n")
	uploaded := captureUploads(t)

	_, err := UploadBundle(t.TempDir(), dagsDir, "http://upload-url", true, "0.0.0")
	require.NoError(t, err)

	assert.Contains(t, uploaded(), "dags/dbt/shop/"+cosmosBoostArtifact)
	assert.Contains(t, uploaded(), "dags/dbt/shop/dbt_project.yml")
}

// TestUploadBundleSlimManifest pins the slim manifest's gating end to end: on
// by default once the step is enabled, and off when the per-feature env var
// says so. Either way the hash sidecar still ships - the two optimizations are
// independent.
func TestUploadBundleSlimManifest(t *testing.T) {
	for _, tc := range []struct {
		name     string
		envVar   string // value for the per-feature env var; "" leaves it unset
		wantSlim bool
	}{
		{name: "on by default", wantSlim: true},
		{name: "env var switches it off", envVar: "false", wantSlim: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupCosmosBoostEnv(t)
			enableCosmosBoost(t)
			if tc.envVar != "" {
				t.Setenv("ASTRO_COSMOS_BOOST_SLIM_MANIFEST_ENABLED", tc.envVar)
			}

			bundleDir := writeDbtBundle(t)
			writeTreeFile(t, bundleDir, "target/manifest.json", `{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{}}`)
			uploaded := captureUploads(t)

			_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
			require.NoError(t, err)

			assert.Contains(t, uploaded(), "target/.astro/dbt_metadata.json", "the manifest must still be stamped")
			if tc.wantSlim {
				assert.Contains(t, uploaded(), "target/.astro/manifest.slim.json")
			} else {
				assert.NotContains(t, uploaded(), "target/.astro/manifest.slim.json")
			}
			assert.NoDirExists(t, filepath.Join(bundleDir, "target", ".astro"), "nothing is written into the project")
		})
	}
}

func TestUploadBundleSkipsPreDeployByDefault(t *testing.T) {
	setupCosmosBoostEnv(t)
	// cosmos_boost.pre_deploy defaults to false, so nothing may be produced.
	bundleDir := writeDbtBundle(t)
	uploaded := captureUploads(t)

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.NotContains(t, uploaded(), cosmosBoostArtifact, "opt-in gate is off by default")
	assert.NoDirExists(t, filepath.Join(bundleDir, ".astro"))
}

// TestDeployBundleDbtPathGeneratesSidecarsWhenEnabled drives the full
// `astro dbt deploy` bundle path (DeployBundle into UploadBundle).
func TestDeployBundleDbtPathGeneratesSidecarsWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	enableCosmosBoost(t)

	canCiCdDeploy = func(token string) bool { return true }

	bundleDir := writeDbtBundle(t)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockV1Client, true, false)
	mockCreateDeploy(mockV1Client, "http://bundle-upload-url", nil)
	mockUpdateDeploy(mockV1Client, "version-id")
	uploaded := captureUploads(t)

	_, err := DeployBundle(&DeployBundleInput{
		BundlePath:    bundleDir,
		MountPath:     "dbt/shop",
		DeploymentID:  "test-deployment-id",
		BundleType:    "dbt",
		AstroV1Client: mockV1Client,
	})
	require.NoError(t, err)

	assert.Contains(t, uploaded(), cosmosBoostArtifact, "astro dbt deploy must ship the sidecar")
	assert.NoDirExists(t, filepath.Join(bundleDir, ".astro"))
}

// TestUploadBundleLeavesOutStaleSidecars: a sidecar an earlier CLI wrote into
// the project never ships, whether the step is on (a fresh one replaces it)
// or off (none ships), and the deploy leaves it in the tree.
func TestUploadBundleLeavesOutStaleSidecars(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		setupCosmosBoostEnv(t)
		if enabled {
			enableCosmosBoost(t)
		}
		bundleDir := writeDbtBundle(t)
		stale := writeTreeFile(t, bundleDir, cosmosBoostArtifact, staleOursSidecar)
		writeTreeFile(t, bundleDir, "gone/.astro/manifest.slim.json", `{"_generated_by":{"application":"astro"}}`)
		uploaded := captureUploads(t)
		before := treeSnapshot(t, bundleDir)

		_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
		require.NoError(t, err)

		assert.Equal(t, before, treeSnapshot(t, bundleDir), "enabled=%v: the tree must not change", enabled)
		assert.FileExists(t, stale)
		assert.NotContains(t, uploaded(), "gone/.astro/manifest.slim.json", "enabled=%v", enabled)
		if enabled {
			require.Contains(t, uploaded(), cosmosBoostArtifact)
			assert.NotContains(t, uploaded()[cosmosBoostArtifact], `"stale"`, "the fresh sidecar replaces the stale one")
		} else {
			assert.NotContains(t, uploaded(), cosmosBoostArtifact, "a disabled deploy ships no sidecar, stale or fresh")
		}
	}
}

// TestUploadBundleForeignSidecar: a sidecar from an unrecognized producer is
// the user's file. Disabled, it ships untouched; enabled, the deploy stops and
// asks for its removal, because the plugin would consume its version.hash.
func TestUploadBundleForeignSidecar(t *testing.T) {
	t.Run("disabled ships it", func(t *testing.T) {
		setupCosmosBoostEnv(t)
		bundleDir := writeDbtBundle(t)
		writeTreeFile(t, bundleDir, cosmosBoostArtifact, foreignSidecar)
		uploaded := captureUploads(t)

		_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
		require.NoError(t, err)
		assert.Equal(t, foreignSidecar, uploaded()[cosmosBoostArtifact])
	})
	t.Run("enabled refuses", func(t *testing.T) {
		setupCosmosBoostEnv(t)
		enableCosmosBoost(t)
		bundleDir := writeDbtBundle(t)
		foreign := writeTreeFile(t, bundleDir, cosmosBoostArtifact, foreignSidecar)
		uploaded := captureUploads(t)

		_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")

		require.Error(t, err)
		assert.ErrorContains(t, err, "unrecognized producer")
		assert.Nil(t, uploaded(), "nothing may be uploaded")
		assert.FileExists(t, foreign, "the foreign file must be preserved")
	})
}

// TestUploadBundleMissingDirIsUnchanged: a DAG deploy of a project with no
// dags/ uploads the empty tarball it always did, with Cosmos Boost on or off:
// the step never fails a deploy that had nothing for it.
func TestUploadBundleMissingDirIsUnchanged(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		setupCosmosBoostEnv(t)
		if enabled {
			enableCosmosBoost(t)
		}
		dagsDir := filepath.Join(t.TempDir(), "dags")
		uploaded := captureUploads(t)

		_, err := UploadBundle(t.TempDir(), dagsDir, "http://upload-url", true, "0.0.0")
		require.NoError(t, err, "enabled=%v", enabled)
		assert.Empty(t, uploaded(), "enabled=%v", enabled)
		assert.NoDirExists(t, dagsDir)
	}
}

// TestUploadBundleFromTheWorkingDirectory: astro dbt deploy --project-path .
// bundles ".", and an old sidecar there is still replaced by the fresh one,
// at the same path, with no second copy under another name.
func TestUploadBundleFromTheWorkingDirectory(t *testing.T) {
	setupCosmosBoostEnv(t)
	enableCosmosBoost(t)
	bundleDir := writeDbtBundle(t)
	writeTreeFile(t, bundleDir, cosmosBoostArtifact, staleOursSidecar)
	t.Chdir(bundleDir)
	uploaded := captureUploads(t)

	_, err := UploadBundle(t.TempDir(), ".", "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	names := make([]string, 0, len(uploaded()))
	for name := range uploaded() {
		names = append(names, name)
	}
	assert.ElementsMatch(t, []string{"dbt_project.yml", cosmosBoostArtifact}, names)
	assert.NotContains(t, uploaded()[cosmosBoostArtifact], `"stale"`)
}
