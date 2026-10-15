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
		name, err := bt.BundleName(dbt)
		require.NoError(t, err)
		assert.Equal(t, "jaffle_shop", name)
		require.True(t, bt.HasDefaultMountPath())
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
		assert.Equal(t, BundleTypeNone, bt.Name)
		assert.False(t, bt.HasDefaultMountPath())
		_, err := bt.DefaultMountPath(plain)
		assert.ErrorContains(t, err, "has no default mount path")
	})
	t.Run("any other type is a label with no defaults", func(t *testing.T) {
		for _, name := range []string{"seeds", BundleTypeNone} {
			bt := ResolveBundleType(name, dbt)
			assert.Equal(t, name, bt.Name, "even where a dbt project is")
			assert.False(t, bt.HasDefaultMountPath(), name)
			assert.Nil(t, bt.Validate, name)
		}
	})
	t.Run("a dbt project with no name", func(t *testing.T) {
		nameless := dbtDir(t, "version: 2\n")
		bt := ResolveBundleType("", nameless)
		_, err := bt.DefaultMountPath(nameless)
		assert.ErrorContains(t, err, "dbt project name not found")
	})
	t.Run("a directory called dbt_project.yml is no dbt project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "dbt_project.yml"), 0o755))
		assert.Equal(t, BundleTypeNone, ResolveBundleType("", dir).Name)
	})
	assert.Equal(t, []string{"dbt"}, BundleTypeNames())
}

// A mount path below a type's default directory says the type.
func TestBundleTypeForMountPath(t *testing.T) {
	bt, ok := BundleTypeForMountPath("/usr/local/airflow/dbt/jaffle_shop")
	require.True(t, ok)
	assert.Equal(t, "dbt", bt.Name)
	bt, ok = BundleTypeForMountPath("/usr/local/airflow/dbt/jaffle_shop/")
	require.True(t, ok, "a trailing slash is cleaned away")
	assert.Equal(t, "dbt", bt.Name)
	for _, p := range []string{
		"/usr/local/airflow/dbt/", "/usr/local/airflow/dbt", "/usr/local/airflow/seeds", "",
		"/usr/local/airflow/dbt/../seeds", "/usr/local/airflow/dbt/a/b", "/usr/local/airflow/dbt/a/..",
	} {
		_, ok := BundleTypeForMountPath(p)
		assert.False(t, ok, p)
	}
}
