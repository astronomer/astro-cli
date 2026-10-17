package local

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// errNoHomeForTest is what proxydaemon.Dir says with no home directory.
var errNoHomeForTest = errors.New("finding a home directory for local Airflow's proxy routes: set HOME or ASTRO_HOME: no home")

// homeless is d as NewDeps builds it with no home directory: the stand-in
// runtime, and no proxy dir.
func homeless(d Deps) Deps {
	d.Runtime = homelessRuntime{err: errNoHomeForTest}
	d.ProxyDir = ""
	d.HomeErr = errNoHomeForTest
	return d
}

// A start says there is no home before any of its pre-start work: it does not
// even look for the project.
func TestStartWithNoHomeFailsBeforeAnything(t *testing.T) {
	for _, args := range [][]string{{"local", "start"}, {"start"}, {"local", "list", "--clean"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, stdout := testDeps(t)
			d = homeless(d)
			d.WorkingDir = func() (string, error) {
				t.Error("looked for the project before saying there is no home")
				return t.TempDir(), nil
			}

			err := execute(t, d, args...)

			require.ErrorIs(t, err, errNoHomeForTest)
			assert.Empty(t, stdout.String(), "nothing ran before the error")
		})
	}
}

// The daemon's and the session watcher's own processes return the error NewDeps
// resolved, not a second resolution's or none.
func TestProxyDirReportsTheResolutionError(t *testing.T) {
	_, err := homeless(Deps{}).proxyDir()
	require.ErrorIs(t, err, errNoHomeForTest)

	_, err = Deps{}.proxyDir()
	require.Error(t, err, "a Deps with no dir and no reason still fails")

	dir, err := Deps{ProxyDir: "/x/.astro/proxy"}.proxyDir()
	require.NoError(t, err)
	assert.Equal(t, "/x/.astro/proxy", dir)
}

// The commands that need the runtime fail with the reason, as an error rather
// than a usage error, and without a panic.
func TestRuntimeCommandsSayThereIsNoHome(t *testing.T) {
	for _, args := range [][]string{
		{"local", "start"},
		{"local", "status"},
		{"local", "api", "/dags"},
		{"start"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, _ := testDeps(t)
			writeManifest(t, &d, t.TempDir())
			d = homeless(d)

			err := execute(t, d, args...)

			require.ErrorIs(t, err, errNoHomeForTest)
			assert.False(t, cliout.IsUsage(err), "a missing home is not a usage error")
		})
	}
}

// The commands that never reach the runtime run as they do with a home.
func TestCommandsWithoutTheRuntimeRunWithNoHome(t *testing.T) {
	t.Run("package mwaa", func(t *testing.T) {
		d, _ := testDeps(t)
		dir := t.TempDir()
		writeManifest(t, &d, dir)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o755))
		d = homeless(d)
		require.NoError(t, execute(t, d, "package", "mwaa", "--out-dir", filepath.Join(t.TempDir(), "out")))
	})
	t.Run("local env list", func(t *testing.T) {
		d, _, _ := envDeps(t, envProject(t, ""), "")
		d = homeless(d)
		require.NoError(t, execute(t, d, "local", "env", "list"))
	})
}

// An upgrade edits the pin first, so a status it cannot read does not fail
// it: it warns, and keeps the restart hint since Airflow may be running.
func TestUpgradeAirflowWithNoHomeWarnsAndKeepsTheRestartHint(t *testing.T) {
	d, _, path := upgradeDeps(t, upgradeFixture)
	d = homeless(d)
	stdout := d.Stdout.(interface{ String() string })
	stderr := d.Stderr.(interface{ String() string })

	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41"))

	assert.Contains(t, readText(t, path), "'apache-airflow==3.41.*'")
	assert.Contains(t, stderr.String(),
		"Warning: could not tell whether Airflow is running ("+errNoHomeForTest.Error()+"); restart it to apply")
	assert.Contains(t, stdout.String(), "Airflow is still running the old version. Run astro local restart when the project is ready.")
}
