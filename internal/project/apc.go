package project

import (
	"path/filepath"
	"sync/atomic"
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
		"to sign in to Astro) and run " + initCommand + " again"
}

// Enclosing1xProject is the 1.x project start is in or below, the one
// Discover would name, or "" when there is none: the walk up stops at a
// directory that is already a project (HasManifest), as a 1.x project inside
// it is that project's business. start need not exist yet.
func Enclosing1xProject(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for dir := abs; ; {
		if Is1xProject(dir) {
			return dir
		}
		if HasManifest(dir) {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
