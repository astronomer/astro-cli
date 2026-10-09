package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/config"
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

	// APC deploy cannot use the project astro init writes, so it is not told to run it
	err = EnsureDockerfileProjectDir(&cobra.Command{}, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "this directory has no .astro/config.yaml and no Dockerfile, so APC deploy cannot build it.\nDeploying to APC needs a Dockerfile-based project")
	assert.NotContains(t, err.Error(), "astro init")
	// it points a pyproject.toml project at the path that does reach APC
	assert.Contains(t, err.Error(), `declare dockerfile = "Dockerfile" under [tool.astro]`)
	assert.Contains(t, err.Error(), "run astro package --tag <image>, then astro deploy <deployment-id> --image-name <image>")

	// success case
	config.WorkingPath = currentWorkingPath
	config.ConfigFileNameWithExt = "utils_test.go"
	config.ConfigDir = ""
	err = EnsureProjectDir(&cobra.Command{}, []string{})
	assert.NoError(t, err)
}

// An APC image deploy builds the Dockerfile at the project root, so a 1.x
// project without one is refused, naming only what is missing.
func TestEnsureDockerfileProjectDirNamesWhatIsMissing(t *testing.T) {
	prev := config.WorkingPath
	defer func() { config.WorkingPath = prev }()
	dir := t.TempDir()
	config.WorkingPath = dir
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, config.ConfigDir), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, config.ConfigDir, config.ConfigFileNameWithExt), []byte("project:\n  name: demo\n"), 0o600))

	err := EnsureDockerfileProjectDir(&cobra.Command{}, nil)
	assert.ErrorContains(t, err, "this directory has no Dockerfile, so APC deploy cannot build it")
	assert.NotContains(t, err.Error(), "no .astro/config.yaml")

	assert.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
	assert.NoError(t, EnsureDockerfileProjectDir(&cobra.Command{}, nil))
}

// In the home directory neither check suggests astro init, which will not
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
		err = EnsureDockerfileProjectDir(&cobra.Command{}, nil)
		assert.ErrorContains(t, err, "this is your home directory")
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
