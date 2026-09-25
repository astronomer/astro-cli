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
)

// pinFixture is a manifest a person wrote and commented, pinned the way init
// pins an Airflow 2.9 project.
const pinFixture = `# the orders team's project
[project]
name = 'orders'
requires-python = '>=3.10,<3.13'
dependencies = [
    # Airflow itself, kept in step with the pin
    'apache-airflow[celery]==2.9.*',
    'pandas>=2', # for the reports Dag
]

[tool.astro]
airflow = '2.9' # upgrade with the team
packages = ['libpq-dev']

[tool.astro.deployments.prod]
url = 'https://airflow.example.com'
auth = { method = 'none' }
`

func TestSetAirflowVersionRewritesThePinSurgically(t *testing.T) {
	dir, path := writeEditFixture(t, pinFixture, 0o644)

	// 3.2 rather than 3.1, which would also add the SQLAlchemy cap: that has
	// tests of its own in airflowpin_cap_test.go.
	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)

	want := strings.NewReplacer(
		"requires-python = '>=3.10,<3.13'", "requires-python = '>=3.12'",
		"'apache-airflow[celery]==2.9.*'", "'apache-airflow[celery]==3.2.*'",
		"airflow = '2.9' # upgrade", "airflow = '3.2' # upgrade",
	).Replace(pinFixture)
	assert.Equal(t, want, readFile(t, path), "anything but the pin, its requirement and its Python bound changed")
	assert.Equal(t, AirflowPinChange{
		Previous:       "2.9",
		Version:        "3.2",
		Changed:        true,
		Requirements:   []string{"apache-airflow[celery]==3.2.*"},
		RequiresPython: ">=3.12",
	}, change)
	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "3.2", m.Astro.AirflowVersion)
}

