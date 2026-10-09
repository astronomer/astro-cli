package local

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// upgradeFixture pins Airflow 3.40 with the requires-python the catalog below
// gives that series, a runtime build of it, a Dockerfile, and the leftover
// [tool.astro] airflow line, so one upgrade to 3.41 moves every field the
// output reports.
const upgradeFixture = "[project]\nname = 'x'\nrequires-python = '>=3.13'\n" +
	"dependencies = ['apache-airflow==3.40.*', 'pandas']\n\n" +
	"[tool.astro]\nairflow = '3.40'\nruntime = '3.40-1'\n"

func upgradeCatalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
		"3.40-1": {"metadata": {"airflowVersion": "3.40.0", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.13", "3.14"]}},
		"3.41-2": {"metadata": {"airflowVersion": "3.41.0", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.14", "3.15"]}}}}`))
	require.NoError(t, err)
	return c
}

// statusRuntime answers ReadStatus with state, and fails everything else, so
// a command that tries to start, restart or attach shows up as an error.
type statusRuntime struct {
	fakeRuntime
	state localrt.State
}

func (s statusRuntime) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{State: s.state}, nil
}

func upgradeDeps(t *testing.T, toml string) (d Deps, launched *string, path string) {
	t.Helper()
	d, _ = testDeps(t)
	dir := t.TempDir()
	path = filepath.Join(dir, "pyproject.toml")
	require.NoError(t, os.WriteFile(path, []byte(toml), 0o600))
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return upgradeCatalog(t) }
	launched = new(string)
	d.LaunchOtto = func(prompt string) error {
		*launched = prompt
		return nil
	}
	return d, launched, path
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestUpgradeAirflowWritesThePinAndSaysWhatMoved(t *testing.T) {
	d, launched, path := upgradeDeps(t, upgradeFixture+"dockerfile = 'Dockerfile'\n")
	stdout := d.Stdout.(interface{ String() string })

	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41"))

	written := readText(t, path)
	assert.Contains(t, written, "'apache-airflow==3.41.*'")
	assert.Contains(t, written, "requires-python = '>=3.14'")
	assert.NotContains(t, written, "airflow = '3.40'")
	assert.Contains(t, written, "dockerfile = 'Dockerfile'")

	out := stdout.String()
	for _, want := range []string{
		"Airflow: 3.40 -> 3.41 in " + path,
		"  requirement: apache-airflow==3.41.*",
		"  requires-python: >=3.14",
		"  tool.astro airflow: removed. Nothing reads it any more",
		"Not changed: Dockerfile. This project declares it, so its FROM line decides the image in Docker mode. Update its base image for Airflow 3.41 yourself.",
		"Not changed: providers and Dag code.",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "still running")
	assert.NotContains(t, out, "Upgrading to", "a named version is not a pick")
	assert.NotContains(t, out, "—", "no em-dashes in the output")
	assert.Empty(t, *launched, "Otto starts only with --with-otto")
}

func TestUpgradeAirflowSaysWhereTheRuntimeWent(t *testing.T) {
	d, _, path := upgradeDeps(t, upgradeFixture)
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41"))
	assert.Contains(t, readText(t, path), "runtime = '3.41-2'")
	assert.Contains(t, stdout.String(), "  tool.astro runtime: moved to 3.41-2")

	d, _, path = upgradeDeps(t, upgradeFixture)
	d.RuntimeCatalog = nil
	stdout = d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41"))
	assert.NotContains(t, readText(t, path), "runtime =")
	assert.Contains(t, stdout.String(), "  tool.astro runtime: removed, so the image builds from the newest runtime of the new series")
}

func TestUpgradeAirflowSaysWhenCoreBecameAirflow(t *testing.T) {
	d, _, _ := upgradeDeps(t, "[project]\nname = 'x'\ndependencies = ['apache-airflow-core==3.1.*']\n\n[tool.astro]\n")
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "2.10"))
	assert.Contains(t, stdout.String(), "  apache-airflow-core became apache-airflow, since core is published only for Airflow 3")
}

func TestUpgradeAirflowJSONCarriesTheChange(t *testing.T) {
	d, _, path := upgradeDeps(t, upgradeFixture)
	d.Runtime = statusRuntime{state: localrt.StateRunning}
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41", "--output", "json"))

	var got airflowUpgrade
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &got))
	assert.Equal(t, airflowUpgrade{
		AirflowPinChange: scaffold.AirflowPinChange{
			Previous: "3.40", Version: "3.41", Changed: true,
			Requirements:      []string{"apache-airflow==3.41.*"},
			RemovedAirflowKey: true, RequiresPython: ">=3.14", Runtime: "3.41-2",
		},
		Manifest:      path,
		RestartNeeded: true,
	}, got)
}

