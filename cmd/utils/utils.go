package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
	// HomeDirAdvice is either check's advice in the home directory. Its
	// .astro/config.yaml holds the CLI's own settings, and astro init there
	// would make the whole home directory a project and its build context.
	HomeDirAdvice = "This is your home directory, which cannot be an Astro project: its .astro/config.yaml holds the CLI's own settings. Change to a project directory"
	// EnclosingProjectAdvice is either check's advice in a subdirectory of a
	// project, where astro init would nest a second one. %s is the project.
	EnclosingProjectAdvice = "This directory is inside the project at %s: run this from there"

	notProjectDirMsg = "this is not an Astro project directory"
	verifyFailedMsg  = "failed to verify that your working directory is an Astro project"
)

// EnsureProjectDir fails outside an Astro project directory: one with a
// pyproject.toml carrying [tool.astro], or a 1.x project's .astro/config.yaml.
// Accepting both here is what makes its advice to run astro init true: the
// project astro init writes passes.
func EnsureProjectDir(cmd *cobra.Command, args []string) error {
	dir := config.WorkingPath
	// IsProjectDir first: it reports an unreadable path, which HasManifest
	// counts as a manifest to fix.
	isProjectDir, err := config.IsProjectDir(dir)
	if err != nil {
		return verifyFailed(err, AstroProjectDirAdvice)
	}
	if isProjectDir || project.HasManifest(dir) {
		return nil
	}
	return notProjectDir(advice(dir, AstroProjectDirAdvice, isAstroProject))
}

// EnsureDockerfileProjectDir is EnsureProjectDir for APC deploy, which builds
// the Dockerfile at the root of a project with a .astro/config.yaml.
func EnsureDockerfileProjectDir(cmd *cobra.Command, args []string) error {
	dir := config.WorkingPath
	isProjectDir, err := config.IsProjectDir(dir)
	if err != nil {
		return verifyFailed(err, APCProjectDirAdvice)
	}
	if !isProjectDir {
		return notProjectDir(advice(dir, APCProjectDirAdvice, isDockerfileProject))
	}
	hasDockerfile, err := dockerfileExists(dir)
	if err != nil {
		return verifyFailed(err, APCProjectDirAdvice)
	}
	if !hasDockerfile {
		return errors.New(ansi.Red(fmt.Sprintf(APCNoDockerfileAdvice, dir) + "\n"))
	}
	return nil
}

func isAstroProject(dir string) bool {
	isProjectDir, err := config.IsProjectDir(dir)
	return (err == nil && isProjectDir) || project.HasManifest(dir)
}

func isDockerfileProject(dir string) bool {
	isProjectDir, err := config.IsProjectDir(dir)
	if err != nil || !isProjectDir {
		return false
	}
	hasDockerfile, err := dockerfileExists(dir)
	return err == nil && hasDockerfile
}

func dockerfileExists(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, "Dockerfile"))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// advice is what to say in dir, which is not a project: fallback, unless dir
// is the home directory or inside a project isProject recognizes, where
// fallback's advice to make dir a project would be wrong.
func advice(dir, fallback string, isProject func(string) bool) string {
	if isHome(dir) {
		return HomeDirAdvice
	}
	// The home directory never encloses a project: its .astro/config.yaml is
	// the CLI's settings, and a pyproject.toml there is not an invitation to
	// run everything below it from ~.
	enclosing := project.Enclosing(dir, func(d string) bool { return !isHome(d) && isProject(d) })
	if enclosing != "" {
		return fmt.Sprintf(EnclosingProjectAdvice, enclosing)
	}
	return fallback
}

func isHome(dir string) bool {
	return config.HomePath != "" && filepath.Clean(dir) == filepath.Clean(config.HomePath)
}

func verifyFailed(err error, advice string) error {
	return errors.Wrap(err, ansi.Red(verifyFailedMsg+".\n"+advice))
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
