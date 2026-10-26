package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

var freeze339 = &RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{
	"zipp==4.1.0",
	"apache-airflow==3.3.2+astro.2",
	"apache-airflow-core==3.3.2+astro.2",
	"apache-airflow-task-sdk==1.3.2+astro.1",
	"astronomer-kubernetes-executor==10.22.0+astro.1",
	"pandas==2.2.3",
	"protobuf==6.33.6",
	"SQLAlchemy==2.0.50",
	"a2wsgi==1.10.10",
}}

func projectWith(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(src), 0o600))
	return dir
}

// setBuild is SetAstroBuild with no wrap, keeping the seed record, for a test
// of the Airflow build's settings alone.
func setBuild(dir string, b runtimeversions.AstroBuild) error {
	refused, err := SetAstroBuildSeed(dir, nil, b, nil)
	if err == nil && refused {
		return errors.New("the seed record was refused")
	}
	return err
}

// seedFor is SeedManifest's scratch manifest for dir, read back as the
// constraints it states.
func seedFor(t *testing.T, dir string, pins *RuntimePins) (constraints []string, n int) {
	t.Helper()
	out, n, err := SeedManifest(dir, pins)
	require.NoError(t, err)
	if out == nil {
		return nil, n
	}
	m, err := manifest.Parse(out)
	require.NoError(t, err)
	return m.UV.ConstraintDependencies, n
}

// The scratch copy states the image's versions, each limited to its Python,
// beside the project's own constraints and Airflow's build pins, and leaves
// out what a seed never holds: Airflow itself, Astronomer's own builds, and a
// package the project gives a version the image's does not meet.
func TestSeedManifestStatesTheImagesVersions(t *testing.T) {
	src := strings.Replace(astroHead, "dependencies = ['apache-airflow==3.3.*']", "dependencies = ['apache-airflow==3.3.*', 'Pandas>=3']", 1) +
		"\n[tool.uv]\nconstraint-dependencies = ['protobuf>=7', 'apache-airflow==3.3.2+astro.1']\n"
	dir := projectWith(t, src)
	got, n := seedFor(t, dir, freeze339)
	assert.Equal(t, []string{
		"protobuf>=7",
		"apache-airflow==3.3.2+astro.1",
		"a2wsgi==1.10.10; python_version == '3.12'",
		"SQLAlchemy==2.0.50; python_version == '3.12'",
		"zipp==4.1.0; python_version == '3.12'",
	}, got)
	assert.Equal(t, 3, n)
	assert.Equal(t, src, readManifest(t, dir), "the project's own manifest is not touched")
}

func TestThePinOfAPackageTheProjectNamesFollowsItsRequirement(t *testing.T) {
	src := strings.Replace(astroHead, "dependencies = ['apache-airflow==3.3.*']", "dependencies = ['apache-airflow==3.3.*', 'pandas>=2.2', 'numpy>=3', \"tomli>=1; python_version < '3.11'\"]", 1) +
		"\n[project.optional-dependencies]\nplot = ['zipp', 'matplotlib<3']\n\n[dependency-groups]\ndev = ['pytest>=9', {include-group = 'lint'}]\nlint = ['a2wsgi']\n" +
		"\n[tool.uv]\noverride-dependencies = ['protobuf>=6']\ndev-dependencies = ['ruff']\nconstraint-dependencies = ['SQLAlchemy<2.1']\n"
	dir := projectWith(t, src)
	got, _ := seedFor(t, dir, &RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{
		"pandas==2.2.3", "numpy==2.3.0", "zipp==4.1.0", "matplotlib==3.10.0", "pytest==8.4.0", "a2wsgi==1.10.10",
		"protobuf==6.33.6", "ruff==0.14.0", "SQLAlchemy==2.0.50", "tomli==2.3.0",
	}})
	joined := strings.Join(got, "\n")
	for _, kept := range []string{"pandas==", "zipp==", "a2wsgi==", "ruff==", "SQLAlchemy==2.0.50;", "tomli=="} {
		assert.Contains(t, joined, kept, "the project's requirement admits the image's version")
	}
	for _, dropped := range []string{"numpy==", "matplotlib==", "pytest==", "protobuf=="} {
		assert.NotContains(t, joined, dropped, "excluded by the project's requirement, or overridden")
	}
}

