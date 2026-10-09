package utils

import (
	"errors"
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
	assert.Contains(t, err.Error(), "this is not an Astro project directory.\nDeploying to APC needs a Dockerfile-based project, one with a .astro/config.yaml")
	assert.NotContains(t, err.Error(), "astro init")
	// it points a pyproject.toml project at the path that does reach APC
	assert.Contains(t, err.Error(), `declare dockerfile = "Dockerfile" under [tool.astro]`)
	assert.Contains(t, err.Error(), "run astro package, and deploy the image it tags with --image-name")

	// success case
	config.WorkingPath = currentWorkingPath
	config.ConfigFileNameWithExt = "utils_test.go"
	config.ConfigDir = ""
	err = EnsureProjectDir(&cobra.Command{}, []string{})
	assert.NoError(t, err)
	err = EnsureDockerfileProjectDir(&cobra.Command{}, []string{})
	assert.NoError(t, err)
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
