package pack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/platformversions"
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

// withAirflow repins the project's Airflow requirement, which is where the
// manifest states the version.
func withAirflow(v string) projectOpt {
	return func(_ string, m *manifest.Manifest) {
		deps := slices.DeleteFunc(slices.Clone(m.Project.Dependencies), manifest.NamesAirflow)
		m.Project.Dependencies = append([]string{manifest.AirflowRequirement(v)}, deps...)
	}
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

// A local run leaves __pycache__ behind, and the artifact used to carry it into
// the bucket — including bytecode for DAGs that had since been deleted, which
// leaks the names of files no longer in the project, and bytecode built by
// whichever interpreter happened to run locally.
func TestTreeTargetsSkipBytecode(t *testing.T) {
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		req := newProject(t, withPlugin("p.py", "p = 1\n"))
		// What a local parse leaves behind, at every level the walk reaches.
		writeInto(t, req.ProjectDir, filepath.Join("dags", "__pycache__", "one.cpython-313.pyc"), "bytecode\n")
		writeInto(t, req.ProjectDir, filepath.Join("dags", "__pycache__", "deleted.cpython-312.pyc"), "bytecode\n")
		writeInto(t, req.ProjectDir, filepath.Join("dags", "sub", "__pycache__", "two.cpython-313.pyc"), "bytecode\n")
		writeInto(t, req.ProjectDir, filepath.Join("plugins", "__pycache__", "p.cpython-313.pyc"), "bytecode\n")
		// A stray .pyc outside __pycache__ is still not source.
		writeInto(t, req.ProjectDir, filepath.Join("dags", "loose.pyc"), "bytecode\n")

		_, err := target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err, target.Name())

		// The source still ships.
		assert.Equal(t, "one = 1\n", readArtifact(t, req.OutDir, filepath.Join("dags", "one.py")), target.Name())
		assert.Equal(t, "two = 2\n", readArtifact(t, req.OutDir, filepath.Join("dags", "sub", "two.py")), target.Name())

		// A loose .pyc with no source beside it DOES ship: it is importable
		// (PEP 3147's legacy layout), so a project may be vendoring a
		// compiled-only module and dropping it would break the DAG that
		// imports it.
		assert.Equal(t, "bytecode\n", readArtifact(t, req.OutDir, filepath.Join("dags", "loose.pyc")), target.Name())

		// No cache directory survives, anywhere in the artifact. Checked by
		// path COMPONENT, not substring: a directory the user named
		// something.pyc is their file and ships.
		require.NoError(t, filepath.WalkDir(req.OutDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				assert.NotEqual(t, pycacheDir, d.Name(), "%s: %s", target.Name(), path)
			}
			return nil
		}), target.Name())

		// MWAA zips its plugins rather than copying them, so walking the tree
		// would not see inside. Look in the zip too.
		if target.Name() == TargetMWAA {
			zr, err := zip.OpenReader(filepath.Join(req.OutDir, "plugins.zip"))
			require.NoError(t, err)
			// Assert the zip has contents BEFORE asserting what is absent: a
			// loop over an empty zip runs no assertions at all, which is how an
			// empty plugins.zip passed this test before.
			require.NotEmpty(t, zr.File, "plugins.zip is empty")
			var names []string
			for _, f := range zr.File {
				names = append(names, f.Name)
				assert.NotContains(t, strings.Split(f.Name, "/"), pycacheDir,
					"plugins.zip carries a cache dir: %s", f.Name)
			}
			assert.Contains(t, names, "p.py", "the real plugin did not ship")
			zr.Close()
		}
	}
}