// Nothing left to seed with: no scratch manifest at all.
func TestSeedManifestWithNothingToSeed(t *testing.T) {
	dir := projectWith(t, astroHead)
	out, n, err := SeedManifest(dir, &RuntimePins{Build: "10.5.0", Python: "3.11"})
	require.NoError(t, err)
	assert.Nil(t, out)
	assert.Zero(t, n)
}

// The seed locks the project in place, so its sources stay as they are
// written, relative paths and all, and a workspace's root is seeded too. The
// runtime pins an older astro wrote never reach the seed.
func TestSeedManifestLeavesTheProjectAsItIsWritten(t *testing.T) {
	src := strings.Replace(astroHead, "dependencies = ['apache-airflow==3.3.*']", "dependencies = ['apache-airflow==3.3.*', 'mylib']", 1) +
		"\n[tool.uv.sources]\nmylib = { path = 'libs/mylib' }\n"
	dir := projectWith(t, src)
	out, _, err := SeedManifest(dir, freeze339)
	require.NoError(t, err)
	assert.Contains(t, string(out), "'libs/mylib'")

	ws := projectWith(t, astroHead+"\n[tool.uv.workspace]\nmembers = ['packages/*']\n")
	_, n, err := SeedManifest(ws, freeze339)
	require.NoError(t, err)
	assert.Positive(t, n)

	legacy := projectWith(t, astroHead+"\n[tool.uv]\nconstraint-dependencies = [\n"+
		"    'apache-airflow==3.3.2+astro.1',\n"+
		"    # Astro Runtime 3.3-8 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.\n"+
		"    \"zipp==3.0.0; python_version == '3.12'\",\n"+
		"    # End of what astro writes. Constraints below this line are yours.\n"+
		"    'numpy<3',\n]\n")
	out, _, err = SeedManifest(legacy, freeze339)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "zipp==3.0.0")
	assert.Contains(t, string(out), "zipp==4.1.0")
	assert.Contains(t, string(out), "numpy<3")
	assert.Contains(t, string(out), "apache-airflow==3.3.2+astro.1")
}

// The record follows Airflow's build pins and the project's own constraints;
// nil keeps it, a zero Seed takes it out.
func TestSetAstroBuildWritesTheSeedRecord(t *testing.T) {
	dir := projectWith(t, astroHead+"\n[tool.uv]\nconstraint-dependencies = ['numpy<3']\n")
	_, err := SetAstroBuildSeed(dir, nil, build332, &Seed{Build: "3.3-9", Python: "3.12"})
	require.NoError(t, err)
	want := `constraint-dependencies = [
    'numpy<3',
    'apache-airflow==3.3.2+astro.1',
    'apache-airflow-task-sdk==1.3.2+astro.1',
    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.
]`
	assert.Contains(t, readManifest(t, dir), want)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "3.3-9", m.UV.RuntimeBuild)
	assert.Equal(t, "3.12", m.UV.RuntimePython)

	before := readManifest(t, dir)
	_, err = SetAstroBuildSeed(dir, nil, build332, nil)
	require.NoError(t, err)
	assert.Equal(t, before, readManifest(t, dir), "nil keeps the record")

	_, err = SetSeedRecord(dir, nil, &Seed{})
	require.NoError(t, err)
	out := readManifest(t, dir)
	assert.NotContains(t, out, "astro seeded")
	assert.Contains(t, out, "'apache-airflow==3.3.2+astro.1'")
}

// With no Astronomer build of Airflow, the record still has an array to sit
// in, with no constraint in it.
func TestTheSeedRecordAloneKeepsTheArray(t *testing.T) {
	dir := projectWith(t, astroHead)
	_, err := SetAstroBuildSeed(dir, nil, runtimeversions.AstroBuild{}, &Seed{Build: "3.3-9", Python: "3.12"})
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "3.3-9", m.UV.RuntimeBuild)
	assert.Empty(t, m.UV.ConstraintDependencies)

	_, err = SetSeedRecord(dir, nil, &Seed{})
	require.NoError(t, err)
	assert.NotContains(t, readManifest(t, dir), "constraint-dependencies")
}

