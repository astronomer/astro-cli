package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// pinFixture is a manifest a person wrote and commented, pinned the way init
// pins an Airflow 2.9 project.
const pinFixture = `# the orders team's project
[project]
name = 'orders'
requires-python = '>=3.10,<3.13'
dependencies = [
    # Airflow itself: upgrade with the team
    'apache-airflow[celery]==2.9.*',
    'pandas>=2', # for the reports Dag
]

[tool.astro]
packages = ['libpq-dev']

[tool.astro.deployments.prod]
url = 'https://airflow.example.com'
auth = { method = 'none' }
`

// leftoverFixture is pinFixture as init wrote it before the requirement was
// the only place the version lives: with a [tool.astro] airflow line beside it.
var leftoverFixture = strings.Replace(pinFixture, "[tool.astro]\n", "[tool.astro]\nairflow = '2.9' # upgrade with the team\n", 1)

func TestSetAirflowVersionRewritesTheRequirementSurgically(t *testing.T) {
	dir, path := writeEditFixture(t, pinFixture, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)

	want := strings.NewReplacer(
		"requires-python = '>=3.10,<3.13'", "requires-python = '>=3.12'",
		"'apache-airflow[celery]==2.9.*'", "'apache-airflow[celery]==3.2.*'",
	).Replace(pinFixture)
	assert.Equal(t, want, readFile(t, path), "anything but the requirement and its Python bound changed")
	assert.Equal(t, AirflowPinChange{
		Previous:       "2.9",
		Version:        "3.2",
		Changed:        true,
		Requirements:   []string{"apache-airflow[celery]==3.2.*"},
		RequiresPython: ">=3.12",
	}, change)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "3.2", m.Airflow().Pin)
}

// Moving off 3.1 moves the Python floor init wrote for it to the one the
// runtime offers from 3.2 on.
func TestSetAirflowVersionRaisesThePythonFloorPastThreePointOne(t *testing.T) {
	dir, path := writeEditFixture(t,
		"[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n",
		0o644)

	change, err := SetAirflowVersion(dir, nil, "3.3")
	require.NoError(t, err)
	assert.Equal(t, ">=3.12", change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.12'")
}

func TestSetAirflowVersionPinsAFullVersionExactly(t *testing.T) {
	dir, path := writeEditFixture(t, pinFixture, 0o644)

	_, err := SetAirflowVersion(dir, nil, "2.10.5")
	require.NoError(t, err)

	got := readFile(t, path)
	assert.Contains(t, got, "'apache-airflow[celery]==2.10.5',")
	assert.Contains(t, got, "requires-python = '>=3.10,<3.13'", "a bound both pins derive changed")
}

// The pin is checked before the manifest is read, so a bad one never reaches
// the wrapper, where the desktop takes its write lock.
func TestSetAirflowVersionRefusesAnInvalidPin(t *testing.T) {
	for _, v := range []string{"", "3.x", "3.1.2.4", "v3", " 3.1", "latest"} {
		t.Run(v, func(t *testing.T) {
			dir, path := writeEditFixture(t, pinFixture, 0o644)
			wrapped := false
			wrap := func(run func() error) error {
				wrapped = true
				return run()
			}

			_, err := SetAirflowVersion(dir, wrap, v)

			require.ErrorIs(t, err, ErrInvalidAirflowVersion)
			assert.False(t, wrapped, "the wrapper ran for a pin that was refused")
			assert.Equal(t, pinFixture, readFile(t, path), "a refused pin changed the file")
		})
	}
}

func TestSetAirflowVersionRunsInsideTheWrapper(t *testing.T) {
	dir, path := writeEditFixture(t, pinFixture, 0o644)
	var during string
	wrap := func(run func() error) error {
		err := run()
		during = readFile(t, path)
		return err
	}

	_, err := SetAirflowVersion(dir, wrap, "3.1")
	require.NoError(t, err)
	assert.Contains(t, during, "'apache-airflow[celery]==3.1.*'", "the write happened outside the wrapper")

	// The wrapper's own error comes back, and a wrapper that refuses to run the
	// edit leaves the file alone.
	dir, path = writeEditFixture(t, pinFixture, 0o644)
	locked := errors.New("locked")
	_, err = SetAirflowVersion(dir, func(func() error) error { return locked }, "3.1")
	require.ErrorIs(t, err, locked)
	assert.Equal(t, pinFixture, readFile(t, path))
}

func TestSetAirflowVersionKeepsTheFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits to keep")
	}
	dir, path := writeEditFixture(t, pinFixture, 0o600)

	_, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// A requirement that already pins the version is left as written, however it
