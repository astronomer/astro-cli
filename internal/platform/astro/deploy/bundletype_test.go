package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dbtDir(t *testing.T, project string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dbt_project.yml"), []byte(project), 0o600))
	return dir
}

// The type a bundle deploys as, and the defaults it brings.
func TestResolveBundleType(t *testing.T) {
	dbt := dbtDir(t, "name: jaffle_shop\n")
	plain := t.TempDir()

	t.Run("a dbt project, detected", func(t *testing.T) {
		bt := ResolveBundleType("", dbt)
		assert.Equal(t, "dbt", bt.Name)
		require.NoError(t, bt.Validate(dbt))
		mount, err := bt.DefaultMountPath(dbt)
		require.NoError(t, err)
		assert.Equal(t, "/usr/local/airflow/dbt/jaffle_shop", mount)
	})
	t.Run("dbt asked for, in any case", func(t *testing.T) {
		for _, name := range []string{"dbt", "DBT"} {
			bt := ResolveBundleType(name, plain)
			assert.Equal(t, "dbt", bt.Name, name)
			assert.ErrorContains(t, bt.Validate(plain), "no dbt_project.yml in "+plain, name)
		}
	})
	t.Run("nothing detected", func(t *testing.T) {
		bt := ResolveBundleType("", plain)
		assert.Equal(t, BundleType{Name: BundleTypeNone}, bt)
	})
	t.Run("any other type is a label with no defaults", func(t *testing.T) {
		for _, name := range []string{"seeds", BundleTypeNone} {
			bt := ResolveBundleType(name, dbt)
			assert.Equal(t, BundleType{Name: name}, bt, "even where a dbt project is")
		}
	})
	t.Run("a dbt project with no name", func(t *testing.T) {
		nameless := dbtDir(t, "version: 2\n")
		_, err := ResolveBundleType("", nameless).DefaultMountPath(nameless)
		assert.ErrorContains(t, err, "dbt project name not found")
	})
	t.Run("a directory called dbt_project.yml is no dbt project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "dbt_project.yml"), 0o755))
		assert.Equal(t, BundleTypeNone, ResolveBundleType("", dir).Name)
	})
	assert.Equal(t, []string{"dbt"}, BundleTypeNames())
}
