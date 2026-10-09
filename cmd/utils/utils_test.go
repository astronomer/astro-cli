package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

// make1x lays out a project the way Astro CLI 1.x made one.
func make1x(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "Dockerfile", "FROM quay.io/astronomer/astro-runtime:12.0.0\n")
	writeFile(t, dir, filepath.Join(".astro", "config.yaml"), "project:\n  name: demo\n")
}

func deployIn(t *testing.T, dir string) {
	t.Helper()
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
}

// isNoProject reports whether err is reported under the kind no_project.
func isNoProject(err error) bool {
	var notFound *project.NotFoundError
	return errors.As(err, &notFound)
}

// A deploy outside a pyproject.toml project gets one of three answers, all
// no_project, by project.Discover's rule.
func TestNoDeployableProject(t *testing.T) {
	prevHome := config.HomePath
	t.Cleanup(func() { config.HomePath = prevHome })
	config.HomePath = t.TempDir()

	t.Run("a 1.x project", func(t *testing.T) {
		dir := t.TempDir()
		make1x(t, dir)
		deployIn(t, dir)
		err := NoDeployableProject(Deploy1xRefusedAstro)
		assert.EqualError(t, err, "this project uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 deploys only pyproject.toml projects. Convert it with astro init, or deploy it with Astro CLI 1.x")
		assert.True(t, isNoProject(err))
	})

	t.Run("below a 1.x project, which it names", func(t *testing.T) {
		dir := t.TempDir()
		make1x(t, dir)
		sub := filepath.Join(dir, "dags")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		deployIn(t, sub)
		err := NoDeployableProject(Deploy1xRefusedAstro)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "this directory is inside a project at "+dir+" that uses the Astro CLI 1.x layout")
		assert.Contains(t, err.Error(), "Convert it with astro init in "+dir+",")
		assert.True(t, isNoProject(err))

		err = NoDeployableProject(Deploy1xRefusedAPC)
		assert.Contains(t, err.Error(), "which Astro CLI v2 does not deploy to Astro Private Cloud. Deploy it with Astro CLI 1.x")
		assert.NotContains(t, err.Error(), "astro init")
	})

	t.Run("a 1.x project keeping a pyproject.toml for its tools", func(t *testing.T) {
		dir := t.TempDir()
		make1x(t, dir)
		writeFile(t, dir, "pyproject.toml", "[tool.ruff]\nline-length = 100\n")
		deployIn(t, dir)
		err := NoDeployableProject(Deploy1xRefusedAstro)
		assert.Contains(t, err.Error(), "this project uses the Astro CLI 1.x layout")
	})

	t.Run("below a pyproject.toml project", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "pyproject.toml", "[project]\nname = \"demo\"\n\n[tool.astro]\n")
		sub := filepath.Join(dir, "dags")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		deployIn(t, sub)
		err := NoDeployableProject(Deploy1xRefusedAstro)
		assert.EqualError(t, err, "this directory is inside the project at "+dir+". Run the deploy from the project directory, "+dir)
		assert.True(t, isNoProject(err))
	})

	t.Run("no project", func(t *testing.T) {
		deployIn(t, t.TempDir())
		err := NoDeployableProject(Deploy1xRefusedAstro)
		assert.EqualError(t, err, notProjectAdvice)
		assert.True(t, isNoProject(err))
	})

	t.Run("a .astro/config.yaml alone is no project", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, filepath.Join(".astro", "config.yaml"), "project:\n  name: demo\n")
		deployIn(t, dir)
		assert.EqualError(t, NoDeployableProject(Deploy1xRefusedAstro), notProjectAdvice)
	})
}

// In the home directory the advice does not suggest astro init, which would
// make all of ~ a project, and the home directory is recognized under another
// spelling too. A Dockerfile beside ~/.astro, where the global config lives,
// does not make it a 1.x project.
func TestNoDeployableProjectInTheHomeDirectory(t *testing.T) {
	prevHome := config.HomePath
	t.Cleanup(func() { config.HomePath = prevHome })
	home := t.TempDir()
	config.HomePath = home
	make1x(t, home)
	for _, dir := range []string{home, home + string(filepath.Separator), filepath.Join(home, ".")} {
		deployIn(t, dir)
		err := NoDeployableProject(Deploy1xRefusedAstro)
		assert.EqualError(t, err, homeDirRefusal)
		assert.True(t, isNoProject(err))
		got, err := Project1xDir(dir)
		assert.NoError(t, err)
		assert.Empty(t, got)
	}

	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(home, link); err == nil {
		deployIn(t, link)
		assert.EqualError(t, NoDeployableProject(Deploy1xRefusedAstro), homeDirRefusal)
	}
}

// A pyproject.toml project is never a 1.x one, whatever 1.x files it still
// has beside it.
func TestProject1xDirIsEmptyForAManifestProject(t *testing.T) {
	dir := t.TempDir()
	make1x(t, dir)
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"demo\"\n\n[tool.astro]\n")
	got, err := Project1xDir(dir)
	assert.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetDefaultDeployDescription(t *testing.T) {
	// Test case where --dags flag is not set
	description := GetDefaultDeployDescription(false)
	assert.Equal(t, "Deployed via <astro deploy>", description)

	// Test case where --dags flag is set
	descriptionWithDags := GetDefaultDeployDescription(true)
	assert.Equal(t, "Deployed via <astro deploy --dags>", descriptionWithDags)
}

func TestChainRunEsExecutesAllFunctionsSuccessfully(t *testing.T) {
	runE1 := func(cmd *cobra.Command, args []string) error {
		return nil
	}
	runE2 := func(cmd *cobra.Command, args []string) error {
		return nil
	}
	chain := ChainRunEs(runE1, runE2)
	err := chain(&cobra.Command{}, []string{})
	assert.NoError(t, err)
}

func TestChainRunEsReturnsErrorIfAnyFunctionFails(t *testing.T) {
	runE1 := func(cmd *cobra.Command, args []string) error {
		return nil
	}
	runE2 := func(cmd *cobra.Command, args []string) error {
		return errors.New("error in runE2")
	}
	chain := ChainRunEs(runE1, runE2)
	err := chain(&cobra.Command{}, []string{})
	assert.Error(t, err)
	assert.Equal(t, "error in runE2", err.Error())
}

func TestChainRunEsStopsExecutionAfterError(t *testing.T) {
	runE1 := func(cmd *cobra.Command, args []string) error {
		return errors.New("error in runE1")
	}
	runE2 := func(cmd *cobra.Command, args []string) error {
		t.FailNow() // This should not be called
		return nil
	}
	chain := ChainRunEs(runE1, runE2)
	err := chain(&cobra.Command{}, []string{})
	assert.Error(t, err)
	assert.Equal(t, "error in runE1", err.Error())
}