// The runtime pins an older astro wrote come out at the next write, with
// their comments, and the project's own lines after them stay.
func TestTheRuntimePinsOfAnOlderAstroAreTakenOut(t *testing.T) {
	block := "    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.\n" +
		"    # astro writes them. To choose a version yourself, give the package a bound the image's version doesn't meet.\n" +
		"    \"a2wsgi==1.10.10; python_version == '3.12'\",\n" +
		"    \"zipp==4.1.0; python_version == '3.12'\",\n"
	for name, end := range map[string]string{
		"ended":     "    # End of what astro writes. Constraints below this line are yours.\n",
		"not ended": "",
	} {
		t.Run(name, func(t *testing.T) {
			src := astroHead + "\n[tool.uv]\nconstraint-dependencies = [\n    'numpy<3',\n    'apache-airflow==3.3.2+astro.1',\n" + block + end + "    'urllib3<3',\n]\n"
			dir := projectWith(t, src)
			_, err := SetAstroBuildSeed(dir, nil, build332, &Seed{Build: "3.3-9", Python: "3.12"})
			require.NoError(t, err)
			out := readManifest(t, dir)
			for _, gone := range []string{"ships these versions", "a2wsgi", "zipp", "End of what astro writes"} {
				assert.NotContains(t, out, gone)
			}
			for _, kept := range []string{"'numpy<3'", "'urllib3<3'", "# astro seeded uv.lock from Astro Runtime 3.3-9"} {
				assert.Contains(t, out, kept)
			}
		})
	}
}

// A [tool.uv] written as an inline table cannot hold the record, so it is
// refused, and the rest is written.
func TestTheSeedRecordIsRefusedWhereItCannotBeWritten(t *testing.T) {
	src := strings.Replace(astroHead, "[tool.astro]\n", "[tool.astro]\n\n[tool]\nuv = { constraint-dependencies = ['numpy<3'] }\n", 1)
	dir := projectWith(t, src)
	refused, err := SetAstroBuildSeed(dir, nil, runtimeversions.AstroBuild{}, &Seed{Build: "3.3-9", Python: "3.12"})
	require.NoError(t, err)
	assert.True(t, refused)
	out := readManifest(t, dir)
	assert.Contains(t, out, "numpy<3")
	assert.NotContains(t, out, "astro seeded")
}

// SeededPins are what the seed states, by the same filter: less a version the
// project chooses itself, Airflow's distributions by name, and Astronomer's
// own builds.
func TestSeededPins(t *testing.T) {
	src := strings.Replace(astroHead, "dependencies = ['apache-airflow==3.3.*']", "dependencies = ['apache-airflow==3.3.*', 'pandas>=3', 'zipp']", 1)
	dir := projectWith(t, src)
	pins := &RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{
		"pandas==2.2.3", "zipp==4.1.0", "SQLAlchemy==2.0.50",
		"apache-airflow-task-sdk==1.3.2", "astronomer-providers-logging==1.6.8+astro.1",
	}}
	assert.Equal(t, []string{"SQLAlchemy==2.0.50", "zipp==4.1.0"}, SeededPins(dir, pins))
	assert.Empty(t, SeededPins(filepath.Join(dir, "missing"), pins))
}

const pinsCatalog = `{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}}
  }, "runtimeVersions": {
    "12.5.0": {"metadata": {"airflowVersion": "2.10.3", "channel": "deprecated"}},
    "12.6.0": {"metadata": {"airflowVersion": "2.10.4", "channel": "deprecated"}}
  }}`

