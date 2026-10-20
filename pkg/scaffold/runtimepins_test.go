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

// setBuild is SetAstroBuild with no wrap, for a test that expects the pins
// written whenever they are given.
func setBuild(dir string, b runtimeversions.AstroBuild, pins *RuntimePins) error {
	refused, err := SetAstroBuild(dir, nil, b, pins)
	if err == nil && refused {
		return errors.New("the runtime pins were refused")
	}
	return err
}

func writePins(t *testing.T, dir string, b runtimeversions.AstroBuild, pins *RuntimePins) string {
	t.Helper()
	require.NoError(t, setBuild(dir, b, pins))
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

func TestTheRuntimePinsAreWrittenBetweenTheirComments(t *testing.T) {
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
    "a2wsgi==1.10.10; python_version == '3.12'",
    "SQLAlchemy==2.0.50; python_version == '3.12'",
    "zipp==4.1.0; python_version == '3.12'",
    # End of what astro writes. Constraints below this line are yours.
]`
	assert.Contains(t, out, want,
		"the project's pandas and protobuf are its own, Airflow's are the build's pins, and an +astro build is not PyPI's")

	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, "3.3-9", m.UV.RuntimeBuild)
	assert.Equal(t, []string{
		"a2wsgi==1.10.10; python_version == '3.12'",
		"SQLAlchemy==2.0.50; python_version == '3.12'",
		"zipp==4.1.0; python_version == '3.12'",
	}, m.UV.RuntimePins, "each pin holds the image's Python alone, so a Python requires-python allows beside it still locks")

	again := writePins(t, dir, build332, freeze339)
	assert.Equal(t, out, again, "the same build and pins change nothing")
}

func TestANewBuildReplacesTheRuntimePins(t *testing.T) {
	dir := projectWith(t, astroHead)
	writePins(t, dir, build332, freeze339)
	out := writePins(t, dir, build332, &RuntimePins{Build: "3.3-10", Python: "3.13", Pins: []string{"apache-airflow==3.3.3+astro.1", "zipp==4.2.0"}})
	assert.Contains(t, out, "# Astro Runtime 3.3-10 (Python 3.13) ships these versions")
	assert.Contains(t, out, "zipp==4.2.0;")
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
	assert.Contains(t, out, "zipp==4.1.0;")
	assert.NotContains(t, out, "pandas==2.2.3")
}

// A constraint the project adds after the pins, as anyone appending to the
// array does, is its own: a refresh keeps it, after the pins, and it takes
// that package's pin out.
func TestAConstraintAppendedAfterThePinsStays(t *testing.T) {
	dir := projectWith(t, astroHead)
	written := writePins(t, dir, build332, freeze339)
	appended := strings.Replace(written, "    # End of what astro writes. Constraints below this line are yours.\n",
		"    # End of what astro writes. Constraints below this line are yours.\n    'zipp<5',\n    'numpy==2.3.0',\n", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(appended), 0o600))

	for _, pins := range []*RuntimePins{freeze339, nil} {
		out := writePins(t, dir, build332, pins)
		assert.Contains(t, out, "Constraints below this line are yours.\n    'zipp<5',\n    'numpy==2.3.0',\n]")
		assert.NotContains(t, out, "zipp==4.1.0;", "the project constrains zipp itself")
	}
}

// A block written without the end comment, as before it existed, ends at its
// last pin: a constraint after it is not taken for one, and the block gains
// the comment.
func TestABlockWithoutItsEndKeepsWhatFollows(t *testing.T) {
	src := astroHead + "\n[tool.uv]\nconstraint-dependencies = [\n" +
		"    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.\n" +
		"    'a2wsgi==1.10.10',\n    'numpy<3',\n]\n"
	dir := projectWith(t, src)
	out := writePins(t, dir, build332, nil)
	assert.Contains(t, out, "'a2wsgi==1.10.10',\n    # End of what astro writes. Constraints below this line are yours.\n    'numpy<3',\n]")
}

// A package in a dependency group or an extra is the project's to version
// too: `uv add --dev 'pytest>=9'` must lock where the image ships pytest 8.
func TestGroupsAndExtrasAreTheProjectsOwn(t *testing.T) {
	src := astroHead + "\n[project.optional-dependencies]\nplot = ['zipp']\n\n[dependency-groups]\ndev = ['pytest>=9', {include-group = 'lint'}]\nlint = ['a2wsgi']\n" +
		"\n[tool.uv]\noverride-dependencies = ['protobuf>=7']\ndev-dependencies = ['ruff']\n"
	dir := projectWith(t, src)
	out := writePins(t, dir, build332, &RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{
		"pytest==8.4.0", "zipp==4.1.0", "a2wsgi==1.10.10", "SQLAlchemy==2.0.50", "protobuf==6.33.6", "ruff==0.14.0",
	}})
	assert.Contains(t, out, "SQLAlchemy==2.0.50;")
	for _, pin := range []string{"pytest==", "zipp==", "a2wsgi==", "protobuf==", "ruff=="} {
		assert.NotContains(t, out, pin)
	}
}

// ListedPackages is what the project lists to install: not an override or a
// constraint, which install nothing of their own.
func TestListedPackages(t *testing.T) {
	src := astroHead + "\n[project.optional-dependencies]\nplot = ['Zipp>=4']\n\n[dependency-groups]\ndev = ['pytest>=9', {include-group = 'lint'}]\n" +
		"\n[tool.uv]\ndev-dependencies = ['ruff']\noverride-dependencies = ['protobuf>=7']\nconstraint-dependencies = ['numpy<3']\n"
	listed, err := ListedPackages(projectWith(t, src))
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"apache-airflow": true, "zipp": true, "pytest": true, "ruff": true}, listed)
}

// PruneRuntimePins needs no lookup: a package the project has since listed
// loses its pin, and nothing else changes.
func TestPruneRuntimePinsDropsWhatTheProjectNowLists(t *testing.T) {
	dir := projectWith(t, astroHead)
	written := writePins(t, dir, build332, freeze339)
	added := strings.Replace(written, "'apache-airflow==3.3.*',", "'apache-airflow==3.3.*',\n    'pandas>=3',", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(added), 0o600))

	require.NoError(t, PruneRuntimePins(dir, nil))
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(added, "    \"pandas==2.2.3; python_version == '3.12'\",\n", "", 1), string(out))
}

// A [tool.uv] written as an inline table cannot hold the comments that mark
// the pins, so they are refused, rather than written as lines nothing tells
// from the project's. (Astronomer's index cannot be written into one either,
// which SetAstroBuild already refuses, so the build here is none.)
func TestPinsAreRefusedWhereTheyCannotBeMarked(t *testing.T) {
	src := strings.Replace(astroHead, "[tool.astro]\n", "[tool.astro]\n\n[tool]\nuv = { constraint-dependencies = ['numpy<3'] }\n", 1)
	dir := projectWith(t, src)
	refused, err := SetAstroBuild(dir, nil, runtimeversions.AstroBuild{}, freeze339)
	require.NoError(t, err)
	assert.True(t, refused)
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Contains(t, string(out), "numpy<3")
	assert.NotContains(t, string(out), "zipp")
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
		{name: "a patch pin takes the newest build of that release, as the Airflow pin does", manifest: project("'apache-airflow==3.3.1'", "", ">=3.12"), catalog: c, build: "3.3-8", py: "3.13"},
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
