package utils

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
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
		return noProject(homeDirRefusal)
	}
	isProjectDir, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, ansi.Red("failed to verify that your working directory is an Astro project.\n"+projectDirAdvice))
	}
	if !isProjectDir {
		return noProject("this is not an Astro project directory.\n" + projectDirAdvice + "\n")
	}
	return nil
}

const projectDirAdvice = "Change to an Astro project directory, or run astro init to make this one an Astro project"

// homeDirRefusal is the no-project advice in the home directory, where astro
// init would make every file under ~ part of a project.
const homeDirRefusal = "this is your home directory, not an Astro project directory.\nChange to an Astro project directory\n"

// NoDeployableProject is what a deploy says when the working directory holds
// no pyproject.toml project: msg1x when it holds a project in the Astro CLI
// 1.x layout, and EnsureProjectDir's advice otherwise. Both are reported under
// the kind no_project.
func NoDeployableProject(msg1x string) error {
	if config.IsHomeDir(config.WorkingPath) {
		return noProject(homeDirRefusal)
	}
	is1x, err := Is1xLayout(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, ansi.Red("failed to verify that your working directory is an Astro project.\n"+projectDirAdvice))
	}
	if is1x {
		return &noProjectError{msg: msg1x, cause: &project.NotFoundError{Start: config.WorkingPath, Project1xDir: config.WorkingPath}}
	}
	return noProject("this is not an Astro project directory.\n" + projectDirAdvice + "\n")
}

// What a deploy from a project in the Astro CLI 1.x layout is told. v2 deploys
// only pyproject.toml projects, and Astro CLI 1.x keeps deploying the 1.x
// layout, so nobody has to convert a project to keep shipping it.
const (
	Deploy1xRefusedAstro = "this project uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 deploys only pyproject.toml projects. " +
		"Convert it with astro init, or deploy it with Astro CLI 1.x"
	Deploy1xRefusedAPC = "this project uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 cannot deploy projects to Astro Private Cloud yet. " +
		"Deploy it with Astro CLI 1.x for now"
)

// Is1xLayout reports whether dir holds a project in the Astro CLI 1.x layout
// and no pyproject.toml project: a Dockerfile beside a .astro directory
// (project.Is1xProject), or a .astro/config.yaml (config.IsProjectDir), which
// only 1.x writes.
func Is1xLayout(dir string) (bool, error) {
	// ~/.astro holds the global config, so with a Dockerfile in ~ the home
	// directory would pass for a 1.x project.
	if config.IsHomeDir(dir) || project.HasManifest(dir) {
		return false, nil
	}
	if project.Is1xProject(dir) {
		return true, nil
	}
	return config.IsProjectDir(dir)
}

// noProject is a refusal for want of a project here, saying msg in red, as
// these checks always have.
func noProject(msg string) error {
	return &noProjectError{msg: ansi.Red(msg), cause: &project.NotFoundError{Start: config.WorkingPath}}
}

// noProjectError carries its own words, and unwraps to the
// *project.NotFoundError that cmd/local's ProblemKinds reports as no_project.
type noProjectError struct {
	msg   string
	cause error
}

func (e *noProjectError) Error() string { return e.msg }
func (e *noProjectError) Unwrap() error { return e.cause }

func GetDefaultDeployDescription(isDagOnlyDeploy bool) string {
	if isDagOnlyDeploy {
		return "Deployed via <astro deploy --dags>"
	}

	return "Deployed via <astro deploy>"
}