func TestUpgradeAirflowSaysARunningAirflowNeedsARestart(t *testing.T) {
	for _, state := range []localrt.State{localrt.StateRunning, localrt.StateStarting} {
		d, _, _ := upgradeDeps(t, upgradeFixture)
		d.Runtime = statusRuntime{state: state}
		stdout := d.Stdout.(interface{ String() string })
		require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41"))
		assert.Contains(t, stdout.String(),
			"Airflow is still running the old version. Run astro local restart when the project is ready.", state)
	}
	d, _, _ := upgradeDeps(t, upgradeFixture)
	d.Runtime = statusRuntime{state: localrt.StateStopped}
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41"))
	assert.NotContains(t, stdout.String(), "still running")
}

func TestUpgradeAirflowToThePinItHasChangesNothing(t *testing.T) {
	const pinned = "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n"
	d, _, path := upgradeDeps(t, pinned)
	d.Runtime = statusRuntime{state: localrt.StateRunning}
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.1"))
	assert.Equal(t, pinned, readText(t, path))
	out := stdout.String()
	assert.Contains(t, out, "Airflow is already pinned to 3.1. Nothing changed.")
	assert.Contains(t, out, "Not changed: Dockerfile.", "a declared Dockerfile is reported on a no-op too")
	assert.NotContains(t, out, "still running", "nothing changed, so nothing needs a restart")
	assert.NotContains(t, out, "providers and Dag code")
}

func TestUpgradeAirflowRefusesWhatItCannotRun(t *testing.T) {
	for version, want := range map[string]error{
		"1.10": errUnsupportedAirflow,
		"4":    errUnsupportedAirflow,
		"next": scaffold.ErrInvalidAirflowVersion,
	} {
		d, launched, path := upgradeDeps(t, upgradeFixture)
		err := execute(t, d, "local", "upgrade", "airflow", version, "--with-otto")
		require.ErrorIs(t, err, want, version)
		assert.Equal(t, upgradeFixture, readText(t, path), version)
		assert.Empty(t, *launched, version)
	}
	assert.Contains(t, checkSupportedAirflow("4").Error(), "Local Airflow runs Airflow 2 and Airflow 3")
	for _, ok := range []string{"2", "2.10", "3", "3.1", "3.1.2"} {
		assert.NoError(t, checkSupportedAirflow(ok), ok)
	}
}

func TestUpgradeAirflowWithOttoHandsOttoThePrompt(t *testing.T) {
	d, launched, _ := upgradeDeps(t, upgradeFixture+"dockerfile = 'Dockerfile'\n")
	// Every other runtime call fails, so a start or restart would be an error.
	d.Runtime = statusRuntime{state: localrt.StateStopped}
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41", "--with-otto"))

	want := scaffold.AirflowUpgradePrompt(&scaffold.AirflowPinChange{
		Previous: "3.40", Version: "3.41", Changed: true,
		Requirements:      []string{"apache-airflow==3.41.*"},
		RemovedAirflowKey: true, RequiresPython: ">=3.14", RuntimeRemoved: true, Dockerfile: "Dockerfile",
	}, cliUpgradePrompt)
	assert.Equal(t, want, *launched)
	assert.Contains(t, *launched, "The Astro CLI has already made the mechanical edit")
	assert.Contains(t, *launched, "bring Airflow up on 3.41: astro local restart if it is running, astro local start if it is stopped.")
}

func TestUpgradeAirflowWithOttoReportsOttosFailure(t *testing.T) {
	d, _, _ := upgradeDeps(t, upgradeFixture)
	boom := errors.New("otto failed")
	d.LaunchOtto = func(string) error { return boom }
	require.ErrorIs(t, execute(t, d, "local", "upgrade", "airflow", "3.41", "--with-otto"), boom)
}

func TestUpgradeAirflowWithOttoRefusesWhatItCannotDo(t *testing.T) {
	d, launched, path := upgradeDeps(t, upgradeFixture)
	err := execute(t, d, "local", "upgrade", "airflow", "3.41", "--with-otto", "--output", "json")
	require.ErrorContains(t, err, "cannot be combined with --output json")
	assert.Equal(t, upgradeFixture, readText(t, path))
	assert.Empty(t, *launched)

	d, _, path = upgradeDeps(t, upgradeFixture)
	d.LaunchOtto = nil
	require.ErrorContains(t, execute(t, d, "local", "upgrade", "airflow", "3.41", "--with-otto"), "not available")
	assert.Equal(t, upgradeFixture, readText(t, path))
}

func TestUpgradeAirflowNeedsAProject(t *testing.T) {
	d, _ := testDeps(t)
	require.Error(t, execute(t, d, "local", "upgrade", "airflow", "3.1"))
}

