package utils

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
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

// Deploy1xRefused is what a deploy is told when the working directory is in
// a project in the Astro CLI 1.x layout, at dir: where is "this project" when
// the deploy ran in it, and names dir when it ran below it. v2 deploys only
// pyproject.toml projects, on Astro and on Astro Private Cloud alike, and
// astro init converts a 1.x project the same way for both; Astro CLI 1.x
// keeps deploying the 1.x layout, so nobody has to convert a project to keep
// shipping it.
func Deploy1xRefused(where, dir string) string {
	return where + " uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 deploys only pyproject.toml projects. " +
		"Convert it with astro init" + in(dir) + ", or deploy it with Astro CLI 1.x"
}

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

// Where is what a deploy finds at and above its working directory: the
// nearest pyproject.toml project (ManifestDir) or project in the Astro CLI
// 1.x layout (Project1xDir), whichever comes first; both empty for none.
//
// For a pyproject.toml project it carries the manifest the walk loaded, or
// why it did not load (LoadErr: it fails to parse or validate, or cannot be
// read), so nothing after the walk reads the file again.
type Where struct {
	ManifestDir  string
	Project1xDir string
	Manifest     *manifest.Manifest
	LoadErr      error
}

// AtRoot reports whether the walk from dir found a pyproject.toml project at
// dir itself: the deploy runs at the project's root.
func (w Where) AtRoot(dir string) bool {
	return w.ManifestDir != "" && sameDir(w.ManifestDir, dir)
}

// None reports whether the walk found no project at all.
func (w Where) None() bool { return w.ManifestDir == "" && w.Project1xDir == "" }

// Loaded is the manifest of a project the deploy runs at the root of, nil
// when it runs anywhere else or the manifest did not load.
func (w Where) Loaded(dir string) *manifest.Manifest {
	if !w.AtRoot(dir) {
		return nil
	}
	return w.Manifest
}

// Locate walks up from dir to the nearest project, the one rule astro
// deploy, astro remote deploy, APC's deploy and the bundle deploys'
// containment checks decide by, routing included. It is the astro dev stub's
// walk (scaffold.Find1xProject), so the deploy refuses as 1.x exactly where
// the local commands' advice says a 1.x project is. At each directory:
//
//   - a pyproject.toml that is a project's (manifestRoot: scaffold's
//     ManifestPresent) stops the walk, whether its manifest loads or not, so
//     the deploy reports why it did not. A pyproject.toml that parses and has
//     no [tool.astro], a monorepo root's tool settings say, does not;
//   - else a 1.x project (scaffold.Has1xLayout, Is1xProject's other half: a
//     Dockerfile and a .astro/config.yaml that is not the CLI's own settings
//     file) stops it;
//   - else the walk goes on up.
//
// Each directory's pyproject.toml is loaded once.
func Locate(dir string) Where {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Where{}
	}
	for d := abs; ; {
		if root, m, loadErr := manifestRoot(d); root {
			return Where{ManifestDir: d, Manifest: m, LoadErr: loadErr}
		}
		if scaffold.Has1xLayout(d) {
			return Where{Project1xDir: d}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return Where{}
		}
		d = parent
	}
}

// manifestRoot reports whether dir is a pyproject.toml project's root, as
// scaffold.HasManifest has it: a pyproject.toml whose [tool.astro] loads, or
// one that fails to validate, to parse, or to be read, which is a project to
// fix whose deploy reports why. No pyproject.toml, and one that parses with no
// [tool.astro], are not. It returns the manifest it loaded, or why it did not.
func manifestRoot(dir string) (root bool, m *manifest.Manifest, loadErr error) {
	m, err := manifest.Load(filepath.Join(dir, project.Marker))
	if !scaffold.ManifestPresent(err) {
		return false, nil, nil
	}
	return true, m, err
}

// NoDeployableProject is what a deploy says when the working directory is not
// the root of a pyproject.toml project, from Locate's answer where (for the
// working directory):
//
//   - in or below a project in the Astro CLI 1.x layout, Deploy1xRefused, naming
//     that project's directory when the deploy ran below it;
//   - below a pyproject.toml project, to run the deploy from its root;
//   - anywhere else, the no-project advice, which in the home directory does
//     not suggest astro init.
//
// Every one of these is reported under the kind no_project: it unwraps to a
// *project.NotFoundError.
func NoDeployableProject(where Where) error {
	wd := config.WorkingPath
	switch {
	case where.Project1xDir != "":
		says, dir := "this project", ""
		if !sameDir(where.Project1xDir, wd) {
			says, dir = "this directory is inside a project at "+where.Project1xDir+" that", where.Project1xDir
		}
		return &noProjectError{msg: Deploy1xRefused(says, dir), cause: &project.NotFoundError{Start: wd, Project1xDir: where.Project1xDir}}
	case where.ManifestDir != "" && !sameDir(where.ManifestDir, wd):
		return NoProject(fmt.Sprintf("this directory is inside the project at %s. Run the deploy from the project directory, %s", where.ManifestDir, where.ManifestDir))
	case config.IsHomeDir(wd):
		return NoProject(homeDirRefusal)
	}
	return NoProject(notProjectAdvice)
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