// is spaced or quoted, so the same pin writes nothing.
func TestSetAirflowVersionWritesNothingForTheSamePin(t *testing.T) {
	cases := []struct{ pin, entry string }{
		{"2.9", `'apache-airflow[celery]==2.9.*'`},
		{"2.9", `"apache-airflow[celery]==2.9.*"`},
		{"3.1", `'apache-airflow == 3.1.*'`},
		{"3.1", `"apache-airflow==3.1.* ;python_version>='3.10'"`},
		{"3.3", `'apache-airflow-core==3.3.*'`},
	}
	for _, c := range cases {
		t.Run(c.entry, func(t *testing.T) {
			body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", c.entry, 1)
			dir, path := writeEditFixture(t, body, 0o644)
			past := time.Now().Add(-time.Hour).Truncate(time.Second)
			require.NoError(t, os.Chtimes(path, past, past))

			change, err := SetAirflowVersion(dir, nil, c.pin)
			require.NoError(t, err)

			assert.False(t, change.Changed)
			assert.Empty(t, change.Requirements)
			assert.Equal(t, body, readFile(t, path))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.True(t, info.ModTime().Equal(past), "an unchanged pin rewrote the file")
		})
	}
}

// The requirement is the version, so setting a version makes the requirement
// say exactly that: a patch under the series moves to the series, and a series
// narrows to a patch.
func TestSetAirflowVersionMakesTheRequirementSayTheVersion(t *testing.T) {
	cases := []struct{ entry, to, want string }{
		{"'apache-airflow==2.9.1'", "2.9", "'apache-airflow==2.9.*'"},
		{"'apache-airflow==2.9.1'", "3.1", "'apache-airflow==3.1.*'"},
		{"'apache-airflow[celery]==2.9.*'", "2", "'apache-airflow[celery]==2.*'"},
		{"'apache-airflow == 2.9.*'", "2.9.3", "'apache-airflow==2.9.3'"},
		// ==3.1 is exactly 3.1.0 under PEP 440, so it is not already the series.
		{"'apache-airflow==3.1'", "3.1", "'apache-airflow==3.1.*'"},
	}
	for _, c := range cases {
		t.Run(c.entry+" to "+c.to, func(t *testing.T) {
			body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", c.entry, 1)
			dir, path := writeEditFixture(t, body, 0o644)

			change, err := SetAirflowVersion(dir, nil, c.to)
			require.NoError(t, err)

			assert.Contains(t, readFile(t, path), c.want+",")
			assert.Equal(t, []string{strings.Trim(c.want, "'")}, change.Requirements)
			assert.True(t, change.Changed)
			m, err := manifest.Load(path)
			require.NoError(t, err)
			assert.Equal(t, c.to, m.Airflow().Pin)
		})
	}
}

// Every Airflow entry moves, so per-marker entries stay one pin, and a core
// entry stays core.
func TestSetAirflowVersionMovesEveryAirflowEntry(t *testing.T) {
	body := strings.Replace(pinFixture, "    'apache-airflow[celery]==2.9.*',\n",
		"    \"apache-airflow-core==2.9.*; sys_platform == 'linux'\",\n    \"apache-airflow-core==2.9.*; sys_platform != 'linux'\",\n", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.3")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"apache-airflow-core==3.3.*; sys_platform == 'linux'",
		"apache-airflow-core==3.3.*; sys_platform != 'linux'",
	}, change.Requirements)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "3.3", m.Airflow().Pin)
}

