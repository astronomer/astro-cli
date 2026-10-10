package project

import (
	"sync/atomic"

	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// underAPC is whether the current context is Astro Private Cloud, which
// deploys the 1.x layout (with Astro CLI 1.x), so a 1.x project there is not
// to be converted yet. Package state, set once at startup (SetUnderAPC), because
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

// contextUnresolved is whether a current context is named, by the home
// config or ASTRO_DOMAIN, but cannot be resolved to a platform: the config
// cannot be read, or names a context it does not hold for a domain that is
// not Astro's. The CLI then cannot tell whether a 1.x project deploys to
// Astro Private Cloud, so astro init refuses to convert one until the context
// is fixed. Set once at startup, as underAPC is.
var contextUnresolved atomic.Bool

// SetContextUnresolved records that the current context cannot be resolved.
// The root calls it once, before any command runs; tests restore it.
func SetContextUnresolved(unresolved bool) { contextUnresolved.Store(unresolved) }

// ContextUnresolved reports what SetContextUnresolved recorded.
func ContextUnresolved() bool { return contextUnresolved.Load() }

// Block is why astro init refuses to convert a 1.x project here, if it does.
type Block int

const (
	// NotBlocked: init converts as anywhere.
	NotBlocked Block = iota
	// BlockedUnderAPC: the current context is Astro Private Cloud, which
	// deploys the 1.x layout (with Astro CLI 1.x) and not yet pyproject.toml
	// projects.
	BlockedUnderAPC
	// BlockedUnresolved: a current context is named that the CLI cannot
	// resolve, so it cannot tell whether it is APC's.
	BlockedUnresolved
)

// Convert1xBlocked is the one decision of whether astro init refuses dir:
// under an APC or unresolved context, when dir is a 1.x project or lies
// inside one (scaffold.Find1xProject, the walk scaffold.Plan makes). It
// returns why and that project's directory, or NotBlocked and "". Every hint
// about a 1.x project asks it (project1xMessage, the astro dev stub), as
// init does, so none suggests astro init where init refuses.
func Convert1xBlocked(dir string) (why Block, root string) {
	switch {
	case UnderAPC():
		why = BlockedUnderAPC
	case ContextUnresolved():
		why = BlockedUnresolved
	default:
		return NotBlocked, ""
	}
	if root = scaffold.Find1xProject(dir); root == "" {
		return NotBlocked, ""
	}
	return why, root
}

// Blocked1xMessage is the account of the 1.x project in root that astro
// init refuses for why: under APC, Project1xUnderAPC; under an unresolved
// context, to fix or switch the context and run astro init in rerunIn, the
// directory init was given (root, for a hint that was given none).
func Blocked1xMessage(why Block, root, rerunIn string) string {
	if why == BlockedUnresolved {
		return root + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context " +
			"cannot be resolved, so whether the project deploys to Astro Private Cloud, where it is not converted yet, " +
			"cannot be told. Fix the context, or switch to one (astro context list, then astro context switch), and " +
			"run " + initCommand + " in " + rerunIn + " again"
	}
	return Project1xUnderAPC(root)
}

// Project1xUnderAPC is the one account of the 1.x project in dir under an
// Astro Private Cloud context, scaffold.Project1xUnderAPCMessage: astro init's
// refusal (scaffold.Plan's, under Options.DeploysToAPC), the errors of a
// command run in or below such a project (project1xMessage), and the astro
// dev stub all give it.
func Project1xUnderAPC(dir string) string { return scaffold.Project1xUnderAPCMessage(dir) }
