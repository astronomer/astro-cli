package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDeployIgnore(t *testing.T, root, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".astro", "deployignore"), []byte(content), 0o644))
}

func TestReadDeployIgnorePatterns(t *testing.T) {
	t.Run("returns nil when no deploy ignore file exists", func(t *testing.T) {
		patterns, err := readDeployIgnorePatterns(t.TempDir())
		assert.NoError(t, err)
		assert.Nil(t, patterns)
	})

	t.Run("parses patterns, skipping comments and blank lines", func(t *testing.T) {
		dir := t.TempDir()
		writeDeployIgnore(t, dir, "# internal docs\n\ndocs/\n*.md\n!README.md\n")

		patterns, err := readDeployIgnorePatterns(dir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"docs", "*.md", "!README.md"}, patterns)
	})
}

func TestDeployIgnoreSkipFunc(t *testing.T) {
	t.Run("returns nil skip func when no deploy ignore file exists", func(t *testing.T) {
		dir := t.TempDir()
		skip, err := deployIgnoreSkipFunc(dir, dir)
		assert.NoError(t, err)
		assert.Nil(t, skip)
	})

	t.Run("returns nil skip func when the file has no patterns", func(t *testing.T) {
		dir := t.TempDir()
		writeDeployIgnore(t, dir, "# only comments\n\n")

		skip, err := deployIgnoreSkipFunc(dir, dir)
		assert.NoError(t, err)
		assert.Nil(t, skip)
	})

	t.Run("matches relative to the root when bundling the root itself", func(t *testing.T) {
		dir := t.TempDir()
		writeDeployIgnore(t, dir, "target/\n*.log\n")

		skip, err := deployIgnoreSkipFunc(dir, dir)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		assert.True(t, skip("target/run_results.json"))
		assert.True(t, skip("debug.log"))
		assert.False(t, skip("models/customers.sql"))
	})

	t.Run("anchors patterns at the project root for a nested bundle", func(t *testing.T) {
		dir := t.TempDir()
		dagsPath := filepath.Join(dir, "dags")
		require.NoError(t, os.Mkdir(dagsPath, 0o755))
		writeDeployIgnore(t, dir, "dags/docs/\n")

		skip, err := deployIgnoreSkipFunc(dir, dagsPath)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		// paths inside the dags bundle are matched with their dags/ prefix
		assert.True(t, skip("docs/internal.md"))
		assert.False(t, skip("my_dag.py"))
	})

	t.Run("does not match unanchored directories of a nested bundle", func(t *testing.T) {
		dir := t.TempDir()
		dagsPath := filepath.Join(dir, "dags")
		require.NoError(t, os.Mkdir(dagsPath, 0o755))
		// docs/ refers to the project-root docs directory, not dags/docs/
		writeDeployIgnore(t, dir, "docs/\n")

		skip, err := deployIgnoreSkipFunc(dir, dagsPath)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		assert.False(t, skip("docs/internal.md"))
	})

	t.Run("negation patterns re-include files", func(t *testing.T) {
		dir := t.TempDir()
		dagsPath := filepath.Join(dir, "dags")
		require.NoError(t, os.Mkdir(dagsPath, 0o755))
		writeDeployIgnore(t, dir, "dags/docs/\n!dags/docs/keep.md\n")

		skip, err := deployIgnoreSkipFunc(dir, dagsPath)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		assert.True(t, skip("docs/internal.md"))
		assert.False(t, skip("docs/keep.md"))
	})

	t.Run("matches relative to the bundle root when the bundle is outside the root", func(t *testing.T) {
		dir := t.TempDir()
		outsidePath := filepath.Join(t.TempDir(), "external-dags")
		require.NoError(t, os.Mkdir(outsidePath, 0o755))
		writeDeployIgnore(t, dir, "*.md\n")

		skip, err := deployIgnoreSkipFunc(dir, outsidePath)
		assert.NoError(t, err)
		assert.NotNil(t, skip)

		assert.True(t, skip("notes.md"))
		assert.False(t, skip("my_dag.py"))
	})

	t.Run("returns an error for invalid patterns", func(t *testing.T) {
		dir := t.TempDir()
		writeDeployIgnore(t, dir, "[\n")

		skip, err := deployIgnoreSkipFunc(dir, dir)
		assert.ErrorContains(t, err, "invalid pattern")
		assert.Nil(t, skip)
	})
}
