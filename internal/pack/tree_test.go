package pack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platformversions"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// project builds a project directory with a dags/ folder and returns its path.
// Options layer plugins, include, and env onto it.
type projectOpt func(dir string, m *manifest.Manifest)

func withPlugin(name, body string) projectOpt {
	return func(dir string, _ *manifest.Manifest) {
		writeInto(nil, dir, filepath.Join("plugins", name), body)
	}
}

func withInclude(name, body string) projectOpt {
	return func(dir string, _ *manifest.Manifest) {
		writeInto(nil, dir, filepath.Join("include", name), body)
	}
}

func withPackages(pkgs ...string) projectOpt {
	return func(_ string, m *manifest.Manifest) { m.Astro.Packages = pkgs }
}

func withAirflow(v string) projectOpt {
	return func(_ string, m *manifest.Manifest) { m.Astro.AirflowVersion = v }
}

func withDeps(deps ...string) projectOpt {
	return func(_ string, m *manifest.Manifest) { m.Project.Dependencies = deps }
}

func withEnv(env map[string]any) projectOpt {
	return func(_ string, m *manifest.Manifest) { m.Astro.Env = env }
}

// newProject writes a project with two dags and returns the request over it.
// The default manifest mirrors etl-demo: Airflow 3.1, one provider dep, one
// apache-airflow pin to prove it is dropped.
func newProject(t *testing.T, opts ...projectOpt) Request {
	t.Helper()
	dir := t.TempDir()
	writeInto(t, dir, filepath.Join("dags", "one.py"), "one = 1\n")
	writeInto(t, dir, filepath.Join("dags", "sub", "two.py"), "two = 2\n")
	m := &manifest.Manifest{
		Project: manifest.Project{
			Name:         "demo",
			Dependencies: []string{"apache-airflow==3.1.*", "apache-airflow-providers-standard", "pandas"},
		},
		Astro: manifest.Astro{AirflowVersion: "3.1"},
	}
	for _, opt := range opts {
		opt(dir, m)
	}
	return Request{
		ProjectDir: dir,
		OutDir:     filepath.Join(t.TempDir(), "artifact"),
		Manifest:   m,
	}
}

func writeInto(t *testing.T, dir, rel, body string) {
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		if t != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil && t != nil {
		t.Fatal(err)
	}
}

func readArtifact(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err, "reading %s", rel)
	return string(data)
}

func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// --- shared behavior ---------------------------------------------------------

func TestTreeTargetsCopyDags(t *testing.T) {
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		req := newProject(t)
		res, err := target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err, target.Name())
		assert.Equal(t, KindTree, res.Kind)
		assert.Equal(t, req.OutDir, res.TreePath)
		// Both the top-level and the nested dag land under dags/.
		assert.Equal(t, "one = 1\n", readArtifact(t, req.OutDir, filepath.Join("dags", "one.py")))
		assert.Equal(t, "two = 2\n", readArtifact(t, req.OutDir, filepath.Join("dags", "sub", "two.py")))
	}
}

func TestTreeTargetsDefaultOutDir(t *testing.T) {
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		req := newProject(t)
		req.OutDir = "" // fall back to <projectDir>/dist/<target>
		res, err := target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(req.ProjectDir, "dist", target.Name()), res.TreePath)
	}
}

func TestTreeTargetsRebuildClearsStaleFiles(t *testing.T) {
	req := newProject(t)
	// A stale dag from an earlier build.
	writeInto(t, req.OutDir, filepath.Join("dags", "gone.py"), "old\n")
	_, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(req.OutDir, "dags", "gone.py"))
	assert.True(t, os.IsNotExist(statErr), "a removed dag must not linger after a rebuild")
}

func TestTreeTargetsRejectNilManifest(t *testing.T) {
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		_, err := target.Build(context.Background(), Request{}, localrt.Callbacks{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest")
	}
}

func TestTreeTargetsRequireProjectName(t *testing.T) {
	req := newProject(t)
	req.Manifest.Project.Name = ""
	_, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestTreeTargetsWarnOnOSPackages(t *testing.T) {
	for _, tc := range []struct {
		target Target
		name   string
	}{{NewMWAATarget(), "MWAA"}, {NewComposerTarget(), "Composer"}} {
		req := newProject(t, withPackages("libpq-dev"))
		res, err := tc.target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err)
		assert.True(t, hasWarning(res.Warnings, "libpq-dev"), "%s should warn on OS packages: %v", tc.name, res.Warnings)
		assert.True(t, hasWarning(res.Warnings, tc.name), "warning should name the platform")
	}
}

func TestTreeTargetsSaveZipsArtifact(t *testing.T) {
	req := newProject(t)
	req.Save = filepath.Join(t.TempDir(), "artifact.zip")
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Save, res.SavedPath)
	assert.Positive(t, res.Size)

	names := zipEntryNames(t, req.Save)
	assert.Contains(t, names, "dags/one.py")
	assert.Contains(t, names, "dags/sub/two.py")
	assert.Contains(t, names, "requirements.txt")
}

