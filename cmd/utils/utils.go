package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
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

// The advice the project-directory checks give. Shared with the tests, which
// assert what each one must and must not say.
const (
	// AstroProjectDirAdvice is EnsureProjectDir's advice in a directory that is
	// neither a project nor inside one, where astro init is the way forward.
	AstroProjectDirAdvice = "Change to an Astro project directory, or run astro init to make this one an Astro project"
	// APCProjectDirAdvice is EnsureDockerfileProjectDir's. APC deploy builds a
	// Dockerfile project and has no path for the pyproject.toml project astro
	// init writes, so it does not suggest astro init. Such a project reaches
	// APC through its image instead: with a declared Dockerfile, astro package
	// builds the whole project (the runtime base's ONBUILD COPY bakes the DAGs
	// in), and --tag names the image so the deploy can name it back.
	APCProjectDirAdvice = "Deploying to APC needs a Dockerfile-based project: a directory with a Dockerfile and a .astro/config.yaml, such as one made with Astro CLI 1.x. " +
		"From a pyproject.toml project, declare dockerfile = \"Dockerfile\" under [tool.astro] (a Dockerfile FROM an Astro Runtime image), run astro package --tag <image>, then astro deploy --image-name <image>"
	// APCNoDockerfileAdvice is EnsureDockerfileProjectDir's in a directory with
	// a .astro/config.yaml and no Dockerfile, which would otherwise pass here
	// and fail later, parsing the Dockerfile. %s is the directory.
	APCNoDockerfileAdvice = "%s has a .astro/config.yaml but no Dockerfile, which APC deploy builds its image from. Add a Dockerfile FROM an Astro Runtime image, or deploy an image built elsewhere with --image-name"
	// HomeDirAdvice is either check's advice in the home directory when it is
	// no project. A .astro/config.yaml there is the CLI's own settings, and
	// astro init there would make the whole home directory a project and its
	// build context.
	HomeDirAdvice = "This is your home directory, and its .astro/config.yaml holds the CLI's own settings, not a project's. Change to a project directory"
	// EnclosingProjectAdvice is either check's advice in a subdirectory of a
	// project, where astro init would nest a second one. %s is the project.
	EnclosingProjectAdvice = "This directory is inside the project at %s: run this from there"

	notProjectDirMsg = "this is not an Astro project directory"
)

// EnsureProjectDir fails outside an Astro project directory: one with a
// pyproject.toml carrying [tool.astro], or a 1.x project's .astro/config.yaml
// (config.IsAstroProject). Accepting both here is what makes its advice to
// run astro init true: the project astro init writes passes.
func EnsureProjectDir(cmd *cobra.Command, args []string) error {
	return ensureDir(config.IsAstroProject, AstroProjectDirAdvice, nil)
}

// EnsureDockerfileProjectDir is EnsureProjectDir for APC deploy, which builds
// the Dockerfile at the root of a project with a .astro/config.yaml
// (isDockerfileProject).
func EnsureDockerfileProjectDir(cmd *cobra.Command, args []string) error {
	return ensureDir(isDockerfileProject, APCProjectDirAdvice, func(dir string) error {
		// A 1.x project short of its Dockerfile would otherwise be told it is
		// no project at all.
		if is1x, err := config.IsProjectDir(dir); err == nil && is1x {
			return errors.New(ansi.Red(fmt.Sprintf(APCNoDockerfileAdvice, dir) + "\n"))
		}
		return nil
	})
}

// ensureDir is what both checks share: the working directory passes when
// isProject accepts it, the home directory included. An error reading it is
// reported as itself, since it is what to fix, and advice to run astro init
// would send the user to make a project they may have; inside a project
// isProject recognizes, that project is named first. Otherwise the home
// directory gets its own advice, nearly, when set, may say what dir is short
// of, and the advice names the project dir is inside, if there is one, over
// fallback.
func ensureDir(isProject func(string) (bool, error), fallback string, nearly func(string) error) error {
	dir := config.WorkingPath
	ok, err := isProject(dir)
	if ok {
		return nil
	}
	if err != nil {
		if enclosing := enclosingProject(dir, isProject); enclosing != "" {
			return fmt.Errorf("%s\n%w", ansi.Red(notProjectDirMsg+".\n"+fmt.Sprintf(EnclosingProjectAdvice, enclosing)), err)
		}
		return err
	}
	if config.IsHomeDir(dir) {
		return notProjectDir(HomeDirAdvice)
	}
	if nearly != nil {
		if err := nearly(dir); err != nil {
			return err
		}
	}
	if enclosing := enclosingProject(dir, isProject); enclosing != "" {
		return notProjectDir(fmt.Sprintf(EnclosingProjectAdvice, enclosing))
	}
	return notProjectDir(fallback)
}

// isDockerfileProject is the project APC deploy builds: a .astro/config.yaml
// with a Dockerfile beside it.
func isDockerfileProject(dir string) (bool, error) {
	isProjectDir, err := config.IsProjectDir(dir)
	if err != nil || !isProjectDir {
		return false, err
	}
	_, err = os.Stat(filepath.Join(dir, "Dockerfile"))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// enclosingProject is the nearest directory above dir that isProject
// recognizes, or "". An ancestor that cannot be read is not one.
func enclosingProject(dir string, isProject func(string) (bool, error)) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return fileutil.NearestReadableDir(filepath.Dir(abs), isProject)
}

func notProjectDir(advice string) error {
	return errors.New(ansi.Red(notProjectDirMsg + ".\n" + advice + "\n"))
}

func GetDefaultDeployDescription(isDagOnlyDeploy bool) string {
	if isDagOnlyDeploy {
		return "Deployed via <astro deploy --dags>"
	}

	return "Deployed via <astro deploy>"
}
