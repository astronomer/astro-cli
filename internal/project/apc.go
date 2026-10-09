package project

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// underAPC is whether the current context is Astro Private Cloud, whose
// deploy still builds the 1.x layout, so a 1.x project there is not to be
// converted yet. Package state, set once at startup (SetUnderAPC), because
// the advice about a 1.x project is built in many places — this package's
// errors, astro init, the astro dev stub, and whatever reports them — and
// none of them reads the context: it lives in config/, which the core never
// imports. One setting read where the message is built is what keeps them
// from disagreeing.
var underAPC atomic.Bool

// SetUnderAPC records whether the current context is Astro Private Cloud.
// The root calls it once, before any command runs. A test that sets it
// restores it with t.Cleanup and does not run in parallel.
func SetUnderAPC(apc bool) { underAPC.Store(apc) }

// UnderAPC reports what SetUnderAPC recorded; false, Astro, when nothing did.
func UnderAPC() bool { return underAPC.Load() }

// Project1xUnderAPC is the one account of the 1.x project in dir under an
// Astro Private Cloud context. APC's deploy still builds the 1.x layout, so
// astro init refuses to convert one there, and every hint that would
// otherwise say to run it says this instead: astro init's refusal, the
// errors of a command run in or below such a project (project1xMessage), and
// the astro dev stub.
func Project1xUnderAPC(dir string) string {
	return dir + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context is " +
		"Astro Private Cloud, whose astro deploy still builds that layout. Leave the project as it is for now: " +
		"astro deploy keeps working with it on Astro Private Cloud, and converting it will be available once " +
		"Astro Private Cloud deploys pyproject.toml projects. To convert it anyway, for Astro or for local " +
		"development only, switch to an Astro context first (astro context switch astronomer.io, or astro login " +
		"to sign in to Astro) and run " + initCommand + " in " + dir
}

// Project1xAt is the 1.x project dir is in or below, as discovery reports it
// to every command (NotFoundError.Project1xDir, NoAstroSectionError's Dir),
// or "" when discovery names none. dir need not exist yet.
func Project1xAt(dir string) string {
	proj, err := Discover(dir)
	if err == nil {
		if _, loadErr := manifest.Load(filepath.Join(proj.Dir, Marker)); loadErr != nil {
			err = LoadError(dir, proj.Dir, loadErr)
		}
	}
	var nf *NotFoundError
	if errors.As(err, &nf) {
		return nf.Project1xDir
	}
	var ns *NoAstroSectionError
	if errors.As(err, &ns) && ns.Has1xProject {
		return ns.Dir
	}
	return ""
}

// isCLIHome reports a directory whose .astro/ is the CLI's own settings
// rather than a 1.x project's: the home directory, and ASTRO_HOME when it
// moves the settings (config.initHome reads the same two). A stray
// Dockerfile there does not make it a 1.x project.
func isCLIHome(dir string) bool {
	for _, home := range []string{os.Getenv("ASTRO_HOME"), userHome()} {
		if home != "" && samePath(dir, home) {
			return true
		}
	}
	return false
}

func userHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// samePath compares two paths as written and with symlinks resolved, so a
// home under /var reached as /private/var still matches.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