func TestSetAirflowVersionKeepsAMarker(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", `"apache-airflow==2.9.3 ; python_version < '3.13'"`, 1)
	dir, path := writeEditFixture(t, body, 0o644)

	_, err := SetAirflowVersion(dir, nil, "2.10")
	require.NoError(t, err)

	assert.Contains(t, readFile(t, path), `"apache-airflow==2.10.*; python_version < '3.13'"`)
}

// A requires-python someone chose is not the one this package derives, so it
// stays whatever the pin moves to.
func TestSetAirflowVersionKeepsAChosenPythonBound(t *testing.T) {
	body := strings.Replace(pinFixture, "'>=3.10,<3.13'", "'>=3.11,<3.13'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)

	assert.Contains(t, readFile(t, path), "requires-python = '>=3.11,<3.13'")
	assert.Empty(t, change.RequiresPython)
}

// A declared Dockerfile is the build in docker mode, so the pin no longer picks
// the image. The requirement is still written, the Dockerfile is not touched,
// and the change names it so the caller can say its FROM line is the user's to
// move.
func TestSetAirflowVersionReportsADeclaredDockerfile(t *testing.T) {
	body := strings.Replace(pinFixture, "packages = ['libpq-dev']", "dockerfile = 'Dockerfile'", 1)
	dir, path := writeEditFixture(t, body, 0o644)
	dockerfile := "FROM astrocrpublic.azurecr.io/runtime:2.9-1\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600))

	change, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)

	assert.Equal(t, "Dockerfile", change.Dockerfile)
	assert.Contains(t, readFile(t, path), "'apache-airflow[celery]==3.1.*'")
	assert.Equal(t, dockerfile, readFile(t, filepath.Join(dir, "Dockerfile")))
}

