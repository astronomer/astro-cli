package utils

import (
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
	assert.Contains(t, err.Error(), "failed to verify that your working directory is an Astro project.\nTry running astro dev init to turn your working directory into an Astro project")

	// error case when no such file or dir
	config.WorkingPath = "./test"
	err = EnsureProjectDir(&cobra.Command{}, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "this is not an Astro project directory.\nChange to another directory or run astro dev init to turn your working directory into an Astro project")

	// success case
	config.WorkingPath = currentWorkingPath
	config.ConfigFileNameWithExt = "utils_test.go"
	config.ConfigDir = ""
	err = EnsureProjectDir(&cobra.Command{}, []string{})
	assert.NoError(t, err)
}

func TestGetDefaultDeployDescription(t *testing.T) {
	// Test case where --dags flag is not set
	cmd := &cobra.Command{}
	description := GetDefaultDeployDescription(cmd, []string{})
	assert.Equal(t, "Deploy via <astro deploy>", description)

	// Test case where --dags flag is set
	cmdWithDagsFlag := &cobra.Command{}
	cmdWithDagsFlag.Flags().Bool("dags", true, "")
	cmdWithDagsFlag.Flags().Set("dags", "true")
	descriptionWithDags := GetDefaultDeployDescription(cmdWithDagsFlag, []string{})
	assert.Equal(t, "Deploy via <astro deploy --dags>", descriptionWithDags)
}