func TestUpgradeAirflowWithOttoHandsOttoAWriteItCouldNotMake(t *testing.T) {
	for name, tc := range map[string]struct{ toml, from string }{
		"a manifest that does not parse": {"[project\nname = 'x'\n", ""},
		"a manifest the writer refuses": {
			"[project]\nname = 'x'\ndependencies = ['apache-airflow==2.9.*', 'apache-airflow-core==2.9.*']\n\n[tool.astro]\n", "2.9",
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, launched, path := upgradeDeps(t, tc.toml)
			stderr := d.Stderr.(interface{ String() string })
			require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.1", "--with-otto"))
			assert.Equal(t, tc.toml, readText(t, path), "a failed write leaves the file")
			assert.Contains(t, stderr.String(), "Error: setting the Airflow version: ")
			assert.Contains(t, stderr.String(), "Starting Otto to make the upgrade instead.")

			require.NotEmpty(t, *launched)
			first, _, _ := strings.Cut(stderr.String(), "\n")
			writeErr := strings.TrimPrefix(first, "Error: ")
			assert.Equal(t, scaffold.AirflowUpgradeFallbackPrompt(tc.from, "3.1", writeErr, cliUpgradePrompt), *launched)
			assert.Contains(t, *launched, "The Astro CLI tried to move the requirement and could not: setting the Airflow version: ")
		})
	}
}

func TestUpgradeAirflowWithoutOttoReportsAWriteItCouldNotMake(t *testing.T) {
	d, launched, _ := upgradeDeps(t, "[project\nname = 'x'\n")
	require.ErrorContains(t, execute(t, d, "local", "upgrade", "airflow", "3.1"), "setting the Airflow version: ")
	assert.Empty(t, *launched)
}

// With no version, the catalog's upgrade target is the version: 3.41, the
// newest Airflow 3 upgradeCatalog offers.
func TestUpgradeAirflowWithNoVersionMovesToTheCatalogsTarget(t *testing.T) {
	d, launched, path := upgradeDeps(t, upgradeFixture)
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow"))
	assert.Contains(t, readText(t, path), "'apache-airflow==3.41.*'")
	out := stdout.String()
	assert.True(t, strings.HasPrefix(out, "Upgrading to 3.41 (the latest Airflow 3 in the runtime catalog)\n"), out)
	assert.Contains(t, out, "Airflow: 3.40 -> 3.41 in ")
	assert.Empty(t, *launched)
}

func TestUpgradeAirflowWithNoVersionJSONNamesTheTarget(t *testing.T) {
	d, _, _ := upgradeDeps(t, upgradeFixture)
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "-o", "json"))
	var got airflowUpgrade
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &got))
	assert.Equal(t, "3.41", got.Target)
	assert.Equal(t, "3.41", got.Version)
	assert.True(t, got.Changed)

	// A named version is not a pick, so it reports no target.
	d, _, _ = upgradeDeps(t, upgradeFixture)
	stdout = d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "3.41", "-o", "json"))
	got = airflowUpgrade{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &got))
	assert.Empty(t, got.Target)
}

func TestUpgradeAirflowWithNoVersionOnTheLatestChangesNothing(t *testing.T) {
	const latest = "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.41.*']\n\n[tool.astro]\n"
	d, _, path := upgradeDeps(t, latest)
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow"))
	assert.Equal(t, latest, readText(t, path))
	assert.Equal(t, "Airflow is already pinned to 3.41. Nothing changed.\n", stdout.String())
}

func TestUpgradeAirflowWithNoVersionNeedsTheCatalog(t *testing.T) {
	for name, catalog := range map[string]func(context.Context) *runtimeversions.Catalog{
		"no seam":          nil,
		"a catalog failed": func(context.Context) *runtimeversions.Catalog { return nil },
	} {
		t.Run(name, func(t *testing.T) {
			d, launched, path := upgradeDeps(t, upgradeFixture)
			d.RuntimeCatalog = catalog
			err := execute(t, d, "local", "upgrade", "airflow", "--with-otto")
			require.ErrorContains(t, err, "could not read the runtime catalog")
			require.ErrorContains(t, err, "Pass a version instead, like astro local upgrade airflow 3.1")
			assert.Equal(t, upgradeFixture, readText(t, path))
			assert.Empty(t, *launched, "a version that cannot be picked starts no Otto")
		})
	}
}

func TestUpgradeAirflowWithNoVersionNeedsAPinToMoveFrom(t *testing.T) {
	d, launched, _ := upgradeDeps(t, "[project\nname = 'x'\n")
	err := execute(t, d, "local", "upgrade", "airflow", "--with-otto")
	require.ErrorContains(t, err, "could not read the Airflow version pyproject.toml pins")
	require.ErrorContains(t, err, "Pass a version instead")
	assert.Empty(t, *launched)
}