func TestSetAirflowVersionNeverCreatesAManifest(t *testing.T) {
	dir := t.TempDir()

	_, err := SetAirflowVersion(dir, nil, "3.1")

	require.ErrorIs(t, err, manifest.ErrNotFound)
	_, statErr := os.Stat(filepath.Join(dir, manifest.Marker))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// A manifest carrying the leftover [tool.astro] airflow line does not load,
// and SetAirflowVersion is one of the two edits that repair it: it runs on
// that manifest, deletes the line, and writes nothing else of it.
func TestSetAirflowVersionDeletesALeftoverAirflowKey(t *testing.T) {
	for _, to := range []string{"2.9", "3.2"} {
		t.Run(to, func(t *testing.T) {
			dir, path := writeEditFixture(t, leftoverFixture, 0o644)
			_, err := manifest.Load(path)
			require.Error(t, err, "the fixture should not load as it stands")

			change, err := SetAirflowVersion(dir, nil, to)
			require.NoError(t, err)

			assert.True(t, change.RemovedAirflowKey)
			assert.True(t, change.Changed)
			got := readFile(t, path)
			assert.NotContains(t, got, "airflow = ")
			m, err := manifest.Load(path)
			require.NoError(t, err, "the repaired manifest does not load:\n%s", got)
			assert.Equal(t, to, m.Airflow().Pin)
		})
	}

	// The same version is still a change: the line goes, and only the line.
	dir, path := writeEditFixture(t, leftoverFixture, 0o644)
	change, err := SetAirflowVersion(dir, nil, "2.9")
	require.NoError(t, err)
	assert.Empty(t, change.Requirements)
	assert.Equal(t, pinFixture, readFile(t, path))
}

func TestRepinAirflow(t *testing.T) {
	cases := []struct {
		spec, version, want string
		ok                  bool
	}{
		{"apache-airflow==2.9.*", "3.1", "apache-airflow==3.1.*", true},
		{"apache-airflow==2.9.3", "3", "apache-airflow==3.*", true},
		{"Apache_Airflow[celery,statsd] == 2.9.3", "3.1.2", "Apache_Airflow[celery,statsd]==3.1.2", true},
		{"apache-airflow==2.9.3;python_version<'3.13'", "2.10", "apache-airflow==2.10.*; python_version<'3.13'", true},
		{"apache-airflow-core==3.2.*", "3.3", "apache-airflow-core==3.3.*", true},
		// Whatever the entry pinned before, it now pins version.
		{"apache-airflow>=2.9", "3.1", "apache-airflow==3.1.*", true},
		{"apache-airflow", "3.1", "apache-airflow==3.1.*", true},
		{"apache-airflow[celery] >=2.9,<3 ; os_name == 'posix'", "3.1", "apache-airflow[celery]==3.1.*; os_name == 'posix'", true},
		{"apache-airflow @ https://example.com/airflow-2.9.3.whl", "3.1", "apache-airflow==3.1.*", true},
		{"apache-airflow-core~=3.2", "3.3.2", "apache-airflow-core==3.3.2", true},
		{"apache-airflow-core[otel]==3.2.*", "2.10", "apache-airflow[otel]==2.10.*", true},
		{"apache-airflow-providers-amazon==8.0.0", "3.1", "", false},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			got, ok := repinAirflow(c.spec, c.version)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

// Beside a requirement, MigrateAirflowKey deletes the leftover line and nothing
// else, whether it named the requirement's series or another one. The
// requirement is what runs, and the key does not move it.
func TestMigrateAirflowKeyBesideARequirement(t *testing.T) {
	for name, key := range map[string]string{
		"same series":      "airflow = '2.9' # upgrade with the team\n",
		"different series": "airflow = \"3.3\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(pinFixture, "[tool.astro]\n", "[tool.astro]\n"+key, 1)
			dir, path := writeEditFixture(t, body, 0o644)

			got, err := MigrateAirflowKey(dir, nil)
			require.NoError(t, err)

			assert.Equal(t, AirflowKeyMigration{Removed: true}, got)
			assert.Equal(t, pinFixture, readFile(t, path))
			m, err := manifest.Load(path)
			require.NoError(t, err)
			assert.Equal(t, "2.9", m.Airflow().Pin)
		})
	}
}

func TestMigrateAirflowKeyLeavesAManifestWithoutOne(t *testing.T) {
	dir, path := writeEditFixture(t, pinFixture, 0o644)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, past, past))

	got, err := MigrateAirflowKey(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, AirflowKeyMigration{}, got)
	assert.Equal(t, pinFixture, readFile(t, path))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(past), "a manifest with nothing to remove was rewritten")
}

// keyOnlyFixture is a manifest as it was valid before the requirement became
// the version: [tool.astro] airflow and no Airflow requirement at all.
var keyOnlyFixture = strings.NewReplacer(
	"    # Airflow itself: upgrade with the team\n    'apache-airflow[celery]==2.9.*',\n", "",
	"[tool.astro]\n", "[tool.astro]\nairflow = '2.9'\n",
).Replace(pinFixture)

// With no requirement, the line was the project's only statement of its
// version, so the repair moves it into one rather than deleting it: deleting
// alone would leave a manifest that names no Airflow, which does not load
// either.
func TestMigrateAirflowKeyMovesALoneKeyIntoTheRequirement(t *testing.T) {
	dir, path := writeEditFixture(t, keyOnlyFixture, 0o644)
	_, err := manifest.Load(path)
	require.Error(t, err, "the fixture should not load as it stands")

	got, err := MigrateAirflowKey(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, AirflowKeyMigration{Removed: true, Requirements: []string{"apache-airflow==2.9.*"}}, got)
	out := readFile(t, path)
	assert.NotContains(t, out, "airflow = ")
	m, err := manifest.Load(path)
	require.NoError(t, err, "the migrated manifest does not load:\n%s", out)
	assert.Equal(t, "2.9", m.Airflow().Pin)
	assert.Equal(t, []string{"pandas>=2", "apache-airflow==2.9.*"}, m.Project.Dependencies)
}