// The gate that decides whether MWAA is told to upload a plugins.zip has to use
// the same rule as the walk that fills it. It did not: it asked "is there a file
// that is not named *.pyc", while the walk skipped the whole __pycache__
// directory. So one non-.pyc file inside that directory — Cython's .so, or the
// temp file CPython leaves when a parse is killed mid-write — produced a
// 22-byte, zero-entry zip, with the next-steps text still telling the user to
// point a live MWAA environment at it.
func TestMWAAPluginsZipIsNeverEmptyWhenAdvertised(t *testing.T) {
	for _, tc := range []struct{ name, rel string }{
		{"cython artifact in the cache", filepath.Join("plugins", pycacheDir, "fast.cpython-313-darwin.so")},
		{"interrupted write in the cache", filepath.Join("plugins", pycacheDir, "p.cpython-313.pyc.918273")},
		{"only bytecode in the cache", filepath.Join("plugins", pycacheDir, "p.cpython-313.pyc")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := newProject(t)
			writeInto(t, req.ProjectDir, tc.rel, "not source\n")

			res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
			require.NoError(t, err)

			zipPath := filepath.Join(req.OutDir, "plugins.zip")
			if _, statErr := os.Stat(zipPath); statErr == nil {
				zr, openErr := zip.OpenReader(zipPath)
				require.NoError(t, openErr)
				defer zr.Close()
				require.NotEmpty(t, zr.File, "an advertised plugins.zip must not be empty")
			}
			// And if there is no zip, nothing may tell the user to upload one.
			for _, step := range res.NextSteps {
				assert.NotContains(t, step, "plugins.zip",
					"next steps advertise a plugins.zip that was not written")
			}
		})
	}
}

// A symlink is not a regular file, so zipDir drops it — which means a plugins/
// whose only entry is one must not be advertised either. Same failure as the
// cache mismatch, by a different route.
func TestMWAASymlinkOnlyPluginsMakesNoZip(t *testing.T) {
	req := newProject(t)
	realFile := filepath.Join(req.ProjectDir, "include", "real.py")
	writeInto(t, req.ProjectDir, filepath.Join("include", "real.py"), "x = 1\n")
	require.NoError(t, os.MkdirAll(filepath.Join(req.ProjectDir, "plugins"), 0o755))
	if err := os.Symlink(realFile, filepath.Join(req.ProjectDir, "plugins", "link.py")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(req.OutDir, "plugins.zip"))
	for _, step := range res.NextSteps {
		assert.NotContains(t, step, "plugins.zip")
	}
}

// dirHasFiles also gates the include/ warning, so the rule it uses must not
// quietly change what gets warned about. An include/ holding a real module —
// even a compiled one — still warns, because include/ is never shipped and the
// DAG importing from it breaks at runtime.
func TestIncludeWarningSurvivesCompiledOnlyModules(t *testing.T) {
	req := newProject(t)
	writeInto(t, req.ProjectDir, filepath.Join("include", "shared.pyc"), "bytecode\n")

	res, err := NewComposerTarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasWarning(res.Warnings, "include"),
		"include/ with a compiled module produced no warning: %v", res.Warnings)
}

// A plugins/ holding nothing but a local run's __pycache__ has no plugins in
// it, so it must not yield a plugins.zip the next-steps text then tells the
// user to upload.
func TestMWAABytecodeOnlyPluginsMakesNoZip(t *testing.T) {
	req := newProject(t)
	writeInto(t, req.ProjectDir, filepath.Join("plugins", "__pycache__", "p.cpython-313.pyc"), "bytecode\n")

	_, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(req.OutDir, "plugins.zip"))
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
		assert.False(t, manifest.NamesAirflow(line), "apache-airflow must be dropped: %q", line)
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

// The upload command names the bucket [tool.astro.targets.mwaa] declares,
// written with or without its scheme, and keeps the placeholder otherwise.
func TestMWAANextStepsNameTheDeclaredBucket(t *testing.T) {
	for _, tc := range []struct {
		name, section, want string
	}{
		{"with scheme", "bucket = 's3://acme-airflow-orders'\n", " s3://acme-airflow-orders/"},
		{"bare name, trailing slash", "bucket = 'acme-airflow-orders/'\n", " s3://acme-airflow-orders/"},
		{"no bucket", "region = 'us-east-1'\n", " s3://<your-mwaa-bucket>/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := manifest.Parse([]byte("[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n\n[tool.astro.targets.mwaa]\n" + tc.section))
			require.NoError(t, err)
			req := newProject(t)
			req.Manifest.Astro.Targets = m.Astro.Targets
			res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
			require.NoError(t, err)
			require.NotEmpty(t, res.NextSteps)
			assert.True(t, strings.HasSuffix(res.NextSteps[0], tc.want), "%q should end with %q", res.NextSteps[0], tc.want)
		})
	}
}

