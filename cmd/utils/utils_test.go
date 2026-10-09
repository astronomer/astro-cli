package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
)

// inDir points the project checks at dir, with home as the home directory.
func inDir(t *testing.T, dir, home string) {
	t.Helper()
	prevWorking, prevHome := config.WorkingPath, config.HomePath
	t.Cleanup(func() { config.WorkingPath, config.HomePath = prevWorking, prevHome })
	config.WorkingPath, config.HomePath = dir, home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// A 1.x project: a .astro/config.yaml, and the Dockerfile APC builds.
func write1xProject(t *testing.T, dir string, withDockerfile bool) {
	t.Helper()
	writeFile(t, filepath.Join(dir, config.ConfigDir, config.ConfigFileNameWithExt), "project:\n  name: demo\n")
	if withDockerfile {
		writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM astrocrpublic.azurecr.io/runtime:3.1-1\n")
	}
}

// The project astro init writes.
func writeManifestProject(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = \"demo\"\n\n[tool.astro]\n")
}

func ensure(f func(*cobra.Command, []string) error) error {
	return f(&cobra.Command{}, nil)
}

func TestEnsureProjectDir(t *testing.T) {
	home := t.TempDir()

	t.Run("an unreadable path", func(t *testing.T) {
		inDir(t, "./\000x", home)
		err := ensure(EnsureProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), verifyFailedMsg)
		assert.Contains(t, err.Error(), AstroProjectDirAdvice)
	})

	t.Run("not a project: astro init, which the check then accepts", func(t *testing.T) {
		dir := t.TempDir()
		inDir(t, dir, home)
		err := ensure(EnsureProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), notProjectDirMsg)
		assert.Contains(t, err.Error(), AstroProjectDirAdvice)
		assert.NotContains(t, err.Error(), "dev init", "astro dev init does not exist in v2")

		// Following the advice works by construction: what astro init writes passes.
		writeManifestProject(t, dir)
		assert.NoError(t, ensure(EnsureProjectDir))
	})

	t.Run("a 1.x project", func(t *testing.T) {
		dir := t.TempDir()
		write1xProject(t, dir, false)
		inDir(t, dir, home)
		assert.NoError(t, ensure(EnsureProjectDir))
	})

	t.Run("the home directory is never one, and is not told to run astro init", func(t *testing.T) {
		write1xProject(t, home, false) // ~/.astro/config.yaml is the CLI's settings
		inDir(t, home, home)
		err := ensure(EnsureProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), HomeDirAdvice)
		assert.NotContains(t, err.Error(), "astro init")
	})

	t.Run("inside a project, that project is named", func(t *testing.T) {
		for name, write := range map[string]func(*testing.T, string){
			"pyproject.toml": writeManifestProject,
			"1.x":            func(t *testing.T, d string) { write1xProject(t, d, false) },
		} {
			t.Run(name, func(t *testing.T) {
				proj := t.TempDir()
				write(t, proj)
				sub := filepath.Join(proj, "dags", "sub")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				inDir(t, sub, home)
				err := ensure(EnsureProjectDir)
				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf(EnclosingProjectAdvice, proj))
				assert.NotContains(t, err.Error(), "astro init", "init here would nest a second project")
			})
		}
	})
}

func TestEnsureDockerfileProjectDir(t *testing.T) {
	home := t.TempDir()

	t.Run("an unreadable path", func(t *testing.T) {
		inDir(t, "./\000x", home)
		err := ensure(EnsureDockerfileProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), verifyFailedMsg)
		assert.Contains(t, err.Error(), APCProjectDirAdvice)
	})

	t.Run("not a project: the path a pyproject.toml project has to APC", func(t *testing.T) {
		dir := t.TempDir()
		inDir(t, dir, home)
		err := ensure(EnsureDockerfileProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), notProjectDirMsg)
		assert.Contains(t, err.Error(), APCProjectDirAdvice)
		// APC deploy cannot build the project astro init writes.
		assert.NotContains(t, err.Error(), "astro init")
		assert.Contains(t, err.Error(), "astro package --tag <image>")
		assert.Contains(t, err.Error(), "--image-name <image>")

		// Nor does a pyproject.toml project pass.
		writeManifestProject(t, dir)
		assert.Error(t, ensure(EnsureDockerfileProjectDir))
	})

	t.Run("a .astro/config.yaml with no Dockerfile says the Dockerfile is missing", func(t *testing.T) {
		dir := t.TempDir()
		write1xProject(t, dir, false)
		inDir(t, dir, home)
		err := ensure(EnsureDockerfileProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf(APCNoDockerfileAdvice, dir))
	})

	t.Run("a Dockerfile project", func(t *testing.T) {
		dir := t.TempDir()
		write1xProject(t, dir, true)
		inDir(t, dir, home)
		assert.NoError(t, ensure(EnsureDockerfileProjectDir))
	})

	t.Run("the home directory, whose .astro/config.yaml is the CLI's settings", func(t *testing.T) {
		write1xProject(t, home, true)
		inDir(t, home, home)
		err := ensure(EnsureDockerfileProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), HomeDirAdvice)
	})

	t.Run("inside a Dockerfile project, that project is named", func(t *testing.T) {
		proj := t.TempDir()
		write1xProject(t, proj, true)
		sub := filepath.Join(proj, "dags")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		inDir(t, sub, home)
		err := ensure(EnsureDockerfileProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf(EnclosingProjectAdvice, proj))
	})
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
