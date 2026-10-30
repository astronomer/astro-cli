package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// runtimeUpgradeCatalog has builds 3.3-3 to 3.3-10 as the issue describes
// them (one Airflow across several builds, then a patch), one yanked, and one
// of 3.4.
func runtimeUpgradeCatalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
		"3.3-3": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
		"3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-02-01", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
		"3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-03-01", "yanked": true, "yankedReason": "bad scheduler.", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
		"3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-04-01", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.13"}},
		"3.4-1": {"metadata": {"airflowVersion": "3.4.0", "channel": "stable", "releaseDate": "2026-05-01", "pythonVersions": ["3.13", "3.14"], "defaultPythonVersion": "3.14"}}}}`))
	require.NoError(t, err)
	return c
}

// refreshDeps is a project on manifest src with the build lookup, the
// runtime's constraints and uv lock all answering, and a log of what the
// constraints were asked for and how often uv lock ran.
type refreshDeps struct {
	d              Deps
	path           string
	stdout, stderr *bytes.Buffer
	asked          []string
	locks          int
}

func newRefreshDeps(t *testing.T, src string) *refreshDeps {
	t.Helper()
	t.Setenv("UV_EXCLUDE_NEWER", "")
	rd := &refreshDeps{stderr: &bytes.Buffer{}}
	d, stdout := testDeps(t)
	rd.stdout = stdout
	d.Stderr = rd.stderr
	d.Runtime = statusRuntime{}
	dir := t.TempDir()
	rd.path = filepath.Join(dir, "pyproject.toml")
	require.NoError(t, os.WriteFile(rd.path, []byte(src), 0o600))
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return runtimeUpgradeCatalog(t) }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) { return build332, nil }
	d.RuntimeConstraints = func(_ context.Context, build, python string) ([]string, error) {
		rd.asked = append(rd.asked, build+"/"+python)
		return []string{"SQLAlchemy==2.0." + build[len(build)-1:]}, nil
	}
	d.Lock = func(_ context.Context, dir string) error {
		rd.locks++
		return os.WriteFile(filepath.Join(dir, lockFile), []byte("version = 1\n"), 0o600)
	}
	rd.d = d
	return rd
}

// The issue's acceptance: on a 3.3.* project, the build is written, the
// constraints become 3.3-10's, and uv.lock is locked, in the one command.
func TestUpgradeRuntimeWritesTheBuildRefreshesConstraintsAndLocks(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)

	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-10"))

	got := readText(t, rd.path)
	assert.Contains(t, got, "runtime = '3.3-10'")
	assert.Contains(t, got, "'apache-airflow==3.3.*'", "3.3-10 is of the project's series")
	assert.Contains(t, got, "# Astro Runtime 3.3-10 (Python 3.13) constrains these.")
	assert.Equal(t, []string{"3.3-10/3.13"}, rd.asked)
	assert.Equal(t, 1, rd.locks)
	out := rd.stdout.String()
	for _, want := range []string{
		"Runtime: no build -> 3.3-10 in " + rd.path,
		"  Airflow: 3.3.2",
		"  Python: 3.13",
		"  [tool.uv]: now holds what Astro Runtime 3.3-10 constrains",
		"  uv.lock: locked",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "Not changed: providers", "the Airflow requirement did not move")
	assert.Empty(t, rd.stderr.String())
}

func TestUpgradeRuntimeGoesBackAndAcrossSeries(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest+"runtime = '3.3-10'\n")
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-3"))
	assert.Contains(t, readText(t, rd.path), "runtime = '3.3-3'")
	assert.Contains(t, rd.stdout.String(), "Runtime: 3.3-10 -> 3.3-3")

	rd = newRefreshDeps(t, buildManifest)
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.4-1"))
	got := readText(t, rd.path)
	assert.Contains(t, got, "'apache-airflow==3.4.*'")
	assert.Contains(t, got, "runtime = '3.4-1'")
	out := rd.stdout.String()
	assert.Contains(t, out, "  Airflow: 3.3 -> 3.4.0")
	assert.Contains(t, out, "Not changed: providers and Dag code.")
}

func TestUpgradeRuntimeWithNoBuildPicksTheNewestOfTheSeries(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest+"runtime = '3.3-3'\n")
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime"))
	assert.Contains(t, readText(t, rd.path), "runtime = '3.3-10'", "3.3-8 is yanked, and 3.4-1 is another series")
	assert.Contains(t, rd.stdout.String(), "Upgrading to 3.3-10")

	rd.d.RuntimeCatalog = nil
	err := execute(t, rd.d, "local", "upgrade", "runtime")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Pass a build instead")
}

// A build newer than the newest published one (the catalog dates it ahead,
// or a cached copy predates it) is not traded for an older one.
func TestUpgradeRuntimeWithNoBuildNeverMovesBack(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest+"runtime = '3.3-12'\n")
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime"))
	assert.Contains(t, readText(t, rd.path), "runtime = '3.3-12'")
	assert.Contains(t, rd.stdout.String(), "The runtime is already 3.3-12.")
}

// An upgrade that keeps the build still refreshes, and says so rather than
// that nothing changed.
func TestUpgradeRuntimeOnTheSameBuildSaysWhatTheRefreshChanged(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest+"runtime = '3.3-10'\n")
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-10"))
	out := rd.stdout.String()
	assert.Contains(t, out, "The runtime is already 3.3-10. Refreshed what follows from it:")
	assert.Contains(t, out, "  uv.lock: locked")
	assert.NotContains(t, out, "Nothing changed")

	rd.stdout.Reset()
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-10"))
	assert.Contains(t, rd.stdout.String(), "The runtime is already 3.3-10. Nothing changed.")
}

func TestUpgradeRuntimePicksAPython(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-10", "--python", "3.14"))
	assert.Contains(t, readText(t, rd.path), "requires-python = '==3.14.*'")
	assert.Equal(t, []string{"3.3-10/3.14"}, rd.asked, "the constraints follow the Python picked")
	assert.Contains(t, rd.stdout.String(), "  requires-python: ==3.14.*")
}

// A yanked build is allowed, as the manifest checks allow it, and warned
// about with the catalog's reason.
func TestUpgradeRuntimeWarnsAboutAYankedBuild(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-8"))
	assert.Contains(t, readText(t, rd.path), "runtime = '3.3-8'")
	assert.Contains(t, rd.stderr.String(), "runtime 3.3-8 is yanked: bad scheduler")
}

// Offline, an Airflow 3 build is still written from its tag, and the command
// says what it could not check or refresh.
func TestUpgradeRuntimeOfflineSaysWhatItCouldNotRefresh(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)
	rd.d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return nil }
	rd.d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) {
		return runtimeversions.AstroBuild{}, errors.New("dial tcp: no route to host")
	}
	rd.d.Lock = func(context.Context, string) error { return errors.New("network unreachable") }

	require.NoError(t, execute(t, rd.d, "local", "upgrade", "runtime", "3.3-10", "-o", "json"))
	assert.Contains(t, readText(t, rd.path), "runtime = '3.3-10'")

	var res runtimeUpgrade
	require.NoError(t, json.Unmarshal(rd.stdout.Bytes(), &res))
	assert.True(t, res.Changed)
	assert.Empty(t, res.Findings, "an Airflow 3 build's tag already said its series, as the image-build check says")
	require.NotNil(t, res.Refresh.Lock)
	assert.Equal(t, lockFailed, res.Refresh.Lock.Status)
	assert.Len(t, res.Refresh.Warnings, 2)
	assert.Contains(t, res.Refresh.Warnings[0], "Astronomer's index could not be read")
	assert.Contains(t, res.Refresh.Warnings[1], "uv lock failed: network unreachable")
	assert.Empty(t, rd.stderr.String(), "in json mode the result carries the warnings")
}

func TestUpgradeRuntimeRefusesAFloatingTag(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)
	err := execute(t, rd.d, "local", "upgrade", "runtime", "3.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not one build like 3.3-10")
	assert.Equal(t, buildManifest, readText(t, rd.path))
	assert.Zero(t, rd.locks)
}

// The issue's second half: moving the Airflow pin refreshes the constraints
// and relocks in the same command, with no local start.
func TestUpgradeAirflowRefreshesConstraintsAndLocks(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)

	require.NoError(t, execute(t, rd.d, "local", "upgrade", "airflow", "3.4"))

	got := readText(t, rd.path)
	assert.Contains(t, got, "'apache-airflow==3.4.*'")
	assert.Contains(t, got, "# Astro Runtime 3.4-1 (Python 3.14) constrains these.")
	assert.Equal(t, 1, rd.locks)
	out := rd.stdout.String()
	assert.Contains(t, out, "  [tool.uv]: now holds what Astro Runtime 3.4-1 constrains")
	assert.Contains(t, out, "  uv.lock: locked")
}

func TestUpgradeAirflowSaysWhenTheConstraintsCouldNotBeRead(t *testing.T) {
	rd := newRefreshDeps(t, buildManifest)
	rd.d.RuntimeConstraints = func(context.Context, string, string) ([]string, error) {
		return nil, errors.New("cdn.astronomer.io: timeout")
	}
	require.NoError(t, execute(t, rd.d, "local", "upgrade", "airflow", "3.4"))
	assert.Contains(t, rd.stderr.String(), "the runtime's constraints in [tool.uv] were not refreshed: cdn.astronomer.io: timeout")
	assert.Equal(t, 1, rd.locks, "the requirement moved, so the lock still follows it")
}