func TestTreeTargetsNoSaveOmitsPath(t *testing.T) {
	res, err := NewComposerTarget().Build(context.Background(), newProject(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, res.SavedPath)
	assert.Zero(t, res.Size)
}

// --- MWAA --------------------------------------------------------------------

func TestMWAARequirementsDropsAirflowAndPinsConstraint(t *testing.T) {
	// A supported pin resolves to a real constraints URL. Drive it from the
	// shipped data so the test never duplicates the version literals.
	supported := platformversions.MWAA[1] // an exact, supported Airflow version
	req := newProject(t, withAirflow(supported.Airflow))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, res.Warnings, "a supported version warns about nothing")

	reqs := readArtifact(t, req.OutDir, "requirements.txt")
	wantConstraint := fmt.Sprintf(`--constraint "https://raw.githubusercontent.com/apache/airflow/constraints-%s/constraints-%s.txt"`, supported.Airflow, supported.Python)
	assert.Contains(t, reqs, wantConstraint)
	assert.Contains(t, reqs, "apache-airflow-providers-standard")
	assert.Contains(t, reqs, "pandas")
	// apache-airflow itself is dropped: no line is just the base distribution.
	for _, line := range strings.Split(reqs, "\n") {
		assert.NotEqual(t, "apache-airflow==3.1.*", strings.TrimSpace(line), "apache-airflow must be dropped")
	}
	assert.Equal(t, filepath.Join(req.OutDir, "requirements.txt"), res.DepsFile)
}

func TestMWAAPartialPinResolvesToNewest(t *testing.T) {
	// "3" matches the newest supported Airflow 3 on MWAA (the list's first entry).
	newest := platformversions.MWAA[0]
	req := newProject(t, withAirflow("3"))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, res.Warnings)
	want := fmt.Sprintf("constraints-%s/constraints-%s.txt", newest.Airflow, newest.Python)
	assert.Contains(t, readArtifact(t, req.OutDir, "requirements.txt"), want)
}

func TestMWAAUnsupportedPinWarnsAndTemplates(t *testing.T) {
	// etl-demo pins 3.1, which MWAA does not offer.
	req := newProject(t, withAirflow("3.1"))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasWarning(res.Warnings, "3.1"), "should warn about the unsupported pin: %v", res.Warnings)

	reqs := readArtifact(t, req.OutDir, "requirements.txt")
	// No active constraint line; a commented template instead.
	assert.NotContains(t, reqs, "\n--constraint")
	assert.False(t, strings.HasPrefix(reqs, "--constraint"), "no active constraint for an unsupported pin")
	assert.Contains(t, reqs, "# Pick the version your environment runs")
	assert.Contains(t, reqs, "apache-airflow-providers-standard")
}

func TestMWAAZipsPlugins(t *testing.T) {
	req := newProject(t, withPlugin("my_plugin.py", "plugin = 1\n"))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	zipPath := filepath.Join(res.TreePath, "plugins.zip")
	require.FileExists(t, zipPath)
	assert.Contains(t, zipEntryNames(t, zipPath), "my_plugin.py")
	// No stray plugins/ folder — MWAA reads the zip.
	_, statErr := os.Stat(filepath.Join(res.TreePath, "plugins"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestMWAAEmptyPluginsMakesNoZip(t *testing.T) {
	// A plugins/ with only a .gitkeep placeholder is not real content.
	req := newProject(t, withPlugin(".gitkeep", ""))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(res.TreePath, "plugins.zip"))
	assert.True(t, os.IsNotExist(statErr), "an empty plugins/ makes no plugins.zip")
}

func TestMWAANextStepsGiveUploadCommand(t *testing.T) {
	req := newProject(t)
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	require.NotEmpty(t, res.NextSteps)
	assert.Contains(t, res.NextSteps[0], "aws s3 sync")
	assert.Contains(t, res.NextSteps[0], res.TreePath)
}

// --- Composer ----------------------------------------------------------------

func TestComposerDepsFileHasNoConstraintLine(t *testing.T) {
	req := newProject(t)
	res, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, res.Warnings, "3.1 is supported on Composer 3")

	deps := readArtifact(t, req.OutDir, composerDepsFile)
	assert.NotContains(t, deps, "--constraint", "Composer resolves constraints itself")
	assert.Contains(t, deps, "apache-airflow-providers-standard")
	assert.Contains(t, deps, "pandas")
	for _, line := range strings.Split(deps, "\n") {
		assert.NotEqual(t, "apache-airflow==3.1.*", strings.TrimSpace(line), "apache-airflow must be dropped")
	}
	assert.Equal(t, filepath.Join(req.OutDir, composerDepsFile), res.DepsFile)
}

func TestComposerUnsupportedPinWarns(t *testing.T) {
	// 2.5.0 is not in Composer 3's list.
	req := newProject(t, withAirflow("2.5.0"))
	res, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasWarning(res.Warnings, "2.5.0"), "should warn: %v", res.Warnings)
}