// A lone key that is not a version has nothing to move, and is refused
// without a write.
func TestMigrateAirflowKeyRefusesALoneKeyThatIsNotAVersion(t *testing.T) {
	body := strings.Replace(keyOnlyFixture, "airflow = '2.9'", "airflow = 'latest'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	_, err := MigrateAirflowKey(dir, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "latest")
	assert.Equal(t, body, readFile(t, path))
}

// SetAirflowVersion repairs the same manifest, writing the requirement for the
// version it is given, and reports the key's version as the previous one.
func TestSetAirflowVersionRepairsALoneKey(t *testing.T) {
	dir, path := writeEditFixture(t, keyOnlyFixture, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)

	assert.Equal(t, "2.9", change.Previous)
	assert.True(t, change.RemovedAirflowKey)
	assert.Equal(t, []string{"apache-airflow==3.2.*"}, change.Requirements)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "3.2", m.Airflow().Pin)
}

// A key beside a requirement that states no single version is the project's
// version, so the migration moves every Airflow requirement to it, keeping
// names, extras and markers.
func TestMigrateAirflowKeyReplacesAnUnclearRequirement(t *testing.T) {
	for name, tc := range map[string]struct {
		entries string
		want    []string
	}{
		"a range": {
			"    'apache-airflow[celery]>=2.9',\n",
			[]string{"apache-airflow[celery]==2.9.*"},
		},
		"two pins that disagree, under markers": {
			"    \"apache-airflow==2.9.*; python_version >= '3.11'\",\n    \"apache-airflow==2.8.*; python_version < '3.11'\",\n",
			[]string{"apache-airflow==2.9.*; python_version < '3.11'"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(leftoverFixture, "    'apache-airflow[celery]==2.9.*',\n", tc.entries, 1)
			dir, path := writeEditFixture(t, body, 0o644)

			got, err := MigrateAirflowKey(dir, nil)
			require.NoError(t, err)

			assert.True(t, got.Removed)
			assert.Equal(t, tc.want, got.Requirements)
			m, err := manifest.Load(path)
			require.NoError(t, err, "the migrated manifest does not load:\n%s", readFile(t, path))
			assert.Equal(t, "2.9", m.Airflow().Pin)
		})
	}
}

// Both distributions is a choice of distribution, which an Airflow 3 version
// cannot make, so the migration is refused with the manifest's own reason and
// writes nothing. (For an Airflow 2 there is only one distribution, so the
// core entry becomes apache-airflow and the two agree.)
func TestMigrateAirflowKeyRefusesBothDistributions(t *testing.T) {
	body := strings.NewReplacer(
		"    'apache-airflow[celery]==2.9.*',\n", "    'apache-airflow>=3.1',\n    'apache-airflow-core>=3.1',\n",
		"airflow = '2.9'", "airflow = '3.2'",
	).Replace(leftoverFixture)
	dir, path := writeEditFixture(t, body, 0o644)

	_, err := MigrateAirflowKey(dir, nil)

	require.ErrorIs(t, err, ErrEditRefused)
	assert.Contains(t, err.Error(), "Keep one of them")
	assert.Equal(t, body, readFile(t, path))
}

// SetAirflowVersion is given the version, so it repairs any requirement that
// states none, with or without a leftover key.
func TestSetAirflowVersionRepairsAnUnclearRequirement(t *testing.T) {
	for name, body := range map[string]string{
		"a range, no key":            strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow[celery]>=2.9'", 1),
		"no requirement, no key":     strings.Replace(pinFixture, "    # Airflow itself: upgrade with the team\n    'apache-airflow[celery]==2.9.*',\n", "", 1),
		"a range and a leftover key": strings.Replace(leftoverFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow[celery]>=2.9'", 1),
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := writeEditFixture(t, body, 0o644)
			_, err := manifest.Load(path)
			require.Error(t, err, "the fixture should not load as it stands")

			change, err := SetAirflowVersion(dir, nil, "3.2")
			require.NoError(t, err)

			assert.True(t, change.Changed)
			m, err := manifest.Load(path)
			require.NoError(t, err, "the repaired manifest does not load:\n%s", readFile(t, path))
			assert.Equal(t, "3.2", m.Airflow().Pin)
		})
	}
}

