package airflowrt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaffold_Airflow3_Minimal(t *testing.T) {
	dirs, files, err := Scaffold(ScaffoldConfig{
		AirflowVersion:   Airflow3,
		RuntimeImageName: "astrocrpublic.azurecr.io/runtime",
		RuntimeImageTag:  "3.1-12",
	})
	require.NoError(t, err)

	assert.Contains(t, dirs, "dags")
	assert.Contains(t, dirs, "plugins")
	assert.Contains(t, dirs, "include")
	assert.Contains(t, dirs, ".astro")
	assert.NotContains(t, dirs, "tests/dags")

	fileMap := scaffoldFileMap(files)

	// Core files present
	assert.Contains(t, fileMap, ".gitignore")
	assert.Contains(t, fileMap, ".dockerignore")
	assert.Contains(t, fileMap, "Dockerfile")
	assert.Contains(t, fileMap, "requirements.txt")
	assert.Contains(t, fileMap, "packages.txt")
	assert.Contains(t, fileMap, ".env")
	assert.Contains(t, fileMap, "dags/.airflowignore")
	assert.Contains(t, fileMap, "dags/exampledag.py")
	assert.Contains(t, fileMap, ".astro/dag_integrity_exceptions.txt")

	// Optional files absent
	assert.NotContains(t, fileMap, "README.md")
	assert.NotContains(t, fileMap, "airflow_settings.yaml")
	assert.NotContains(t, fileMap, "tests/dags/test_dag_example.py")
	assert.NotContains(t, fileMap, ".astro/config.yaml")

	// Dockerfile content
	assert.Equal(t, "FROM astrocrpublic.azurecr.io/runtime:3.1-12\n", fileMap["Dockerfile"])

	// Gitignore includes standalone dir
	assert.Contains(t, fileMap[".gitignore"], ".astro/standalone/")
}

func TestScaffold_Airflow3_Full(t *testing.T) {
	dirs, files, err := Scaffold(ScaffoldConfig{
		AirflowVersion:      Airflow3,
		RuntimeImageName:    "astrocrpublic.azurecr.io/runtime",
		RuntimeImageTag:     "3.1-12",
		ProjectName:         "my-project",
		IncludeTests:        true,
		IncludeReadme:       true,
		IncludeSettingsYaml: true,
		ClientImage: &ClientImageConfig{
			BaseImageRegistry: "images.astronomer.cloud/baseimages",
			ImageTag:          "3.1-12",
		},
	})
	require.NoError(t, err)

	assert.Contains(t, dirs, "tests/dags")

	fileMap := scaffoldFileMap(files)

	assert.Contains(t, fileMap, "README.md")
	assert.Contains(t, fileMap, "airflow_settings.yaml")
	assert.Contains(t, fileMap, "tests/dags/test_dag_example.py")
	assert.Contains(t, fileMap, ".astro/test_dag_integrity_default.py")
	assert.Contains(t, fileMap, ".astro/config.yaml")
	assert.Contains(t, fileMap, "Dockerfile.client")
	assert.Contains(t, fileMap, "requirements-client.txt")
	assert.Contains(t, fileMap, "packages-client.txt")

	assert.Equal(t, "project:\n  name: my-project\n", fileMap[".astro/config.yaml"])
	assert.True(t, strings.HasPrefix(fileMap["Dockerfile.client"], "FROM images.astronomer.cloud/baseimages"))
}

func TestScaffold_Airflow2(t *testing.T) {
	_, files, err := Scaffold(ScaffoldConfig{
		AirflowVersion:      Airflow2,
		RuntimeImageName:    "quay.io/astronomer/astro-runtime",
		RuntimeImageTag:     "2.9-11",
		IncludeTests:        true,
		IncludeReadme:       true,
		IncludeSettingsYaml: true,
	})
	require.NoError(t, err)

	fileMap := scaffoldFileMap(files)
	assert.Contains(t, fileMap, "Dockerfile")
	assert.Contains(t, fileMap, "dags/exampledag.py")
	assert.Contains(t, fileMap, "README.md")
	assert.Contains(t, fileMap, "airflow_settings.yaml")
	assert.Contains(t, fileMap, "tests/dags/test_dag_example.py")
	assert.NotContains(t, fileMap, "Dockerfile.client") // AF2 has no client files
}

func TestScaffold_MissingImage(t *testing.T) {
	_, _, err := Scaffold(ScaffoldConfig{
		AirflowVersion: Airflow3,
	})
	assert.Error(t, err)
}

func TestScaffold_UnsupportedVersion(t *testing.T) {
	_, _, err := Scaffold(ScaffoldConfig{
		AirflowVersion:   AirflowVersion(99),
		RuntimeImageName: "foo",
		RuntimeImageTag:  "bar",
	})
	assert.Error(t, err)
}

func scaffoldFileMap(files []ScaffoldFile) map[string]string {
	m := make(map[string]string, len(files))
	for _, f := range files {
		m[f.RelPath] = f.Content
	}
	return m
}