// Moving off 3.1 moves the Python floor init wrote for it to the one the
// runtime offers from 3.2 on.
func TestSetAirflowVersionRaisesThePythonFloorPastThreePointOne(t *testing.T) {
	dir, path := writeEditFixture(t,
		"[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nairflow = '3.1'\n",
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
	assert.Contains(t, got, "airflow = '2.10.5' # upgrade with the team")
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
	assert.Contains(t, during, "airflow = '3.1'", "the write happened outside the wrapper")

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

// Setting a double-quoted value, even to what it already says, rewrites it
// single-quoted, so the same pin in either quoting has to leave the file alone.
func TestSetAirflowVersionWritesNothingForTheSamePin(t *testing.T) {
	for name, body := range map[string]string{
		"single-quoted": pinFixture,
		"double-quoted": strings.Replace(pinFixture, "airflow = '2.9'", `airflow = "2.9"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := writeEditFixture(t, body, 0o644)
			past := time.Now().Add(-time.Hour).Truncate(time.Second)
			require.NoError(t, os.Chtimes(path, past, past))

			change, err := SetAirflowVersion(dir, nil, "2.9")
			require.NoError(t, err)

			assert.False(t, change.Changed)
			assert.Equal(t, body, readFile(t, path))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.True(t, info.ModTime().Equal(past), "an unchanged pin rewrote the file")
		})
	}
}

// An apache-airflow entry pinned as a range says something a single version
// cannot, so it stays, and the change says so rather than dropping it.
func TestSetAirflowVersionLeavesARequirementItCannotRead(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow>=2.9,<3'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)

	assert.Contains(t, readFile(t, path), "'apache-airflow>=2.9,<3'")
	assert.Equal(t, []string{"apache-airflow>=2.9,<3"}, change.UnreadRequirements)
	assert.Empty(t, change.Requirements)
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
// the image. The pin is still written, the Dockerfile is not touched, and the
// change names it so the caller can say its FROM line is the user's to move.
func TestSetAirflowVersionReportsADeclaredDockerfile(t *testing.T) {
	body := strings.Replace(pinFixture, "packages = ['libpq-dev']", "dockerfile = 'Dockerfile'", 1)
	dir, path := writeEditFixture(t, body, 0o644)
	dockerfile := "FROM astrocrpublic.azurecr.io/runtime:2.9-1\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600))

	change, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)

	assert.Equal(t, "Dockerfile", change.Dockerfile)
	assert.Contains(t, readFile(t, path), "airflow = '3.1'")
	assert.Equal(t, dockerfile, readFile(t, filepath.Join(dir, "Dockerfile")))
}

func TestSetAirflowVersionNeverCreatesAManifest(t *testing.T) {
	dir := t.TempDir()

	_, err := SetAirflowVersion(dir, nil, "3.1")

	require.ErrorIs(t, err, manifest.ErrNotFound)
	_, statErr := os.Stat(filepath.Join(dir, manifest.Marker))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
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
		{"apache-airflow>=2.9", "3.1", "", false},
		{"apache-airflow", "3.1", "", false},
		{"apache-airflow @ https://example.com/airflow-2.9.3.whl", "3.1", "", false},
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

// A requirement the unchanged pin already covers is left as written, however it
// is spaced and however narrowly it pins, so the same pin writes nothing.
func TestSetAirflowVersionLeavesARequirementThePinCovers(t *testing.T) {
	cases := []struct{ pin, entry string }{
		{"3.1", `'apache-airflow == 3.1.*'`},
		{"3.1", `"apache-airflow==3.1.* ;python_version>='3.10'"`},
		{"2.9", `'apache-airflow==2.9.1'`},
		{"3.1", `'apache-airflow==3.1'`},
	}
	for _, c := range cases {
		t.Run(c.entry, func(t *testing.T) {
			body := strings.NewReplacer(
				"'apache-airflow[celery]==2.9.*'", c.entry,
				"airflow = '2.9'", "airflow = '"+c.pin+"'",
			).Replace(pinFixture)
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

// A pin that widens still takes its own requirement with it: the new pin covers
// the old series, but leaving it would keep installing that series.
func TestSetAirflowVersionMovesThePreviousPinsRequirementWhenItWidens(t *testing.T) {
	cases := []struct{ from, entry, to, want string }{
		{"2.9", "'apache-airflow[celery]==2.9.*'", "2", "'apache-airflow[celery]==2.*'"},
		{"2.9", "'apache-airflow == 2.9.*'", "2", "'apache-airflow==2.*'"},
		{"3.1.2", "'apache-airflow==3.1.2'", "3.1", "'apache-airflow==3.1.*'"},
	}
	for _, c := range cases {
		t.Run(c.from+" to "+c.to+" "+c.entry, func(t *testing.T) {
			body := strings.NewReplacer(
				"'apache-airflow[celery]==2.9.*'", c.entry,
				"airflow = '2.9'", "airflow = '"+c.from+"'",
			).Replace(pinFixture)
			dir, path := writeEditFixture(t, body, 0o644)

			change, err := SetAirflowVersion(dir, nil, c.to)
			require.NoError(t, err)

			got := readFile(t, path)
			assert.Contains(t, got, c.want+",")
			assert.Contains(t, got, "airflow = '"+c.to+"'")
			assert.Equal(t, []string{strings.Trim(c.want, "'")}, change.Requirements)
			assert.True(t, change.Changed)
		})
	}
}

// A patch someone chose under the old series is not the previous pin's own
// requirement, so a pin widening over it leaves it alone.
func TestSetAirflowVersionKeepsAChosenPatchWhenThePinWidens(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow==2.9.1'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "2")
	require.NoError(t, err)

	got := readFile(t, path)
	assert.Contains(t, got, "'apache-airflow==2.9.1',")
	assert.Contains(t, got, "airflow = '2' # upgrade with the team")
	assert.Empty(t, change.Requirements)
}

// A patch pin outside the new series still moves with it.
func TestSetAirflowVersionMovesAnExactPatchPin(t *testing.T) {
	body := strings.Replace(pinFixture, "'apache-airflow[celery]==2.9.*'", "'apache-airflow==2.9.1'", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)

	assert.Contains(t, readFile(t, path), "'apache-airflow==3.1.*',")
	assert.Equal(t, []string{"apache-airflow==3.1.*"}, change.Requirements)
}

func TestPinCovers(t *testing.T) {
	cases := []struct {
		pin, stated string
		want        bool
	}{
		{"3.1", "3.1", true},
		{"3.1", "3.1.2", true},
		{"3", "3.1.2", true},
		{"2.9", "2.9.1", true},
		{"3.1.2", "3.1.2", true},
		{"3.1.2", "3.1.3", false},
		{"3.1", "3.10", false},
		{"3.1", "2.9", false},
		{"3", "2.9.1", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, pinCovers(c.pin, c.stated), "pinCovers(%q, %q)", c.pin, c.stated)
	}
}
