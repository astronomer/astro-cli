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

// runtimeCatalog has two builds of 3.2, the newer of them yanked, three of
// 3.3, and two Airflow 2 lines.
func runtimeCatalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{
  "runtimeVersions": {
    "12.9.0": {"metadata": {"airflowVersion": "2.10.5", "channel": "stable", "releaseDate": "2025-06-01"}},
    "13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable", "releaseDate": "2026-09-17"}}
  },
  "runtimeVersionsV3": {
    "3.2-4": {"metadata": {"airflowVersion": "3.2.2", "channel": "stable", "releaseDate": "2026-06-01"}},
    "3.2-5": {"metadata": {"airflowVersion": "3.2.2", "channel": "stable", "releaseDate": "2026-07-01", "yanked": true}},
    "3.3-5": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-08-01"}},
    "3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-08-20"}},
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-09-23"}}
  }
}`))
	require.NoError(t, err)
	return c
}

func withRuntime(requirement, runtime string) string {
	return "[project]\nname = 'x'\ndependencies = [\n    '" + requirement + "',  # the version\n]\n\n[tool.astro]\nruntime = '" + runtime + "'  # pinned build\npackages = ['git']\n"
}

// Guard 3: a pin that leaves the build's series takes the build with it, to
// the newest non-yanked build of the new series when the catalog is at hand.
func TestSetAirflowVersionMovesTheRuntimeWithTheSeries(t *testing.T) {
	dir, path := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-8"), 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.2", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "3.2-4", change.Runtime, "3.2-5 is yanked")
	assert.False(t, change.RuntimeRemoved)
	assert.True(t, change.Changed)
	got := readFile(t, path)
	assert.Contains(t, got, "runtime = '3.2-4'  # pinned build", "the line is edited in place, comment kept")
	m, err := manifest.Parse([]byte(got))
	require.NoError(t, err)
	assert.Equal(t, manifest.Airflow{Pin: "3.2", Runtime: "3.2-4"}, m.Airflow())
}

// An exact pin takes the newest build carrying that exact Airflow, so the move
// does not trade a series mismatch for a patch warning.
func TestSetAirflowVersionMovesTheRuntimeToABuildCarryingAnExactPin(t *testing.T) {
	dir, _ := writeEditFixture(t, withRuntime("apache-airflow==3.2.*", "3.2-4"), 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.3.1", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "3.3-7", change.Runtime)
}

// Offline there is no build to vouch for, so the line goes, and the image
// builds from the newest build of the new series.
func TestSetAirflowVersionRemovesTheRuntimeWithoutACatalog(t *testing.T) {
	dir, path := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-8"), 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.2", AirflowPinOptions{})
	require.NoError(t, err)
	assert.True(t, change.RuntimeRemoved)
	assert.Empty(t, change.Runtime)
	got := readFile(t, path)
	assert.NotContains(t, got, "runtime")
	assert.Contains(t, got, "packages = ['git']")
	_, err = manifest.Parse([]byte(got))
	require.NoError(t, err)
}

// A catalog that lists no build of the new series is offline for this purpose.
func TestSetAirflowVersionRemovesTheRuntimeWhenTheCatalogHasNoBuild(t *testing.T) {
	dir, _ := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-8"), 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.1", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.True(t, change.RuntimeRemoved)
}

// Same series: a build the new pin still covers agrees, and is the user's
// choice. 3.3-5 carries 3.3.1, which ==3.3.1 and ==3.3.* both cover.
func TestSetAirflowVersionLeavesTheRuntimeWithinItsSeries(t *testing.T) {
	for _, version := range []string{"3.3.1", "3.3"} {
		dir, path := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-5"), 0o644)
		change, err := SetAirflowVersionWith(dir, nil, version, AirflowPinOptions{Catalog: runtimeCatalog(t)})
		require.NoError(t, err, version)
		assert.Empty(t, change.Runtime, version)
		assert.False(t, change.RuntimeRemoved, version)
		assert.Contains(t, readFile(t, path), "runtime = '3.3-5'", version)
	}
}

// An exact pin the build does not carry moves it, when the catalog says so, to
// a build carrying that pin: left alone, it would warn at every image build.
// Without a catalog only the tag is known, and the tag agrees.
func TestSetAirflowVersionMovesARuntimeAnExactPinExcludes(t *testing.T) {
	dir, path := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-5"), 0o644)
	change, err := SetAirflowVersionWith(dir, nil, "3.3.2", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "3.3-8", change.Runtime)
	assert.Contains(t, readFile(t, path), "runtime = '3.3-8'")

	dir, path = writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-5"), 0o644)
	change, err = SetAirflowVersionWith(dir, nil, "3.3.2", AirflowPinOptions{})
	require.NoError(t, err)
	assert.Empty(t, change.Runtime)
	assert.False(t, change.RuntimeRemoved)
	assert.Contains(t, readFile(t, path), "runtime = '3.3-5'")
}

// An Airflow 2 build moves by what the catalog says it carries: to Airflow 3,
// and to another Airflow 2 series, which its tag cannot show. Offline the
// generation is all the tag says, so it stays.
func TestSetAirflowVersionMovesAnAirflow2Runtime(t *testing.T) {
	dir, _ := writeEditFixture(t, withRuntime("apache-airflow==2.11.*", "13.11.0"), 0o644)
	change, err := SetAirflowVersionWith(dir, nil, "3.3", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "3.3-8", change.Runtime)

	dir, path := writeEditFixture(t, withRuntime("apache-airflow==2.11.*", "13.11.0"), 0o644)
	change, err = SetAirflowVersionWith(dir, nil, "2.10", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "12.9.0", change.Runtime, "13.11.0 carries 2.11.2, which 2.10 does not cover")
	assert.Contains(t, readFile(t, path), "runtime = '12.9.0'")

	dir, path = writeEditFixture(t, withRuntime("apache-airflow==2.11.*", "13.11.0"), 0o644)
	change, err = SetAirflowVersionWith(dir, nil, "2.10", AirflowPinOptions{})
	require.NoError(t, err)
	assert.Empty(t, change.Runtime)
	assert.False(t, change.RuntimeRemoved)
	assert.Contains(t, readFile(t, path), "runtime = '13.11.0'")
}

// A manifest whose runtime no longer loads is one SetAirflowVersionWith can
// read and repair, like the other version problems.
func TestSetAirflowVersionRepairsAMismatchedRuntime(t *testing.T) {
	dir, _ := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.2-4"), 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.3", AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "3.3-8", change.Runtime)
}

func TestAlignRuntime(t *testing.T) {
	cases := []struct {
		name, body  string
		catalog     bool
		wantRuntime string
		wantRemoved bool
		wantChanged bool
	}{
		{name: "an agreeing build is left", body: withRuntime("apache-airflow==3.3.*", "3.3-5"), catalog: true, wantRuntime: "3.3-5"},
		{name: "another series moves", body: withRuntime("apache-airflow==3.3.*", "3.2-4"), catalog: true, wantRuntime: "3.3-8", wantChanged: true},
		{name: "another series offline goes", body: withRuntime("apache-airflow==3.3.*", "3.2-4"), wantRemoved: true, wantChanged: true},
		{name: "a malformed tag moves", body: withRuntime("apache-airflow==3.3.*", "newest"), catalog: true, wantRuntime: "3.3-8", wantChanged: true},
		{name: "a floating tag moves", body: withRuntime("apache-airflow==3.3.*", "3.3"), catalog: true, wantRuntime: "3.3-8", wantChanged: true},
		{
			name: "beside a dockerfile it goes", body: withRuntime("apache-airflow==3.3.*", "3.3-8") + "dockerfile = 'Dockerfile'\n",
			catalog: true, wantRemoved: true, wantChanged: true,
		},
		{name: "no runtime", body: "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n", catalog: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := writeEditFixture(t, tc.body, 0o644)
			var opts AirflowPinOptions
			if tc.catalog {
				opts.Catalog = runtimeCatalog(t)
			}
			change, err := AlignRuntime(dir, nil, opts)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRuntime, change.Runtime)
			assert.Equal(t, tc.wantRemoved, change.Removed)
			assert.Equal(t, tc.wantChanged, change.Changed)
			got := readFile(t, path)
			if tc.wantChanged {
				_, err := manifest.Parse([]byte(got))
				require.NoError(t, err, "the repair loads")
				assert.Contains(t, got, "'apache-airflow==3.3.*',  # the version", "the requirement is not touched")
			} else {
				assert.Equal(t, tc.body, got)
			}
		})
	}
}

func TestAlignRuntimeRefusesAnUnclearRequirement(t *testing.T) {
	dir, _ := writeEditFixture(t, withRuntime("apache-airflow>=3.1", "3.3-8"), 0o644)
	_, err := AlignRuntime(dir, nil, AirflowPinOptions{Catalog: runtimeCatalog(t)})
	require.ErrorIs(t, err, errAirflowUnclear)
}

func TestSetRuntime(t *testing.T) {
	dir, path := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.3-5"), 0o644)

	change, err := SetRuntime(dir, nil, "3.3-8")
	require.NoError(t, err)
	assert.Equal(t, RuntimeChange{Previous: "3.3-5", Runtime: "3.3-8", Changed: true}, change)
	assert.Contains(t, readFile(t, path), "runtime = '3.3-8'  # pinned build")

	// Another series is the manifest's to refuse, and nothing is written.
	before := readFile(t, path)
	_, err = SetRuntime(dir, nil, "3.2-4")
	require.ErrorIs(t, err, ErrEditRefused)
	var ve *manifest.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, manifest.CodeRuntimeMismatch, ve.Problems[0].Code)
	assert.Equal(t, before, readFile(t, path))

	change, err = SetRuntime(dir, nil, "")
	require.NoError(t, err)
	assert.True(t, change.Removed)
	assert.NotContains(t, readFile(t, path), "runtime")
}

func TestSetRuntimeAddsALine(t *testing.T) {
	dir, path := writeEditFixture(t, "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n", 0o644)
	change, err := SetRuntime(dir, nil, "3.3-8")
	require.NoError(t, err)
	assert.True(t, change.Changed)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "3.3-8", m.Airflow().Runtime)
}

// declaredDockerfileProject writes a project declaring docker/Dockerfile with body.
func declaredDockerfileProject(t *testing.T, requirement, body string) (dir, pyproject string) {
	t.Helper()
	dir, pyproject = writeEditFixture(t,
		"[project]\nname = 'x'  # keep\nrequires-python = '>=3.12'\ndependencies = [\n    '"+requirement+"',\n    'pandas',\n]\n\n[tool.astro]\ndockerfile = 'docker/Dockerfile'\n",
		0o644)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docker"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker", "Dockerfile"), []byte(body), 0o600))
	return dir, pyproject
}

func TestCheckDockerfileAirflow(t *testing.T) {
	cases := []struct {
		name, requirement, from string
		want                    bool
	}{
		{name: "airflow 3 same series", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.3-8"},
		{name: "airflow 3 another series", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.2-4", want: true},
		{name: "a python flavor is read through", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.2-4-python-3.12", want: true},
		{name: "final stage decides", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.2-4 AS b\nFROM astrocrpublic.azurecr.io/runtime:3.3-8", want: false},
		{name: "airflow 2 runtime beside airflow 3", requirement: "apache-airflow==3.3.*", from: "FROM quay.io/astronomer/astro-runtime:13.11.0", want: true},
		{name: "airflow 3 runtime beside airflow 2", requirement: "apache-airflow==2.11.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.3-8", want: true},
		{name: "airflow 2 beside airflow 2", requirement: "apache-airflow==2.10.*", from: "FROM quay.io/astronomer/astro-runtime:13.11.0"},
		{name: "a build argument", requirement: "apache-airflow==3.3.*", from: "ARG BASE=astrocrpublic.azurecr.io/runtime:3.2-4\nFROM ${BASE}"},
		{name: "a digest pin", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime@sha256:0123abcd"},
		{name: "not a runtime image", requirement: "apache-airflow==3.3.*", from: "FROM apache/airflow:2.10.5"},
		{name: "untagged", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, pyproject := declaredDockerfileProject(t, tc.requirement, tc.from+"\nRUN echo hi\n")
			m, err := manifest.Load(pyproject)
			require.NoError(t, err)
			err = CheckDockerfileAirflow(dir, m)
			if !tc.want {
				require.NoError(t, err)
				return
			}
			var ve *manifest.ValidationError
			require.ErrorAs(t, err, &ve)
			require.Len(t, ve.Problems, 1)
			assert.Equal(t, manifest.CodeDockerfileAirflowMismatch, ve.Problems[0].Code)
			assert.Equal(t, pyproject, ve.Path)
			assert.Contains(t, err.Error(), "docker/Dockerfile")
			assert.Contains(t, err.Error(), tc.requirement)
		})
	}
}

func TestCheckDockerfileAirflowPassesWithoutADockerfile(t *testing.T) {
	m, err := manifest.Parse([]byte("[project]\nname = 'x'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n"))
	require.NoError(t, err)
	require.NoError(t, CheckDockerfileAirflow(t.TempDir(), m))
	// A declared file that is not there is the build's to report.
	m, err = manifest.Parse([]byte("[project]\nname = 'x'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n"))
	require.NoError(t, err)
	require.NoError(t, CheckDockerfileAirflow(t.TempDir(), m))
}

// The fix rewrites the requirement and nothing else: every other byte of the
// manifest, and the Dockerfile, are as they were, and the result passes the
// check it fixes.
func TestMatchAirflowToDockerfileRoundTrip(t *testing.T) {
	cases := []struct {
		name, requirement, from, want string
		catalog                       bool
		// python is requires-python afterwards. The fixture's >=3.12 is the
		// bound this package writes for an Airflow 3.2 or 3.3 pin, so it moves
		// with the pin to an Airflow 2 as SetAirflowVersionWith moves it, and
		// is the user's (and stays) beside an Airflow 2 pin.
		python string
	}{
		{name: "airflow 3 series", requirement: "apache-airflow==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.2-4", want: "apache-airflow==3.2.*"},
		{name: "an exact pin becomes the series", requirement: "apache-airflow==3.3.1", from: "FROM astrocrpublic.azurecr.io/runtime:3.2-4", want: "apache-airflow==3.2.*"},
		{name: "core keeps its name", requirement: "apache-airflow-core==3.3.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.2-4", want: "apache-airflow-core==3.2.*"},
		{
			name: "airflow 2 offline: the generation", requirement: "apache-airflow==3.3.*", from: "FROM quay.io/astronomer/astro-runtime:13.11.0",
			want: "apache-airflow==2.*", python: ">=3.10,<3.13",
		},
		{
			name: "airflow 2 with the catalog: its series", requirement: "apache-airflow==3.3.*", from: "FROM quay.io/astronomer/astro-runtime:13.11.0",
			want: "apache-airflow==2.11.*", catalog: true, python: ">=3.10,<3.13",
		},
		{name: "airflow 3 from airflow 2", requirement: "apache-airflow==2.11.*", from: "FROM astrocrpublic.azurecr.io/runtime:3.3-8", want: "apache-airflow==3.3.*"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dockerfile := tc.from + "\nRUN echo hi\n"
			dir, pyproject := declaredDockerfileProject(t, tc.requirement, dockerfile)
			before := readFile(t, pyproject)
			var opts AirflowPinOptions
			if tc.catalog {
				opts.Catalog = runtimeCatalog(t)
			}

			change, err := MatchAirflowToDockerfile(dir, nil, opts)
			require.NoError(t, err)
			assert.True(t, change.Changed)
			assert.Equal(t, []string{tc.want}, change.Requirements)
			assert.Equal(t, "docker/Dockerfile", change.Dockerfile)

			after := readFile(t, pyproject)
			want := strings.Replace(before, tc.requirement, tc.want, 1)
			if tc.python != "" {
				want = strings.Replace(want, "requires-python = '>=3.12'", "requires-python = '"+tc.python+"'", 1)
			}
			assert.Equal(t, want, after, "only the requirement, and a requires-python this package wrote, change")
			assert.Equal(t, dockerfile, readFile(t, filepath.Join(dir, "docker", "Dockerfile")))
			m, err := manifest.Load(pyproject)
			require.NoError(t, err)
			require.NoError(t, CheckDockerfileAirflow(dir, m), "the fix passes the check it fixes")
		})
	}
}

func TestMatchAirflowToDockerfileLeavesAnAgreeingRequirement(t *testing.T) {
	dir, pyproject := declaredDockerfileProject(t, "apache-airflow==3.3.1", "FROM astrocrpublic.azurecr.io/runtime:3.3-8\n")
	before := readFile(t, pyproject)
	change, err := MatchAirflowToDockerfile(dir, nil, AirflowPinOptions{})
	require.NoError(t, err)
	assert.False(t, change.Changed)
	assert.Equal(t, "3.3.1", change.Version)
	assert.Equal(t, before, readFile(t, pyproject))
}

func TestMatchAirflowToDockerfileRefusesAFromItCannotRead(t *testing.T) {
	for _, from := range []string{
		"ARG BASE=x\nFROM ${BASE}",
		"FROM astrocrpublic.azurecr.io/runtime@sha256:0123abcd",
		"FROM python:3.12-slim",
	} {
		dir, _ := declaredDockerfileProject(t, "apache-airflow==3.3.*", from+"\n")
		_, err := MatchAirflowToDockerfile(dir, nil, AirflowPinOptions{})
		assert.True(t, errors.Is(err, ErrNoDockerfileAirflow), "%s: %v", from, err)
	}
	dir, _ := writeEditFixture(t, "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n", 0o644)
	_, err := MatchAirflowToDockerfile(dir, nil, AirflowPinOptions{})
	assert.ErrorIs(t, err, ErrNoDockerfileAirflow)
}
