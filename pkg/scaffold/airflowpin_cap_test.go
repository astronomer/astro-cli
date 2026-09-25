package scaffold

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// preThreePointThree is a manifest as init wrote it for a 3.2 pin before the
// cap was scoped to 3.1 and the Airflow 3 floor followed the runtime.
const preThreePointThree = "[project]\nname = 'x'\nversion = '0.1.0'\nrequires-python = '>=3.10'\n" +
	"dependencies = ['apache-airflow==3.2.*']\n\n[tool.astro]\nairflow = '3.2'\n\n" +
	"[tool.uv]\n# SQLAlchemy-Utils, which Airflow depends on, breaks on SQLAlchemy 2.1.\n" +
	"constraint-dependencies = ['sqlalchemy<2.1']\n"

// Such a project never crosses the 3.1 line, so the cap and the old floor are
// cleared on its next move instead.
func TestSetAirflowVersionClearsWhatAnOlderInitWrote(t *testing.T) {
	for _, to := range []string{"3.3", "3.2.2"} {
		t.Run(to, func(t *testing.T) {
			dir, path := writeEditFixture(t, preThreePointThree, 0o644)

			change, err := SetAirflowVersion(dir, nil, to)
			require.NoError(t, err)
			assert.True(t, change.SQLAlchemyCapRemoved)
			assert.Equal(t, ">=3.12", change.RequiresPython)
			got := readFile(t, path)
			assert.Contains(t, got, "requires-python = '>=3.12'")
			assert.NotContains(t, got, "[tool.uv]", "an emptied table goes too\n%s", got)
			assert.NotContains(t, got, "sqlalchemy", "and the comment over it\n%s", got)
		})
	}
}

// What someone chose stays: a SQLAlchemy constraint that is not the CLI's
// exact entry, and a bound that is not one init wrote.
func TestSetAirflowVersionKeepsWhatAPersonWroteOnAnOlderProject(t *testing.T) {
	body := strings.NewReplacer(
		"requires-python = '>=3.10'", "requires-python = '>=3.13'",
		"'sqlalchemy<2.1'", "'sqlalchemy <2.1'",
	).Replace(preThreePointThree)
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.3")
	require.NoError(t, err)
	assert.False(t, change.SQLAlchemyCapRemoved)
	assert.Empty(t, change.RequiresPython)
	assert.Equal(t, []string{"sqlalchemy <2.1"}, loadConstraints(t, dir))
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.13'")
}