// With no previous version to read, no requires-python can be told apart as
// the bound this package wrote for it, so it stays whatever it says.
func TestSetAirflowVersionKeepsRequiresPythonWithNoPreviousVersion(t *testing.T) {
	body := "[project]\nname = 'x'\nrequires-python = '>=3.12'\ndependencies = ['pandas']\n\n[tool.astro]\n"
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "2.9")
	require.NoError(t, err)

	assert.Empty(t, change.Previous)
	assert.Empty(t, change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.12'")
}

// dynamicFixture is a manifest the old adopt path wrote for a project whose
// dependencies a build backend supplies: [tool.astro] airflow, and dependencies
// declared dynamic, so there is nowhere to put the requirement.
const dynamicFixture = `[project]
name = 'orders'
version = '0.1.0'
dynamic = ['dependencies']

[tool.setuptools.dynamic]
dependencies = {file = ['requirements.txt']}

[tool.astro]
airflow = '2.9'
`

// A repair cannot add the requirement beside dynamic dependencies, which PEP
// 621 forbids and uv refuses, so both repairs are refused with the manifest's
// reason and write nothing.
func TestAirflowRepairsRefuseDynamicDependencies(t *testing.T) {
	for name, repair := range map[string]func(dir string) error{
		"MigrateAirflowKey": func(dir string) error { _, err := MigrateAirflowKey(dir, nil); return err },
		"SetAirflowVersion": func(dir string) error { _, err := SetAirflowVersion(dir, nil, "3.2"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := writeEditFixture(t, dynamicFixture, 0o644)

			err := repair(dir)

			var ve *manifest.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Contains(t, err.Error(), "drop dependencies from [project] dynamic")
			assert.Equal(t, dynamicFixture, readFile(t, path))
		})
	}
}

// apache-airflow-core is published only for Airflow 3, so moving a core
// project to an Airflow 2 switches the entry to apache-airflow, keeping its
// extras and marker, and says so. An Airflow 3 target leaves it core.
func TestSetAirflowVersionSwitchesCoreForAnAirflow2(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", `"apache-airflow-core[otel]==3.2.*; os_name == 'posix'"`, 1)

	dir, path := writeEditFixture(t, body, 0o644)
	change, err := SetAirflowVersion(dir, nil, "2.10")
	require.NoError(t, err)
	assert.True(t, change.CoreReplaced)
	assert.Equal(t, []string{"apache-airflow[otel]==2.10.*; os_name == 'posix'"}, change.Requirements)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "2.10", m.Airflow().Pin)

	dir, _ = writeEditFixture(t, body, 0o644)
	change, err = SetAirflowVersion(dir, nil, "3.3")
	require.NoError(t, err)
	assert.False(t, change.CoreReplaced)
	assert.Equal(t, []string{"apache-airflow-core[otel]==3.3.*; os_name == 'posix'"}, change.Requirements)
}

// A core entry already pinned to an Airflow 2 is a problem of its own, and
// setting the version it names is its repair: the entry switches even though
// its version was already right.
func TestSetAirflowVersionRepairsCorePinnedToAnAirflow2(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow-core==2.9.*'", 1)
	dir, path := writeEditFixture(t, body, 0o644)
	_, err := manifest.Load(path)
	require.Error(t, err, "the fixture should not load as it stands")

	change, err := SetAirflowVersion(dir, nil, "2.9")
	require.NoError(t, err)

	assert.True(t, change.CoreReplaced)
	assert.Equal(t, []string{"apache-airflow==2.9.*"}, change.Requirements)
	_, err = manifest.Load(path)
	require.NoError(t, err)
}

