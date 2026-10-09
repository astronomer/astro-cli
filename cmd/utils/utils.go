package utils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

type RunE func(cmd *cobra.Command, args []string) error

// ChainRunEs chains multiple RunE functions together for cleaner composition.
func ChainRunEs(runEs ...RunE) RunE {
	return func(cmd *cobra.Command, args []string) error {
		for _, runE := range runEs {
			if err := runE(cmd, args); err != nil {
				return err
			}
		}
		return nil
	}
}

// EnsureProjectDir fails outside a project directory, with advice to run astro
// init. Its callers on Astro accept the pyproject.toml project astro init writes
// before they get here, so the advice leads somewhere. In the home directory it
// says only to change directory: a project there would take in every file
// under ~.
func EnsureProjectDir(cmd *cobra.Command, args []string) error {
	if config.IsHomeDir(config.WorkingPath) {
		return errors.New(ansi.Red("this is your home directory, not an Astro project directory.\nChange to an Astro project directory\n"))
	}
	return ensureProjectDir("Change to an Astro project directory, or run astro init to make this one an Astro project")
}

// apcAdvice is how a project reaches APC deploy. A pyproject.toml project has
// no path of its own there, so it goes through its image: with a declared
// Dockerfile, astro package builds the whole project (the runtime base's
// ONBUILD COPY bakes the DAGs in) and tags an image carrying the runtime label
// --image-name requires.
const apcAdvice = "Deploying to APC needs a Dockerfile-based project: a .astro/config.yaml and a Dockerfile FROM an Astro Runtime image, such as a project made with Astro CLI 1.x. " +
	"From a pyproject.toml project, declare dockerfile = \"Dockerfile\" under [tool.astro], run astro package --tag <image>, then astro deploy <deployment-id> --image-name <image>"

// EnsureDockerfileProjectDir is EnsureProjectDir for an APC deploy that builds
// an image: it needs a 1.x project and the Dockerfile the build parses at its
// root, and says which of them is missing. It never suggests astro init, whose
// project APC deploy cannot build.
func EnsureDockerfileProjectDir(cmd *cobra.Command, args []string) error {
	if config.IsHomeDir(config.WorkingPath) {
		return errors.New(ansi.Red("this is your home directory, not an Astro project directory.\n" + apcAdvice + "\n"))
	}
	isProjectDir, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, ansi.Red("failed to verify that your working directory is an Astro project.\n"+apcAdvice))
	}
	var missing []string
	if !isProjectDir {
		missing = append(missing, ".astro/config.yaml")
	}
	if _, err := os.Stat(filepath.Join(config.WorkingPath, "Dockerfile")); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, ansi.Red("failed to read the project's Dockerfile.\n"+apcAdvice))
		}
		missing = append(missing, "Dockerfile")
	}
	if len(missing) > 0 {
		return errors.New(ansi.Red("this directory has no " + strings.Join(missing, " and no ") + ", so APC deploy cannot build it.\n" + apcAdvice + "\n"))
	}
	return nil
}

func ensureProjectDir(advice string) error {
	isProjectDir, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, ansi.Red("failed to verify that your working directory is an Astro project.\n"+advice))
	}

	if !isProjectDir {
		return errors.New(ansi.Red("this is not an Astro project directory.\n" + advice + "\n"))
	}

	return nil
}

func GetDefaultDeployDescription(isDagOnlyDeploy bool) string {
	if isDagOnlyDeploy {
		return "Deployed via <astro deploy --dags>"
	}

	return "Deployed via <astro deploy>"
}
