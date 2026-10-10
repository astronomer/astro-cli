package project

import (
	"sync/atomic"

	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// underAPC is whether the current context is Astro Private Cloud, which
// deploys the 1.x layout (with Astro CLI 1.x), so a 1.x project there is not to be
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
// Astro Private Cloud context, scaffold.Project1xUnderAPCMessage: astro init's
// refusal (scaffold.Plan's, under Options.DeploysToAPC), the errors of a
// command run in or below such a project (project1xMessage), and the astro
// dev stub all give it.
func Project1xUnderAPC(dir string) string { return scaffold.Project1xUnderAPCMessage(dir) }
