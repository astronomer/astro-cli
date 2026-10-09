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
	"github.com/astronomer/astro-cli/pkg/manifest"
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

// A manifest project passes whatever state the .astro beside it is in: the
// manifest is read first, so a .astro that is a file (ENOTDIR on its
// config.yaml) does not turn the check into a failure.
func TestEnsureProjectDirReadsTheManifestFirst(t *testing.T) {
	dir := t.TempDir()
	writeManifestProject(t, dir)
	writeFile(t, filepath.Join(dir, config.ConfigDir), "")
	inDir(t, dir, t.TempDir())
	assert.NoError(t, ensure(EnsureProjectDir))
}

// A manifest that loads, as astro init writes it once its Airflow is pinned.
const loadableManifest = "[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n"

// The home directory is a project only with a manifest that loads, as in a
// container whose WORKDIR is $HOME. Its .astro/config.yaml is the CLI's own
// settings, and a manifest there that does not load is refused with the home
// directory's advice, not taken for a project. APC deploy, which builds a
// .astro/config.yaml project, never accepts it.
func TestEnsureProjectDirInTheHomeDirectory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string // pyproject.toml; "" writes none
		accept  bool
	}{
		{name: "a manifest that loads", content: loadableManifest, accept: true},
		{name: "a manifest that fails validation", content: "[project]\nname = \"demo\"\n\n[tool.astro]\n"},
		{name: "a pyproject that fails to parse", content: "this is not : valid = toml [[[\n"},
		{name: "a [tool.astro] with a typo", content: "[project]\nname = \"demo\"\n\n[tool.astro\n"},
		{name: "only .astro/config.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			write1xProject(t, home, true) // ~/.astro/config.yaml, the CLI's settings
			if tc.content != "" {
				writeFile(t, filepath.Join(home, "pyproject.toml"), tc.content)
			}
			inDir(t, home, home)
			err := ensure(EnsureProjectDir)
			if tc.accept {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), HomeDirAdvice)
			}
			err = ensure(EnsureDockerfileProjectDir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), HomeDirAdvice)
		})
	}
}

// A pyproject.toml that does not parse is reported as itself, as the deploy
// that loads it would report it, not with advice to run astro init in a
// project the user already has.
func TestEnsureProjectDirReportsAManifestThatDoesNotParse(t *testing.T) {
	for name, content := range map[string]string{
		"a syntax error": "[project]\nname = \"demo\"\n\n[tool.astro\n",
		"a type error":   "[project]\nname = 3\n\n[tool.astro]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "pyproject.toml"), content)
			inDir(t, dir, t.TempDir())
			err := ensure(EnsureProjectDir)
			require.Error(t, err)
			var parseErr *manifest.ParseError
			require.ErrorAs(t, err, &parseErr)
			assert.Contains(t, err.Error(), filepath.Join(dir, "pyproject.toml"))
			assert.NotContains(t, err.Error(), "astro init")
			assert.NotContains(t, err.Error(), verifyFailedMsg)
		})
	}
}

// The home directory is the same directory however it is reached. HOME
// through a symlink names the directory the working path names directly.
func TestEnsureProjectDirKnowsHomeThroughASymlink(t *testing.T) {
	home := t.TempDir()
	link := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.Symlink(home, link))
	write1xProject(t, home, false)
	inDir(t, home, link)
	err := ensure(EnsureProjectDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), HomeDirAdvice)

	writeFile(t, filepath.Join(home, "pyproject.toml"), loadableManifest)
	assert.NoError(t, ensure(EnsureProjectDir), "a manifest that loads in ~ is a project, through the symlink too")
	require.NoError(t, os.Remove(filepath.Join(home, "pyproject.toml")))

	// And a project below it is not told it is inside ~.
	sub := filepath.Join(home, "elsewhere")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	inDir(t, sub, link)
	err = ensure(EnsureProjectDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), AstroProjectDirAdvice)
}

// An ancestor whose pyproject.toml cannot be read or parsed is not named as
// the project above: nothing says it is one.
func TestEnsureProjectDirDoesNotNameAMalformedAncestor(t *testing.T) {
	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, "pyproject.toml"), "this is not : valid = toml [[[\n")
	sub := filepath.Join(parent, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	inDir(t, sub, t.TempDir())
	err := ensure(EnsureProjectDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), AstroProjectDirAdvice)
	assert.NotContains(t, err.Error(), parent)
}

// Which parent counts as the project above depends on the check, and each
// check's advice uses its own: Astro's is project.IsAstroProject, the same one
// astro init's nested warning uses; APC's needs the Dockerfile it builds.
func TestEnsureAdviceNamesTheChecksOwnProjects(t *testing.T) {
	home := t.TempDir()
	below := func(t *testing.T, write func(string)) (parent, sub string) {
		t.Helper()
		parent = t.TempDir()
		write(parent)
		sub = filepath.Join(parent, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		return parent, sub
	}

	t.Run("a .astro/config.yaml and no Dockerfile", func(t *testing.T) {
		parent, sub := below(t, func(d string) { write1xProject(t, d, false) })
		inDir(t, sub, home)
		err := ensure(EnsureProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf(EnclosingProjectAdvice, parent))
		err = ensure(EnsureDockerfileProjectDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), APCProjectDirAdvice, "APC cannot build that parent")
	})

	t.Run("a Dockerfile and a bare .astro", func(t *testing.T) {
		parent, sub := below(t, func(d string) {
			writeFile(t, filepath.Join(d, "Dockerfile"), "FROM x\n")
			require.NoError(t, os.MkdirAll(filepath.Join(d, config.ConfigDir), 0o755))
		})
		inDir(t, sub, home)
		for check, fallback := range map[string]string{"astro": AstroProjectDirAdvice, "apc": APCProjectDirAdvice} {
			f := EnsureProjectDir
			if check == "apc" {
				f = EnsureDockerfileProjectDir
			}
			err := ensure(f)
			require.Error(t, err)
			assert.Contains(t, err.Error(), fallback, check)
			assert.NotContains(t, err.Error(), parent, check)
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
