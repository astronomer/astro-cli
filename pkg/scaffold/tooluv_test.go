package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// [tool.uv] is the project's. Nothing this package does writes one into a new
// manifest, and nothing edits one a manifest already has, whatever the pin.

func readManifest(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	return string(raw)
}

func TestInitWritesNoToolUV(t *testing.T) {
	for _, pin := range []string{"", "3.1", "3.1.8", "3.2", "3.3", "2.10"} {
		t.Run("pin="+pin, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Run(dir, Options{AirflowVersion: pin})
			require.NoError(t, err)
			assert.NotContains(t, readManifest(t, dir), "[tool.uv]")
		})
	}
}

func TestConvertWritesNoToolUV(t *testing.T) {
	for _, from := range []string{
		"astrocrpublic.azurecr.io/runtime:3.1-12",
		"astrocrpublic.azurecr.io/runtime:3.3-8",
		"quay.io/astronomer/astro-runtime:11.8.0",
	} {
		t.Run(from, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM "+from+"\n"), 0o600))

			_, err := Run(dir, Options{})
			require.NoError(t, err)
			assert.NotContains(t, readManifest(t, dir), "[tool.uv]")
		})
	}
}

func TestAdoptLeavesToolUVAlone(t *testing.T) {
	const theirs = "[tool.uv]\n# ours\nconstraint-dependencies = ['pandas<3', 'sqlalchemy<2.1']\n"
	for _, tc := range []struct {
		name, manifest string
		opts           Options
	}{
		{name: "no [tool.uv], 3.1", manifest: "[project]\nname = 'x'\nversion = '1.0'\n", opts: Options{AirflowVersion: "3.1"}},
		{name: "no [tool.uv], default", manifest: "[project]\nname = 'x'\nversion = '1.0'\n"},
		{name: "their [tool.uv], 3.1", manifest: "[project]\nname = 'x'\nversion = '1.0'\n\n" + theirs, opts: Options{AirflowVersion: "3.1"}},
		{name: "their [tool.uv], 3.3", manifest: "[project]\nname = 'x'\nversion = '1.0'\n\n" + theirs, opts: Options{AirflowVersion: "3.3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(tc.manifest), 0o600))

			res, err := Run(dir, tc.opts)
			require.NoError(t, err)
			require.True(t, res.Adopted)
			got := readManifest(t, dir)
			if strings.Contains(tc.manifest, "[tool.uv]") {
				assert.Contains(t, got, theirs, "the table is carried exactly as written")
			} else {
				assert.NotContains(t, got, "[tool.uv]")
			}
			for _, l := range append(append([]string{}, res.Updated...), res.Created...) {
				assert.NotContains(t, l, "constraint-dependencies")
			}
		})
	}
}

// A pin change, in either direction across 3.1, carries [tool.uv] through
// byte for byte, including a sqlalchemy<2.1 an earlier init wrote.
func TestSetAirflowVersionLeavesToolUVAlone(t *testing.T) {
	const theirs = "\n[tool.uv]\n# ours\nconstraint-dependencies = ['sqlalchemy<2.1']\n"
	for _, tc := range []struct{ from, to string }{
		{from: "3.3", to: "3.1"},
		{from: "3.1", to: "3.3"},
		{from: "3.2", to: "3.3"},
	} {
		t.Run(tc.from+" to "+tc.to, func(t *testing.T) {
			dir, path := writeEditFixture(t,
				"[project]\nname = 'x'\ndependencies = ['apache-airflow=="+tc.from+".*']\n\n"+
					"[tool.astro]\nairflow = '"+tc.from+"'\n"+theirs, 0o644)

			_, err := SetAirflowVersion(dir, nil, tc.to)
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(readFile(t, path), theirs), "[tool.uv] changed:\n%s", readFile(t, path))
		})
	}
}

// olderInitManifest is a manifest as init wrote it for a 3.2 pin before the
// Airflow 3 floor followed the runtime.
const olderInitManifest = "[project]\nname = 'x'\nversion = '0.1.0'\nrequires-python = '>=3.10'\n" +
	"dependencies = ['apache-airflow==3.2.*']\n\n[tool.astro]\n"

// That project never had today's floor, so its old one is recognized as init's
// and moved on the next pin change.
func TestSetAirflowVersionMovesTheFloorAnOlderInitWrote(t *testing.T) {
	for _, to := range []string{"3.3", "3.2.2"} {
		t.Run(to, func(t *testing.T) {
			dir, path := writeEditFixture(t, olderInitManifest, 0o644)

			change, err := SetAirflowVersion(dir, nil, to)
			require.NoError(t, err)
			assert.Equal(t, ">=3.12", change.RequiresPython)
			assert.Contains(t, readFile(t, path), "requires-python = '>=3.12'")
		})
	}
}

// A bound that is not one init wrote stays.
func TestSetAirflowVersionKeepsAChosenFloorOnAnOlderProject(t *testing.T) {
	dir, path := writeEditFixture(t,
		strings.Replace(olderInitManifest, "'>=3.10'", "'>=3.13'", 1), 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.3")
	require.NoError(t, err)
	assert.Empty(t, change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.13'")
}

// init never wrote ">=3.10" for Airflow 2, which always got an upper bound, so
// that bound on a 2.x project is someone's choice and a 2.x move keeps it.
func TestSetAirflowVersionKeepsAnOpenBoundOnAirflowTwo(t *testing.T) {
	dir, path := writeEditFixture(t,
		"[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==2.9.*']\n\n"+
			"[tool.astro]\n", 0o644)

	change, err := SetAirflowVersion(dir, nil, "2.10")
	require.NoError(t, err)
	assert.Empty(t, change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.10'")
}

// Setting the pin it already has moves nothing, even the old floor a real move
// would raise.
func TestSetAirflowVersionLeavesAnOlderProjectAloneOnTheSamePin(t *testing.T) {
	dir, path := writeEditFixture(t, olderInitManifest, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)
	assert.False(t, change.Changed)
	assert.Empty(t, change.RequiresPython)
	assert.Equal(t, olderInitManifest, readFile(t, path))
}
