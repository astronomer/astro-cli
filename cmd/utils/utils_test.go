package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
)

func TestEnsureProjectDir(t *testing.T) {
	currentWorkingPath := config.WorkingPath
	fileName := config.ConfigFileNameWithExt
	dirName := config.ConfigDir
	defer func() {
		config.WorkingPath = currentWorkingPath
		config.ConfigFileNameWithExt = fileName
		config.ConfigDir = dirName
	}()
	// error case when file path is not resolvable
	config.WorkingPath = "./\000x"
	err := EnsureProjectDir(&cobra.Command{}, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify that your working directory is an Astro project.\nChange to an Astro project directory, or run astro init to make this one an Astro project")

	// error case when no such file or dir
	config.WorkingPath = "./test"
	err = EnsureProjectDir(&cobra.Command{}, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "this is not an Astro project directory.\nChange to an Astro project directory, or run astro init to make this one an Astro project")
	// astro dev init does not exist in v2
	assert.NotContains(t, err.Error(), "dev init")

	// --output json reports it as no_project
	var notFound *project.NotFoundError
	assert.ErrorAs(t, err, &notFound)

	// success case
	config.WorkingPath = currentWorkingPath
	config.ConfigFileNameWithExt = "utils_test.go"
	config.ConfigDir = ""
	err = EnsureProjectDir(&cobra.Command{}, []string{})
	assert.NoError(t, err)
}

// In the home directory the check does not suggest astro init, which would
// make all of ~ a project, and the home directory is recognized under another
// spelling too.
func TestEnsureProjectDirInTheHomeDirectory(t *testing.T) {
	prevPath, prevHome := config.WorkingPath, config.HomePath
	defer func() { config.WorkingPath, config.HomePath = prevPath, prevHome }()
	home := t.TempDir()
	config.HomePath = home
	for _, dir := range []string{home, home + string(filepath.Separator), filepath.Join(home, ".")} {
		config.WorkingPath = dir
		err := EnsureProjectDir(&cobra.Command{}, nil)
		assert.ErrorContains(t, err, "this is your home directory, not an Astro project directory.\nChange to an Astro project directory")
		assert.NotContains(t, err.Error(), "astro init")
		err = NoDeployableProject(Deploy1xRefusedAstro)
		assert.ErrorContains(t, err, "this is your home directory")
		assert.NotContains(t, err.Error(), "astro init")
	}

	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(home, link); err == nil {
		config.WorkingPath = link
		assert.ErrorContains(t, EnsureProjectDir(&cobra.Command{}, nil), "this is your home directory")
	}

	// Anywhere else, the advice still includes astro init.
	config.WorkingPath = t.TempDir()
	assert.ErrorContains(t, EnsureProjectDir(&cobra.Command{}, nil), "run astro init")
}

// A deploy outside a pyproject.toml project tells a 1.x project what to do
// about its layout, and anywhere else gives the no-project advice. Both are
// no_project failures.
func TestNoDeployableProject(t *testing.T) {
	prev := config.WorkingPath
	defer func() { config.WorkingPath = prev }()
	write := func(dir, name, content string) {
		assert.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	oneX := t.TempDir()
	write(oneX, "Dockerfile", "FROM quay.io/astronomer/astro-runtime:12.0.0\n")
	write(oneX, filepath.Join(".astro", "config.yaml"), "project:\n  name: demo\n")
	configOnly := t.TempDir()
	write(configOnly, filepath.Join(".astro", "config.yaml"), "project:\n  name: demo\n")
	dockerfileAndAstroDir := t.TempDir()
	write(dockerfileAndAstroDir, "Dockerfile", "FROM x\n")
	assert.NoError(t, os.MkdirAll(filepath.Join(dockerfileAndAstroDir, ".astro"), 0o755))

	for _, dir := range []string{oneX, configOnly, dockerfileAndAstroDir} {
		config.WorkingPath = dir
		err := NoDeployableProject(Deploy1xRefusedAstro)
		assert.EqualError(t, err, Deploy1xRefusedAstro, dir)
		var notFound *project.NotFoundError
		assert.ErrorAs(t, err, &notFound)
	}

	config.WorkingPath = t.TempDir()
	err := NoDeployableProject(Deploy1xRefusedAPC)
	assert.ErrorContains(t, err, "this is not an Astro project directory.\nChange to an Astro project directory, or run astro init")
	assert.NotContains(t, err.Error(), "1.x")
	var notFound *project.NotFoundError
	assert.ErrorAs(t, err, &notFound)
}

// A pyproject.toml project is never the 1.x layout, whatever 1.x files it
// still has beside it.
func TestIs1xLayoutIsFalseForAManifestProject(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "config.yaml"), nil, 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	is1x, err := Is1xLayout(dir)
	assert.NoError(t, err)
	assert.False(t, is1x)
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