func TestRuntimeBuildFor(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(pinsCatalog))
	require.NoError(t, err)
	project := func(deps, astro, requiresPython string) string {
		return "[project]\nname = 'p'\nrequires-python = '" + requiresPython + "'\ndependencies = [" + deps + "]\n\n[tool.astro]\n" + astro
	}
	cases := []struct {
		name        string
		manifest    string
		dockerfile  string
		catalog     *runtimeversions.Catalog
		build, py   string
		none, unset bool
	}{
		{name: "an Airflow 3 pin takes its series' newest build, and the default Python it admits", manifest: project("'apache-airflow==3.3.*'", "", ">=3.12"), catalog: c, build: "3.3-9", py: "3.14"},
		{name: "a patch pin takes the newest build of that release, on the Python the project runs", manifest: project("'apache-airflow==3.3.1'", "", ">=3.12"), catalog: c, build: "3.3-8", py: "3.14"},
		{name: "requires-python picks the Python the image runs", manifest: project("'apache-airflow==3.3.*'", "", ">=3.12,<3.14"), catalog: c, build: "3.3-9", py: "3.13"},
		{name: "runtime names the build", manifest: project("'apache-airflow==3.3.*'", "runtime = '3.3-8'\n", ">=3.12"), catalog: c, build: "3.3-8", py: "3.13"},
		{name: "an Airflow 2 pin runs its build's own Python", manifest: project("'apache-airflow==2.10.*'", "", ">=3.11"), catalog: c, build: "12.6.0", py: "3.12"},
		{name: "a series no build carries", manifest: project("'apache-airflow==3.4.*'", "", ">=3.12"), catalog: c, none: true},
		{name: "no catalog, no build named", manifest: project("'apache-airflow==3.3.*'", "", ">=3.12"), unset: true},
		{name: "no catalog, an Airflow 2 build named", manifest: project("'apache-airflow==2.10.*'", "runtime = '12.6.0'\n", ">=3.11"), build: "12.6.0", py: "3.12"},
		{name: "a Dockerfile's exact build and its Python flavor", manifest: project("'apache-airflow==3.3.*'", "dockerfile = 'Dockerfile'\n", ">=3.12"), dockerfile: "FROM astrocrpublic.azurecr.io/runtime:3.3-8-python-3.12-base\n", catalog: c, build: "3.3-8", py: "3.12"},
		{name: "a Dockerfile's floating tag serves the newest build", manifest: project("'apache-airflow==3.3.*'", "dockerfile = 'Dockerfile'\n", ">=3.12"), dockerfile: "FROM astrocrpublic.azurecr.io/runtime:3.3\n", catalog: c, build: "3.3-9", py: "3.14"},
		{name: "a Dockerfile's Airflow 2 ONBUILD tag", manifest: project("'apache-airflow==2.10.*'", "dockerfile = 'Dockerfile'\n", ">=3.11"), dockerfile: "FROM quay.io/astronomer/astro-runtime:12.5.0\nRUN true\n", build: "12.5.0", py: "3.12"},
		{name: "a Dockerfile on another image has no build", manifest: project("'apache-airflow==3.3.*'", "dockerfile = 'Dockerfile'\n", ">=3.12"), dockerfile: "FROM python:3.12\n", catalog: c, none: true},
		{name: "a Dockerfile built from an argument cannot be told", manifest: project("'apache-airflow==3.3.*'", "dockerfile = 'Dockerfile'\n", ">=3.12"), dockerfile: "ARG BASE\nFROM ${BASE}\n", catalog: c, unset: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := projectWith(t, tc.manifest)
			if tc.dockerfile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(tc.dockerfile), 0o600))
			}
			m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
			require.NoError(t, err)
			build, py, err := RuntimeBuildFor(dir, m, tc.catalog)
			switch {
			case tc.none:
				assert.True(t, errors.Is(err, ErrNoRuntimeBuild), "err = %v, want ErrNoRuntimeBuild", err)
			case tc.unset:
				assert.Error(t, err)
				assert.False(t, errors.Is(err, ErrNoRuntimeBuild), "err = %v: not knowing is not knowing there is none", err)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.build, build)
				assert.Equal(t, tc.py, py)
			}
		})
	}
}
