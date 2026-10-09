package utils

import (
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
// before they get here, so the advice leads somewhere.
func EnsureProjectDir(cmd *cobra.Command, args []string) error {
	return ensureProjectDir("Change to an Astro project directory, or run astro init to make this one an Astro project")
}

// EnsureDockerfileProjectDir is EnsureProjectDir for APC deploy, which builds a
// Dockerfile project and has no path for the pyproject.toml project astro init
// writes, so advice to run astro init would only lead back here. Such a project
// reaches APC through its image instead: with a declared Dockerfile, astro
// package builds the whole project (the runtime base's ONBUILD COPY bakes the
// DAGs in) and tags an image carrying the runtime label --image-name requires.
func EnsureDockerfileProjectDir(cmd *cobra.Command, args []string) error {
	return ensureProjectDir("Deploying to APC needs a Dockerfile-based project, one with a .astro/config.yaml, such as a project made with Astro CLI 1.x. From a pyproject.toml project, declare dockerfile = \"Dockerfile\" under [tool.astro] (a Dockerfile FROM an Astro Runtime image), run astro package, and deploy the image it tags with --image-name")
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