func TestComposerCopiesPluginsAsFolder(t *testing.T) {
	req := newProject(t, withPlugin("my_plugin.py", "plugin = 1\n"))
	res, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	// Composer reads gs://bucket/plugins, so plugins ship as a folder, not a zip.
	assert.Equal(t, "plugin = 1\n", readArtifact(t, res.TreePath, filepath.Join("plugins", "my_plugin.py")))
	_, statErr := os.Stat(filepath.Join(res.TreePath, "plugins.zip"))
	assert.True(t, os.IsNotExist(statErr), "Composer takes a folder, not a zip")
}

func TestComposerNextStepsSplitBucketAndEnv(t *testing.T) {
	req := newProject(t)
	res, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	joined := strings.Join(res.NextSteps, "\n")
	assert.Contains(t, joined, "gcloud composer environments storage dags import")
	assert.Contains(t, joined, "--update-pypi-packages-from-file")
	assert.Contains(t, joined, composerDepsFile)
}

// --- env checklist -----------------------------------------------------------

func envSchema() map[string]any {
	return map[string]any{
		"vars": map[string]any{
			"API_URL": map[string]any{"type": "url", "required": true, "description": "upstream base URL"},
			"DEBUG":   map[string]any{"type": "bool"},
		},
		"connections": map[string]any{
			"warehouse": map[string]any{"conn_type": "postgres", "required": true},
		},
	}
}

func TestTreeTargetsWriteEnvChecklist(t *testing.T) {
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		req := newProject(t, withEnv(envSchema()))
		res, err := target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err, target.Name())
		body := readArtifact(t, res.TreePath, checklistName)
		assert.Contains(t, body, "API_URL")
		assert.Contains(t, body, "required")
		assert.Contains(t, body, "upstream base URL")
		assert.Contains(t, body, "warehouse")
		assert.Contains(t, body, "postgres")
	}
}

func TestTreeTargetsNoEnvNoChecklist(t *testing.T) {
	req := newProject(t) // no env declared
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(res.TreePath, checklistName))
	assert.True(t, os.IsNotExist(statErr), "no env section means no checklist")
}

func TestEnvChecklistSurfacesSchemaError(t *testing.T) {
	req := newProject(t, withEnv(map[string]any{
		"vars": map[string]any{"API_URL": map[string]any{"type": "nonsense"}},
	}))
	_, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err, "a broken env schema should not pass silently")
	assert.Contains(t, err.Error(), "checklist")
}

// --- warnings on include/ ----------------------------------------------------

func TestTreeTargetsWarnOnInclude(t *testing.T) {
	// include/ is not part of the bucket layout; the build succeeds but warns.
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		req := newProject(t, withInclude("helper.py", "h = 1\n"))
		res, err := target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err, target.Name())
		// include/ is not copied into the bucket tree.
		_, statErr := os.Stat(filepath.Join(res.TreePath, "include"))
		assert.True(t, os.IsNotExist(statErr))
		assert.True(t, hasWarning(res.Warnings, "include/"), "%s should warn about include/: %v", target.Name(), res.Warnings)
	}
}

func TestTreeTargetsEmptyIncludeNoWarning(t *testing.T) {
	// A .gitkeep-only include/ (etl-demo's shape) is not real content.
	req := newProject(t, withInclude(".gitkeep", ""))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, hasWarning(res.Warnings, "include/"), "an empty include/ should not warn")
}

// --- json shape --------------------------------------------------------------

func TestTreeResultJSONShape(t *testing.T) {
	req := newProject(t, withAirflow("3.1"), withPackages("libpq-dev"))
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	data, err := json.Marshal(res)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "mwaa", got["target"])
	assert.Equal(t, "tree", got["kind"])
	assert.Equal(t, res.TreePath, got["tree_path"])
	assert.Equal(t, res.DepsFile, got["deps_file"])
	assert.NotEmpty(t, got["warnings"])
	assert.NotEmpty(t, got["next_steps"])
	// Image fields stay absent for a tree result.
	assert.NotContains(t, got, "image")
	assert.NotContains(t, got, "saved_path")
}

func zipEntryNames(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer r.Close()
	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	return names
}
