package local

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
)

// errInitHomeDir is the refusal for `astro init` in the home directory. %s is
// the directory.
const errInitHomeDir = "%s is your home directory, which cannot be an Astro project: everything under it would be the project, and its build context. " +
	"Run astro init in a directory of its own, or name one: astro init my-project"

// warnInitNested is the warning for `astro init` inside another project. The
// first %s is the directory, the second the project above it.
const warnInitNested = "warning: %s is inside the Astro project at %s. Commands run in %[1]s or below will act on the new project, not that one\n"

// refuseInitHomeDir refuses dir when it is the home directory. Nothing else
// guards it, and a project there sweeps every file under ~ into it. A
// DIRECTORY argument naming somewhere else passes.
func refuseInitHomeDir(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	if filepath.Clean(dir) == filepath.Clean(home) {
		return cliout.Usage(fmt.Errorf(errInitHomeDir, dir))
	}
	return nil
}

// warnInitNested warns, on stderr, when dir is inside another project. It is
// not refused: a repository can hold more than one project, and the nearest
// manifest is the one every command finds, so the outer project keeps working
// from its own root. What changes is which project a command run below dir
// acts on, which is worth saying once.
func (c *cli) warnInitNested(dir string) {
	outer := project.Enclosing(dir, func(d string) bool {
		return project.HasManifest(d) || project.Is1xProject(d)
	})
	if outer != "" {
		fmt.Fprintf(c.d.Stderr, warnInitNested, dir, outer)
	}
}
