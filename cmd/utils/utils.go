package utils

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

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

// Where is what a deploy finds at and above its working directory: the
// nearest pyproject.toml project (ManifestDir) or project in the Astro CLI
// 1.x layout (Project1xDir), whichever comes first; both empty for none.
type Where struct {
	ManifestDir  string
	Project1xDir string
}

// AtRoot reports whether the walk from dir found a pyproject.toml project at
// dir itself: the deploy runs at the project's root.
func (w Where) AtRoot(dir string) bool {
	return w.ManifestDir != "" && sameDir(w.ManifestDir, dir)
}

// None reports whether the walk found no project at all.
func (w Where) None() bool { return w == Where{} }

// Locate walks up from dir to the nearest project, the one rule astro
// deploy, astro remote deploy and APC's deploy decide by, their routing
// included. At each directory:
//
//   - a pyproject.toml project's root (IsManifestRoot) stops the walk. A
//     pyproject.toml declaring no tool.astro table, a monorepo root's tool
//     settings say, does not;
//   - else a Dockerfile beside a .astro directory is a 1.x project, except
//     in the home directory, whose .astro holds the global config;
//   - else the walk goes on up.
//
// Each directory's pyproject.toml is read once.
func Locate(dir string) Where {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Where{}
	}
	for d := abs; ; {
		if IsManifestRoot(d) {
			return Where{ManifestDir: d}
		}
		// Not a manifest root, so the 1.x layout alone decides: no second
		// read of the pyproject.toml (project.Is1xProject would make one).
		if !config.IsHomeDir(d) && has1xLayout(d) {
			return Where{Project1xDir: d}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return Where{}
		}
		d = parent
	}
}

// has1xLayout reports whether dir holds a Dockerfile beside a .astro
// directory. A directory that cannot be read holds neither.
func has1xLayout(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, ".astro"))
	return err == nil && info.IsDir()
}

// IsManifestRoot reports whether dir is a pyproject.toml project's root, the
// question every deploy routes on:
//
//   - a pyproject.toml whose [tool.astro] loads, or fails to validate: a
//     project to fix, and the deploy reports why;
//   - one that fails to parse, only when its text declares a tool.astro
//     table (a [tool.astro header, or a tool.astro key): otherwise it is
//     some other tool's file, and not a project;
//   - one that exists and cannot be read, as project.HasManifest has it: the
//     deploy then reports the read error rather than walking past it.
//
// No pyproject.toml, one with no [tool.astro], and one in a directory that
// cannot be looked in are not.
func IsManifestRoot(dir string) bool {
	path := filepath.Join(dir, project.Marker)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		// Not there, or a directory it cannot even look in: nothing at this
		// level, and the walk goes on.
		return false
	}
	_, err := manifest.Load(path)
	switch {
	case err == nil:
		return true
	case errors.Is(err, manifest.ErrNotFound), errors.Is(err, manifest.ErrNoAstroSection), errors.Is(err, fs.ErrNotExist):
		return false
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		// There, and unreadable.
		return !errors.Is(readErr, fs.ErrNotExist)
	}
	return declaresToolAstro.Match(raw)
}

// declaresToolAstro matches a TOML line declaring the tool.astro table: a
// [tool.astro] or [tool.astro.<sub>] header, an array-of-tables header under
// it, or a dotted tool.astro key.
var declaresToolAstro = regexp.MustCompile(`(?m)^\s*(\[\[?\s*"?tool"?\s*\.\s*"?astro"?\s*[\].]|"?tool"?\s*\.\s*"?astro"?\s*[.=])`)

// NoDeployableProject is what a deploy says when the working directory is not
// the root of a pyproject.toml project, from Locate's answer where (for the
// working directory):
//
//   - in or below a project in the Astro CLI 1.x layout, refused1x, naming
//     that project's directory when the deploy ran below it;
//   - below a pyproject.toml project, to run the deploy from its root;
//   - anywhere else, the no-project advice, which in the home directory does
//     not suggest astro init.
//
// Every one of these is reported under the kind no_project: it unwraps to a
// *project.NotFoundError.
func NoDeployableProject(where Where, refused1x Refusal1x) error {
	wd := config.WorkingPath
	switch {
	case where.Project1xDir != "":
		says, dir := "this project", ""
		if !sameDir(where.Project1xDir, wd) {
			says, dir = "this directory is inside a project at "+where.Project1xDir+" that", where.Project1xDir
		}
		return &noProjectError{msg: refused1x(says, dir), cause: &project.NotFoundError{Start: wd, Project1xDir: where.Project1xDir}}
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