// --- Composer ----------------------------------------------------------------

// A core-only project packages without its Airflow requirement, for both
// platforms: each ships Airflow itself, and a core pin in the upload asks it to
// install a second one over it.
func TestTreeTargetsDropAirflowCore(t *testing.T) {
	for name, target := range map[string]struct {
		build func(Request) (Result, error)
		file  string
	}{
		"mwaa": {func(r Request) (Result, error) {
			return NewMWAATarget().Build(context.Background(), r, localrt.Callbacks{})
		}, "requirements.txt"},
		"composer": {func(r Request) (Result, error) {
			return NewComposerTarget().Build(context.Background(), r, localrt.Callbacks{})
		}, composerDepsFile},
	} {
		t.Run(name, func(t *testing.T) {
			req := newProject(t, withDeps("apache-airflow-core==3.1.*", "apache-airflow-providers-standard"))
			_, err := target.build(req)
			require.NoError(t, err)
			body := readArtifact(t, req.OutDir, target.file)
			assert.NotContains(t, body, "apache-airflow-core", "the core pin reached the upload")
			assert.Contains(t, body, "apache-airflow-providers-standard")
		})
	}
}

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
		"API_URL":   map[string]any{},                      // {} : required
		"DEBUG":     "false",                               // a committed default
		"API_TOKEN": map[string]any{"source": "workspace"}, // from Environment Manager
		"connections": map[string]any{
			"warehouse": map[string]any{}, // {} : required
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
		assert.Contains(t, body, "(required)")
		assert.Contains(t, body, "DEBUG")
		assert.Contains(t, body, "(has a default)")
		assert.Contains(t, body, "API_TOKEN")
		assert.Contains(t, body, "(from workspace)")
		assert.Contains(t, body, "warehouse")
	}
}

func TestTreeTargetsNoEnvNoChecklist(t *testing.T) {
	req := newProject(t) // no env declared
	res, err := NewMWAATarget().Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(res.TreePath, checklistName))
	assert.True(t, os.IsNotExist(statErr), "no env section means no checklist")
}

// TestEnvChecklistHonorsOptional reads the annotations off a hand-written
// manifest, one checklist line per value, because "(required)" appearing
// somewhere in the file says nothing about which entry it belongs to.
func TestEnvChecklistHonorsOptional(t *testing.T) {
	m, err := manifest.Parse([]byte(`[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.env]
API_URL = {}
SLACK_WEBHOOK = { optional = true }
LOG_LEVEL = { optional = true, default = 'info' }
API_TOKEN = { optional = true, source = 'workspace' }
NOT_OPTIONAL = { optional = false }

[tool.astro.env.connections]
reporting = { conn_type = 'postgres', optional = true }
`))
	require.NoError(t, err)
	for _, target := range []Target{NewMWAATarget(), NewComposerTarget()} {
		req := newProject(t, withEnv(m.Astro.Env))
		res, err := target.Build(context.Background(), req, localrt.Callbacks{})
		require.NoError(t, err, target.Name())
		lines := strings.Split(readArtifact(t, res.TreePath, checklistName), "\n")
		for _, want := range []string{
			"- [ ] API_URL (required)",
			"- [ ] SLACK_WEBHOOK (optional)",
			"- [ ] LOG_LEVEL (has a default)",
			"- [ ] API_TOKEN (from workspace)",
			"- [ ] NOT_OPTIONAL (required)",
			"- [ ] reporting (optional)",
		} {
			assert.Contains(t, lines, want, target.Name())
		}
	}
}

func TestEnvChecklistSurfacesSchemaError(t *testing.T) {
	req := newProject(t, withEnv(map[string]any{
		"API_URL": map[string]any{"source": "nonsense"},
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
