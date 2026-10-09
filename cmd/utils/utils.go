package utils

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
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

// Refusal1x is what a deploy says when the working directory is in a
// project in the Astro CLI 1.x layout, at dir: where is "this project" when
// the deploy ran in it, and names dir when it ran below it.
type Refusal1x func(where, dir string) string

// What a deploy from a project in the Astro CLI 1.x layout is told. v2
// deploys only pyproject.toml projects, and Astro CLI 1.x keeps deploying the
// 1.x layout, so nobody has to convert a project to keep shipping it.
//
// On Astro Private Cloud the advice is Astro CLI 1.x alone: v2 builds no
// project there yet, and astro init refuses to convert a project under an
// APC context, to keep its 1.x deploys working.
var (
	Deploy1xRefusedAstro Refusal1x = func(where, dir string) string {
		return where + " uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 deploys only pyproject.toml projects. " +
			"Convert it with astro init" + in(dir) + ", or deploy it with Astro CLI 1.x"
	}
	Deploy1xRefusedAPC Refusal1x = func(where, _ string) string {
		return where + " uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), which Astro CLI v2 does not deploy to Astro Private Cloud. " +
			"Deploy it with Astro CLI 1.x"
	}
)

func in(dir string) string {
	if dir == "" {
		return ""
	}
	return " in " + dir
}

// The no-project advice: the home directory, where astro init would make
// every file under ~ part of a project, and anywhere else.
const (
	homeDirRefusal   = "this is your home directory, not an Astro project directory. Change to an Astro project directory"
	notProjectAdvice = "this is not an Astro project directory. Change to an Astro project directory, or run astro init to make this one an Astro project"
)

// NoDeployableProject is what a deploy says when the working directory is not
// the root of a pyproject.toml project, the one kind of project v2 deploys:
//
//   - in or below a project in the Astro CLI 1.x layout, refused1x, naming
//     that project's directory when the deploy ran below it;
//   - below a pyproject.toml project, to run the deploy from its root;
//   - anywhere else, the no-project advice, which in the home directory does
//     not suggest astro init.
//
// Which directory holds what is project.Discover's answer, the one the
// local commands give. Every one of these is reported under the kind
// no_project: it unwraps to a *project.NotFoundError.
func NoDeployableProject(refused1x Refusal1x) error {
	wd := config.WorkingPath
	if config.IsHomeDir(wd) {
		return NoProject(homeDirRefusal)
	}
	found, err := discover(wd)
	if err != nil {
		return err
	}
	switch {
	case found.dir1x != "":
		where, dir := "this project", ""
		if !sameDir(found.dir1x, wd) {
			where, dir = "this directory is inside a project at "+found.dir1x+" that", found.dir1x
		}
		return &noProjectError{msg: refused1x(where, dir), cause: &project.NotFoundError{Start: wd, Project1xDir: found.dir1x}}
	case found.manifestDir != "" && !sameDir(found.manifestDir, wd):
		return NoProject(fmt.Sprintf("this directory is inside the project at %s. Run the deploy from the project directory, %s", found.manifestDir, found.manifestDir))
	}
	return NoProject(notProjectAdvice)
}

// Project1xDir is the directory of the project in the Astro CLI 1.x layout
// that dir is in or below, by project.Discover's rule, or "" when there is
// none: dir is in a pyproject.toml project, or in no project at all.
func Project1xDir(dir string) (string, error) {
	found, err := discover(dir)
	return found.dir1x, err
}

type discovered struct {
	// manifestDir is the pyproject.toml project dir is in or below.
	manifestDir string
	// dir1x is the 1.x project dir is in or below, when there is no
	// pyproject.toml project nearer.
	dir1x string
}

// discover walks up from dir as project.Discover does. A pyproject.toml with
// no [tool.astro] is not a project, and is a 1.x one when the 1.x layout is
// beside it (project.LoadError's rule).
func discover(dir string) (discovered, error) {
	if config.IsHomeDir(dir) {
		// ~/.astro holds the global config, so with a Dockerfile in ~ the
		// home directory would pass for a 1.x project.
		return discovered{}, nil
	}
	p, err := project.Discover(dir)
	var notFound *project.NotFoundError
	switch {
	case errors.As(err, &notFound):
		if config.IsHomeDir(notFound.Project1xDir) {
			return discovered{}, nil
		}
		return discovered{dir1x: notFound.Project1xDir}, nil
	case err != nil:
		return discovered{}, err
	}
	_, err = manifest.Load(filepath.Join(p.Dir, project.Marker))
	var noSection *project.NoAstroSectionError
	if lerr := project.LoadError(dir, p.Dir, err); errors.As(lerr, &noSection) {
		if noSection.Has1xProject {
			return discovered{dir1x: p.Dir}, nil
		}
		return discovered{}, nil
	}
	// A manifest that fails to load otherwise is still a project to fix.
	return discovered{manifestDir: p.Dir}, nil
}

func sameDir(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB)
}

// noProjectError carries its own words, and unwraps to the
// *project.NotFoundError that cmd/local's ProblemKinds reports as no_project.
type noProjectError struct {
	msg   string
	cause error
}

func (e *noProjectError) Error() string { return e.msg }
func (e *noProjectError) Unwrap() error { return e.cause }

// NoProject is a refusal for want of a project here, saying msg, reported
// under the kind no_project.
func NoProject(msg string) error {
	return &noProjectError{msg: msg, cause: &project.NotFoundError{Start: config.WorkingPath}}
}

func GetDefaultDeployDescription(isDagOnlyDeploy bool) string {
	if isDagOnlyDeploy {
		return "Deployed via <astro deploy --dags>"
	}

	return "Deployed via <astro deploy>"
}
