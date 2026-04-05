package airflowrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePackageVersion(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "constraints.txt")
	content := "apache-airflow==2.10.0\napache-airflow-task-sdk==1.0.0\nother-package==3.2.1\n"
	require.NoError(t, os.WriteFile(f, []byte(content), 0o644))

	v, err := ParsePackageVersion(f, "apache-airflow")
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", v)

	v, err = ParsePackageVersion(f, "apache-airflow-task-sdk")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", v)

	_, err = ParsePackageVersion(f, "nonexistent")
	assert.Error(t, err)
}

func TestFetchConstraints_Cached(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, StandaloneDir)
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	tag := "3.1-12"
	python := "3.12"

	constraints := "apache-airflow==2.10.0\napache-airflow-task-sdk==1.0.0\n"
	freeze := "apache-airflow==2.10.0\nsomething-else==1.2.3\n"

	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "constraints-3.1-12-python-3.12.txt"),
		[]byte(constraints), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "freeze-3.1-12-python-3.12.txt"),
		[]byte(freeze), 0o644))

	result, err := FetchConstraints(dir, tag, python)
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", result.AirflowVersion)
	assert.Equal(t, "1.0.0", result.TaskSDKVersion)
	assert.Contains(t, result.FreezePath, "freeze-3.1-12-python-3.12.txt")
}

func TestFetchConstraints_DefaultPython(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, StandaloneDir)
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	constraints := "apache-airflow==2.10.0\n"
	freeze := "apache-airflow==2.10.0\n"

	// Write with default python version
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "constraints-3.1-12-python-3.12.txt"),
		[]byte(constraints), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "freeze-3.1-12-python-3.12.txt"),
		[]byte(freeze), 0o644))

	// Pass empty python version — should default to 3.12
	result, err := FetchConstraints(dir, "3.1-12", "")
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", result.AirflowVersion)
}