// init never wrote ">=3.10" for Airflow 2, which always got an upper bound, so
// that bound on a 2.x project is someone's choice and a 2.x move keeps it.
func TestSetAirflowVersionKeepsAnOpenBoundOnAirflowTwo(t *testing.T) {
	dir, path := writeEditFixture(t,
		"[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==2.9.*']\n\n"+
			"[tool.astro]\nairflow = '2.9'\n", 0o644)

	change, err := SetAirflowVersion(dir, nil, "2.10")
	require.NoError(t, err)
	assert.Empty(t, change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.10'")
}

// Setting the pin it already has moves nothing, even on an older project whose
// cap and floor a real move would clear.
func TestSetAirflowVersionLeavesAnOlderProjectAloneOnTheSamePin(t *testing.T) {
	dir, path := writeEditFixture(t, preThreePointThree, 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)
	assert.False(t, change.Changed)
	assert.False(t, change.SQLAlchemyCapRemoved)
	assert.Empty(t, change.RequiresPython)
	assert.Equal(t, preThreePointThree, readFile(t, path))
}

// The SQLAlchemy cap follows a pin moved with SetAirflowVersion across the 3.1
// line, in both directions, the way init decides it for a new project.

// A default project carries no cap. Moved onto 3.1 without one, uv resolves
// SQLAlchemy 2.1 and Airflow 3.1 cannot import.
func TestSetAirflowVersionAddsTheCapMovingOntoThreePointOne(t *testing.T) {
	dir := t.TempDir()
	_, err := Run(dir, Options{})
	require.NoError(t, err)

	change, err := SetAirflowVersion(dir, nil, "3.1")
	require.NoError(t, err)
	assert.True(t, change.SQLAlchemyCapAdded)
	assert.False(t, change.SQLAlchemyCapRemoved)
	assert.True(t, change.Changed)
	assert.Equal(t, []string{"sqlalchemy<2.1"}, loadConstraints(t, dir))
}

// Left on after an upgrade, the cap makes the project unsatisfiable once an
// Airflow requires SQLAlchemy 2.1. A project init scaffolded on 3.1 and then
// moved ends up with no [tool.uv] at all, as one scaffolded on the new pin.
func TestSetAirflowVersionRemovesTheCapMovingOffThreePointOne(t *testing.T) {
	dir := t.TempDir()
	_, err := Run(dir, Options{AirflowVersion: "3.1"})
	require.NoError(t, err)
	require.Equal(t, []string{"sqlalchemy<2.1"}, loadConstraints(t, dir))

	change, err := SetAirflowVersion(dir, nil, "3.3")
	require.NoError(t, err)
	assert.True(t, change.SQLAlchemyCapRemoved)
	assert.False(t, change.SQLAlchemyCapAdded)
	assert.Nil(t, loadConstraints(t, dir))
	got := readFile(t, filepath.Join(dir, "pyproject.toml"))
	assert.NotContains(t, got, "[tool.uv]", "an emptied table goes too\n%s", got)
	assert.NotContains(t, got, "constraint-dependencies")
	assert.NotContains(t, got, "sqlalchemy", "the comment init wrote over the cap goes with it\n%s", got)
}

// Other constraints stay, and so does the table holding them.
func TestSetAirflowVersionRemovesOnlyTheCapEntry(t *testing.T) {
	dir, path := writeEditFixture(t,
		"[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nairflow = '3.1'\n\n"+
			"[tool.uv]\nconstraint-dependencies = ['pandas<3', 'sqlalchemy<2.1']\n", 0o644)

	change, err := SetAirflowVersion(dir, nil, "3.2")
	require.NoError(t, err)
	assert.True(t, change.SQLAlchemyCapRemoved)
	assert.Equal(t, []string{"pandas<3"}, loadConstraints(t, dir))
	assert.Contains(t, readFile(t, path), "[tool.uv]")
}

// A SQLAlchemy constraint the user wrote is theirs in both directions: moving
// onto 3.1 does not stack the cap beside it, and moving off does not take it.
func TestSetAirflowVersionLeavesAUserSQLAlchemyConstraintAlone(t *testing.T) {
	const theirs = "[tool.uv]\nconstraint-dependencies = ['SQLAlchemy<2.0.40']\n"
	for _, tc := range []struct{ from, to string }{
		{from: "3.3", to: "3.1"},
		{from: "3.1", to: "3.3"},
	} {
		t.Run(tc.from+" to "+tc.to, func(t *testing.T) {
			dir, _ := writeEditFixture(t,
				"[project]\nname = 'x'\ndependencies = ['apache-airflow=="+tc.from+".*']\n\n"+
					"[tool.astro]\nairflow = '"+tc.from+"'\n\n"+theirs, 0o644)

			change, err := SetAirflowVersion(dir, nil, tc.to)
			require.NoError(t, err)
			assert.False(t, change.SQLAlchemyCapAdded)
			assert.False(t, change.SQLAlchemyCapRemoved)
			assert.Equal(t, []string{"SQLAlchemy<2.0.40"}, loadConstraints(t, dir))
		})
	}
}

// Moving within 3.1, or not moving at all, is not crossing the line: the cap is
// neither added nor removed, and a repeated call writes nothing.
func TestSetAirflowVersionLeavesTheCapWithinThreePointOne(t *testing.T) {
	for _, tc := range []struct{ scaffold, to string }{
		{scaffold: "3.1", to: "3.1"},
		{scaffold: "3.3", to: "3.3"},
		{scaffold: "3.1", to: "3.1.8"},
	} {
		t.Run(tc.scaffold+" to "+tc.to, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Run(dir, Options{AirflowVersion: tc.scaffold})
			require.NoError(t, err)
			before := readFile(t, filepath.Join(dir, "pyproject.toml"))
			wantCap := loadConstraints(t, dir)

			change, err := SetAirflowVersion(dir, nil, tc.to)
			require.NoError(t, err)
			assert.False(t, change.SQLAlchemyCapAdded)
			assert.False(t, change.SQLAlchemyCapRemoved)
			assert.Equal(t, wantCap, loadConstraints(t, dir))
			if tc.scaffold == tc.to {
				assert.False(t, change.Changed)
				assert.Equal(t, before, readFile(t, filepath.Join(dir, "pyproject.toml")))
			}
		})
	}

	// A 3.1 project without the cap has had it taken out by someone, and a
	// patch bump inside 3.1 is not the moment to put it back.
	t.Run("3.1 without the cap to 3.1.8", func(t *testing.T) {
		dir, _ := writeEditFixture(t,
			"[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nairflow = '3.1'\n", 0o644)

		change, err := SetAirflowVersion(dir, nil, "3.1.8")
		require.NoError(t, err)
		assert.False(t, change.SQLAlchemyCapAdded)
		assert.Nil(t, loadConstraints(t, dir))
	})
}
