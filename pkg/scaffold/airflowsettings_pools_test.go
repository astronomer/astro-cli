package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

const settingsPoolsOnly = `airflow:
  pools:
    - pool_name: etl
      pool_slot: 4
      pool_description: ETL loads
    - pool_name: ml
      pool_slot: "1"
    - pool_name: unlimited
      pool_slot: -1
`

// Pools go into [tool.astro.pools] as the file wrote them, and a file that
// held only pools has nothing left in it, so it goes.
func TestPoolsAreCarriedIntoTheManifest(t *testing.T) {
	dir := project1xWithSettings(t, settingsPoolsOnly)

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Equal(t, map[string]manifest.Pool{
		"etl":       {Slots: 4, Description: "ETL loads"},
		"ml":        {Slots: 1},
		"unlimited": {Slots: manifest.UnlimitedPoolSlots},
	}, m.Astro.Pools)

	require.NoFileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Contains(t, res.Deleted, SettingsRelPath+" (migrated into pyproject.toml, removed)")
	require.Contains(t, res.Created, "pyproject.toml (migrated 3 pools from "+SettingsRelPath+" into [tool.astro.pools])")
}

// The adopt arm writes them too.
func TestPoolsAreCarriedIntoAnAdoptedManifest(t *testing.T) {
	dir := project1xWithSettings(t, "airflow:\n  pools:\n    - pool_name: etl\n      pool_slot: 4\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = 'adopted'\n"), 0o600))

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Equal(t, map[string]manifest.Pool{"etl": {Slots: 4}}, m.Astro.Pools)
	require.Contains(t, res.Updated, "pyproject.toml (migrated 1 pool from "+SettingsRelPath+" into [tool.astro.pools])")
}

// A pool Airflow could not take is not carried, and the file stays as its
// only record.
func TestAPoolThatCannotBeCarriedKeepsTheFile(t *testing.T) {
	for _, tc := range []struct {
		name, settings, note string
		wantPools            map[string]manifest.Pool
	}{
		{
			name:     "no slots",
			settings: "airflow:\n  pools:\n    - pool_name: etl\n",
			note:     SettingsRelPath + ": pool etl cannot be carried. pool_slot is missing",
		},
		{
			name:     "zero slots",
			settings: "airflow:\n  pools:\n    - pool_name: etl\n      pool_slot: 0\n",
			note:     SettingsRelPath + ": pool etl cannot be carried. pool_slot 0 is not a slot count: use a number above zero, or -1 for no limit",
		},
		{
			name:     "slots that are not a number",
			settings: "airflow:\n  pools:\n    - pool_name: etl\n      pool_slot: four\n",
			note:     SettingsRelPath + ": pool etl cannot be carried. pool_slot \"four\" is not a number",
		},
		{
			name:      "listed twice",
			settings:  "airflow:\n  pools:\n    - pool_name: etl\n      pool_slot: 4\n    - pool_name: etl\n      pool_slot: 5\n",
			note:      SettingsRelPath + ": pool etl is listed twice, and only the first was carried. Delete one entry",
			wantPools: map[string]manifest.Pool{"etl": {Slots: 4}},
		},
		{
			name:     "a slash in the name",
			settings: "airflow:\n  pools:\n    - pool_name: etl/eu\n      pool_slot: 4\n",
			note:     SettingsRelPath + ": pool etl/eu cannot be carried: a pool name cannot contain a slash",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := project1xWithSettings(t, tc.settings)
			cs, err := Plan(dir, Options{})
			require.NoError(t, err)
			res, err := cs.Apply()
			require.NoError(t, err)

			require.Contains(t, res.Notes, tc.note)
			require.FileExists(t, filepath.Join(dir, SettingsRelPath))
			m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			if tc.wantPools == nil {
				require.Empty(t, m.Astro.Pools)
				return
			}
			require.Equal(t, tc.wantPools, m.Astro.Pools)
		})
	}
}

// Airflow does not let a caller change default_pool's description, so its
// slots are carried and the run says what it left out.
func TestDefaultPoolIsCarriedWithoutItsDescription(t *testing.T) {
	dir := project1xWithSettings(t, "airflow:\n  pools:\n    - pool_name: default_pool\n      pool_slot: 64\n      pool_description: mine\n")
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Equal(t, map[string]manifest.Pool{"default_pool": {Slots: 64}}, m.Astro.Pools)
	require.Contains(t, res.Advisories, SettingsRelPath+": carried default_pool's slots and not its description, "+
		"which Airflow does not let a caller change")
}

// A fault elsewhere in the file keeps the file and carries no declarations,
// but its pools still reach the manifest: they declare nothing in
// [tool.astro.env], which is what the all-or-nothing rule protects.
func TestPoolsAreCarriedWhenTheRestOfTheFileIsNot(t *testing.T) {
	dir := project1xWithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
  pools:
    - pool_name: etl
      pool_slot: 4
`)
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Equal(t, map[string]manifest.Pool{"etl": {Slots: 4}}, m.Astro.Pools)
	require.Nil(t, m.Astro.Env)
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
}

// A pool that cannot be carried keeps the file, and nothing else: the
// connections and variables beside it still reach the vault.
func TestAPoolThatCannotBeCarriedDoesNotStopTheValues(t *testing.T) {
	dir := project1xWithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: postgres
      conn_password: hunter2
  pools:
    - pool_name: etl
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Contains(t, writer.stored, "warehouse")
	require.Contains(t, res.Notes, SettingsRelPath+": pool etl cannot be carried. pool_slot is missing")
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Equal(t,
		SettingsRelPath+" still contains warehouse in plaintext, and is kept for a pool entry that could not be carried",
		findAdvisory(t, res.Advisories, "plaintext"))
}
