package deploybuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// sourceProject writes a project pinning Airflow 3.1 with the given
// [tool.astro] body, and a Dockerfile when from is set.
func sourceProject(t *testing.T, astro, from string) (string, *manifest.Manifest) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
		[]byte("[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"+astro), 0o600))
	if from != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM "+from+"\n"), 0o600))
	}
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	return dir, m
}

// A [tool.astro] runtime is held to the runtime check the caller supplies,
// and the check's warnings name the key.
func TestCheckSourceChecksTheRuntimeBuild(t *testing.T) {
	dir, m := sourceProject(t, "runtime = '3.1-12'\n", "")
	var warned []string
	var checked [][2]string
	_, survey, err := CheckSource(&Source{
		ProjectDir: dir,
		Manifest:   m,
		Warn:       func(w string) { warned = append(warned, w) },
		CheckRuntime: func(runtime, pin string) ([]runtimeversions.Finding, error) {
			checked = append(checked, [2]string{runtime, pin})
			return []runtimeversions.Finding{{Kind: runtimeversions.FindingYanked, Message: "yanked"}}, nil
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, survey, "the survey is returned for the build's own checks")
	assert.Equal(t, [][2]string{{"3.1-12", "3.1"}}, checked)
	assert.Equal(t, []string{"pyproject.toml: tool.astro.runtime: yanked"}, warned)
}

// A deploy that builds nothing from the manifest, and one with no manifest,
// are checked for nothing: a Dockerfile FROM another Airflow, which an image
// built from the manifest is refused for, goes through, and nothing is
// surveyed or asked.
func TestCheckSourceSkipsWhatBuildsNothing(t *testing.T) {
	dir, m := sourceProject(t, "dockerfile = 'Dockerfile'\n", "astrocrpublic.azurecr.io/runtime:3.3-8")
	// Neither is meant to run; one that does fails the test.
	check := func(string, string) ([]runtimeversions.Finding, error) { //nolint:unparam // never called
		t.Error("the runtime check ran")
		return nil, nil
	}
	engine := func() (string, []string, error) { //nolint:unparam // never called
		t.Error("the engine was resolved")
		return "", nil, nil
	}
	for name, s := range map[string]Source{
		"--dags":       {DagsOnly: true, Manifest: m},
		"--image-name": {ImageName: "prebuilt:1", Manifest: m},
		"no manifest":  {},
	} {
		t.Run(name, func(t *testing.T) {
			s.ProjectDir, s.CheckRuntime, s.Engine = dir, check, engine
			s.Warn = func(w string) { t.Errorf("warned: %s", w) }
			missing, survey, err := CheckSource(&s)
			require.NoError(t, err)
			assert.Nil(t, survey)
			assert.Zero(t, missing)
		})
	}

	_, _, err := CheckSource(&Source{ProjectDir: dir, Manifest: m})
	require.Error(t, err, "the same project is refused when its image is built")
}

// A declared Dockerfile FROM another Airflow than the pin is refused.
func TestCheckSourceRefusesADockerfileOfAnotherAirflow(t *testing.T) {
	dir, m := sourceProject(t, "dockerfile = 'Dockerfile'\n", "astrocrpublic.azurecr.io/runtime:3.3-8")
	_, _, err := CheckSource(&Source{ProjectDir: dir, Manifest: m})
	var ve *manifest.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, manifest.CodeDockerfileAirflowMismatch, ve.Problems[0].Code)
}