func TestUpgradeAirflowWithNoVersionWithOttoSendsThePickedTarget(t *testing.T) {
	d, launched, _ := upgradeDeps(t, upgradeFixture)
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "--with-otto"))
	want := scaffold.AirflowUpgradePrompt(&scaffold.AirflowPinChange{
		Previous: "3.40", Version: "3.41", Changed: true,
		Requirements:      []string{"apache-airflow==3.41.*"},
		RemovedAirflowKey: true, RequiresPython: ">=3.14", Runtime: "3.41-2",
	}, cliUpgradePrompt)
	assert.Equal(t, want, *launched)
}

// A bare upgrade stays in the pin's generation, and names Airflow 3 as
// available to an Airflow 2 pin without moving to it.
func TestPickAirflowTarget(t *testing.T) {
	catalog := upgradeCatalogV2(t)
	for pin, want := range map[string]struct{ target, available string }{
		"2.9":  {"2.10", "3.41"},
		"2.10": {"2.10", "3.41"}, // already on the newest 2: its own target
		"3.40": {"3.41", ""},
		"3.41": {"3.41", ""},
	} {
		target, available, err := pickAirflowTarget(catalog, pin)
		require.NoError(t, err, pin)
		assert.Equal(t, want.target, target, pin)
		assert.Equal(t, want.available, available, pin)
	}
}

const airflow29 = "[project]\nname = 'x'\ndependencies = ['apache-airflow==2.9.*']\n\n[tool.astro]\n"

const airflow210 = "[project]\nname = 'x'\ndependencies = ['apache-airflow==2.10.*']\n\n[tool.astro]\n"

// An Airflow 2 project behind on Airflow 2 moves to the newest Airflow 2, and
// is told Airflow 3 is there.
func TestUpgradeAirflowWithNoVersionStaysOnAirflow2AndNamesAirflow3(t *testing.T) {
	d, _, path := upgradeDeps(t, airflow29)
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return upgradeCatalogV2(t) }
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow"))
	assert.Contains(t, readText(t, path), "'apache-airflow==2.10.*'")
	out := stdout.String()
	assert.Contains(t, out, "Upgrading to 2.10 (the latest Airflow 2 in the runtime catalog)\n")
	assert.Contains(t, out, "\nAirflow 3 is available: astro local upgrade airflow 3.41\n")
}

// The move within Airflow 2 is work for Otto, Airflow 3 on offer or not.
func TestUpgradeAirflowWithNoVersionWithinAirflow2StartsOtto(t *testing.T) {
	d, launched, _ := upgradeDeps(t, airflow29)
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return upgradeCatalogV2(t) }
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "--with-otto"))
	assert.Contains(t, *launched, "Upgrade my Airflow from 2.9 to 2.10.")
}

// On the newest Airflow 2 the only newer Airflow is the next generation, so a
// bare upgrade changes nothing, says so, and starts no Otto.
func TestUpgradeAirflowWithNoVersionOnTheNewestAirflow2ChangesNothing(t *testing.T) {
	d, launched, path := upgradeDeps(t, airflow210)
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return upgradeCatalogV2(t) }
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "--with-otto"))
	assert.Equal(t, airflow210, readText(t, path))
	assert.Equal(t, "Airflow is already on the newest Airflow 2 (2.10). Airflow 3 is available: astro local upgrade airflow 3.41\n",
		stdout.String())
	assert.Empty(t, *launched, "nothing changed and the next generation is the user's to ask for")
}

func TestUpgradeAirflowWithNoVersionJSONNamesWhatIsAvailable(t *testing.T) {
	for toml, changed := range map[string]bool{airflow29: true, airflow210: false} {
		d, _, _ := upgradeDeps(t, toml)
		d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return upgradeCatalogV2(t) }
		stdout := d.Stdout.(interface{ String() string })
		require.NoError(t, execute(t, d, "local", "upgrade", "airflow", "-o", "json"))
		var got airflowUpgrade
		require.NoError(t, json.Unmarshal([]byte(stdout.String()), &got))
		assert.Equal(t, "2.10", got.Target)
		assert.Equal(t, "3.41", got.Available)
		assert.Equal(t, changed, got.Changed)
	}
}

// An Airflow 3 project has no next generation to name.
func TestUpgradeAirflowWithNoVersionOnAirflow3NamesNothingAvailable(t *testing.T) {
	d, _, _ := upgradeDeps(t, upgradeFixture)
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return upgradeCatalogV2(t) }
	stdout := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "local", "upgrade", "airflow"))
	assert.NotContains(t, stdout.String(), "is available")
}

func upgradeCatalogV2(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{
		"runtimeVersions": {"13.7.0": {"metadata": {"airflowVersion": "2.10.5", "channel": "stable"}}},
		"runtimeVersionsV3": {"3.41-2": {"metadata": {"airflowVersion": "3.41.0", "channel": "stable"}}}}`))
	require.NoError(t, err)
	return c
}