// A core entry pinned to an Airflow 2 states one clear version, only in the
// wrong distribution, so a repair reads that version as the previous one: the
// requires-python init wrote for it moves with the pin.
func TestSetAirflowVersionReadsCoreOnAirflow2AsThePreviousVersion(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow-core==2.9.*'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)

	assert.Equal(t, "2.9", change.Previous)
	assert.Equal(t, ">=3.12", change.RequiresPython)
	assert.Equal(t, []string{"apache-airflow-core==3.2.*"}, change.Requirements)
	_, err = manifest.Load(path)
	require.NoError(t, err)
}

// The leftover key does not overwrite a requirement that states its version,
// even when that requirement names the wrong distribution for it: the core
// entry moves to apache-airflow at its own version, and the key is deleted.
func TestMigrateAirflowKeyKeepsACoreRequirementsVersion(t *testing.T) {
	body := strings.NewReplacer(
		"'apache-airflow[celery]==2.9.*'", "'apache-airflow-core[celery]==2.9.*'",
		"airflow = '2.9'", "airflow = '2.10'",
	).Replace(leftoverFixture)
	dir, path := writeEditFixture(t, body, 0o644)

	got, err := MigrateAirflowKey(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, AirflowKeyMigration{Removed: true, Requirements: []string{"apache-airflow[celery]==2.9.*"}, CoreReplaced: true}, got)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "2.9", m.Airflow().Pin, "the key overwrote the requirement's version")
}

// The migration moves a core project's leftover Airflow 2 key the same way.
func TestMigrateAirflowKeySwitchesCoreForAnAirflow2(t *testing.T) {
	body := strings.Replace(leftoverFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow-core>=3.1'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	got, err := MigrateAirflowKey(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, AirflowKeyMigration{Removed: true, Requirements: []string{"apache-airflow==2.9.*"}, CoreReplaced: true}, got)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "2.9", m.Airflow().Pin)
}

// Removing the line is only a repair when the line is the manifest's only
// problem. With another one, the result would still not load, so nothing is
// written and the error names what is left.
func TestMigrateAirflowKeyRefusesAManifestWithOtherProblems(t *testing.T) {
	body := strings.Replace(leftoverFixture, "packages = ['libpq-dev']", "packages = ['libpq-dev']\npackagez = ['git']", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	_, err := MigrateAirflowKey(dir, nil)

	var ve *manifest.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Contains(t, err.Error(), "tool.astro.packagez")
	assert.Equal(t, body, readFile(t, path))
}

// An edit that changes nothing still has to answer for a manifest read only
// for repair: it reports the manifest's error rather than success, or a no-op
// write (re-declaring what is already declared) passed on an un-migrated
// project as if it loaded.
func TestEditManifestNoOpOnAnUnmigratedManifestIsAnError(t *testing.T) {
	dir, path := writeEditFixture(t, leftoverFixture, 0o644)

	err := EditManifest(dir, nil, func(*manifest.Manifest, tomledit.Editor) error { return nil })

	var ve *manifest.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Contains(t, err.Error(), "Delete the airflow line")
	assert.Equal(t, leftoverFixture, readFile(t, path))
}

// The carve-out is for the repair, not a way around the check: any other edit
// of a manifest with the leftover line is refused, naming the line, until the
// line is gone.
func TestEditManifestRefusesOtherEditsWhileTheAirflowKeyIsThere(t *testing.T) {
	dir, path := writeEditFixture(t, leftoverFixture, 0o644)

	err := EditManifest(dir, nil, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		return ed.Set([]string{"tool", "astro", "workspace"}, "ws-abc")
	})

	require.ErrorIs(t, err, ErrEditRefused)
	assert.Contains(t, err.Error(), "tool.astro.airflow")
	assert.Equal(t, leftoverFixture, readFile(t, path))
}
