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

func writePins(t *testing.T, dir string, b runtimeversions.AstroBuild, pins *RuntimePins) string {
	t.Helper()
	require.NoError(t, SetAstroBuild(dir, nil, b, pins))
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	return string(out)
}

func projectWith(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(src), 0o600))
	return dir
}

func TestTheRuntimePinsAreWrittenUnderTheirComment(t *testing.T) {
	src := strings.Replace(astroHead, "dependencies = ['apache-airflow==3.3.*']", "dependencies = ['apache-airflow==3.3.*', 'Pandas>=2']", 1) +
		"\n[tool.uv]\nconstraint-dependencies = ['protobuf<7']\n"
	dir := projectWith(t, src)
	out := writePins(t, dir, build332, freeze339)

	want := `constraint-dependencies = [
    'protobuf<7',
    'apache-airflow==3.3.2+astro.1',
    'apache-airflow-task-sdk==1.3.2+astro.1',
    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.
    # astro writes them. To choose a version yourself, list the package in [project] dependencies.
    'a2wsgi==1.10.10',
    'SQLAlchemy==2.0.50',
    'zipp==4.1.0',
]`
	assert.Contains(t, out, want,
		"the project's pandas and protobuf are its own, Airflow's are the build's pins, and an +astro build is not PyPI's")

	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "3.3-9", m.UV.RuntimeBuild)
	assert.Equal(t, []string{"a2wsgi==1.10.10", "SQLAlchemy==2.0.50", "zipp==4.1.0"}, m.UV.RuntimePins)

	again := writePins(t, dir, build332, freeze339)
	assert.Equal(t, out, again, "the same build and pins change nothing")
}

func TestANewBuildReplacesTheRuntimePins(t *testing.T) {
	dir := projectWith(t, astroHead)
	writePins(t, dir, build332, freeze339)
	out := writePins(t, dir, build332, &RuntimePins{Build: "3.3-10", Python: "3.13", Pins: []string{"apache-airflow==3.3.3+astro.1", "zipp==4.2.0"}})
	assert.Contains(t, out, "# Astro Runtime 3.3-10 (Python 3.13) ships these versions")
	assert.Contains(t, out, "'zipp==4.2.0'")
	assert.NotContains(t, out, "3.3-9")
	assert.NotContains(t, out, "a2wsgi", "a package the new build does not ship has no pin")
}

// Not known (offline, say): the pins stay, but one the project has since
// listed is its own. That is how `uv add --frozen 'pandas>=3'` gets past a
// pin uv would not lock beside.
func TestUnknownPinsStayLessTheProjectsOwn(t *testing.T) {
	dir := projectWith(t, astroHead)
	writePins(t, dir, build332, freeze339)
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	added := strings.Replace(string(data), "'apache-airflow==3.3.*',", "'apache-airflow==3.3.*',\n    'pandas>=3',", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(added), 0o600))

	out := writePins(t, dir, build332, nil)
	assert.Contains(t, out, "# Astro Runtime 3.3-9 (Python 3.12) ships these versions")
	assert.Contains(t, out, "'zipp==4.1.0'")
	assert.NotContains(t, out, "'pandas==2.2.3'")
}

func TestNoPublishedListTakesThePinsOut(t *testing.T) {
	dir := projectWith(t, astroHead)
	writePins(t, dir, build332, freeze339)
	out := writePins(t, dir, build332, &RuntimePins{Build: "10.5.0", Python: "3.11"})
	assert.NotContains(t, out, "Astro Runtime")
	assert.NotContains(t, out, "zipp")
	assert.Contains(t, out, "'apache-airflow==3.3.2+astro.1'")

	// No build at all either: the whole array goes, as it did.
	out = writePins(t, dir, runtimeversions.AstroBuild{}, &RuntimePins{})
	assert.NotContains(t, out, "constraint-dependencies")
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
		{name: "a patch pin still reads the series, as the image does", manifest: project("'apache-airflow==3.3.1'", "", ">=3.12"), catalog: c, build: "3.3-9", py: "3.14"},
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
