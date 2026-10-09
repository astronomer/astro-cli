package local

import (
	"fmt"
	"path/filepath"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/fileutil"
)

// errInitHomeDir is the refusal for `astro init` in the home directory. %s is
// the directory.
const errInitHomeDir = "%s is your home directory, which cannot be an Astro project: everything under it would be the project, and its build context. " +
	"Run astro init in a directory of its own, or name one: astro init my-project"

// warnInitNested is the warning for `astro init` inside another project. The
// first %s is the directory, the second the project above it.
const warnInitNested = "warning: %s is inside the Astro project at %s. Commands run in %[1]s or below will act on the new project, not that one\n"

// isHomeDir is Deps.IsHomeDir, false when it is not wired.
func (c *cli) isHomeDir(dir string) bool {
	return c.d.IsHomeDir != nil && c.d.IsHomeDir(dir)
}

// refuseInitHomeDir refuses dir when it is the home directory, however it is
// spelled. Nothing else guards it, and a project there sweeps every file under
// ~ into it. A DIRECTORY argument naming somewhere else passes.
func (c *cli) refuseInitHomeDir(dir string) error {
	if c.isHomeDir(dir) {
		return cliout.Usage(fmt.Errorf(errInitHomeDir, dir))
	}
	return nil
}

// warnInitNested warns, on stderr, when dir is inside another project. It is
// not refused: a repository can hold more than one project, and the nearest
// manifest is the one every command finds, so the outer project keeps working
// from its own root. What changes is which project a command run below dir
// acts on, which is worth saying once.
//
// The project above is what `astro deploy` would name from below it
// (project.IsAstroProject), never the home directory or the filesystem root.
// A directory that cannot be read, or whose pyproject.toml does not parse, is
// not one: nothing says it is a project, and a warning claiming so would be
// wrong.
func (c *cli) warnInitNested(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	readable := func(d string) (bool, error) {
		ok, err := project.IsAstroProject(d)
		return ok && err == nil, nil
	}
	outer, _ := fileutil.NearestDir(filepath.Dir(abs), c.isHomeDir, readable) //nolint:errcheck // readable returns no error, so neither does the walk
	if outer != "" {
		fmt.Fprintf(c.d.Stderr, warnInitNested, dir, outer)
	}
}
