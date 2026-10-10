package project

import (
	"sync/atomic"

	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Context is what the CLI knows of the current context, for what it says
// about a 1.x project: Astro Private Cloud deploys the 1.x layout (with Astro
// CLI 1.x) and not yet pyproject.toml projects, so a 1.x project there is not
// to be converted yet, and a context the CLI cannot resolve may be APC's.
type Context struct {
	// APC is set when the current context is Astro Private Cloud.
	APC bool
	// Unresolved is set when a context is named, by the home config or
	// ASTRO_DOMAIN, that cannot be resolved to a platform: the config cannot
	// be read, or names a context it does not hold for a domain that is not
	// Astro's.
	Unresolved bool
	// FromASTRODomain is set when ASTRO_DOMAIN named the context, so the way
	// to change it is to change or unset ASTRO_DOMAIN.
	FromASTRODomain bool
}

// current is the Context SetContext recorded. Package state, set once at
// startup, because the advice about a 1.x project is built in many places —
// this package's errors, astro init, the astro dev stub, and whatever reports
// them — and none of them reads the context: it lives in config/, which the
// core never imports. One setting read where the decision is made is what
// keeps them from disagreeing.
var current atomic.Pointer[Context]

// SetContext records the current context. The root calls it once, before any
// command runs. A test that sets it restores it with t.Cleanup and does not
// run in parallel.
func SetContext(c Context) { current.Store(&c) }

// currentContext is what SetContext recorded; the zero Context, Astro, when
// nothing did.
func currentContext() Context {
	if c := current.Load(); c != nil {
		return *c
	}
	return Context{}
}

// UnderAPC reports that the current context is Astro Private Cloud.
func UnderAPC() bool { return currentContext().APC }

// Block is why astro init refuses to convert a 1.x project here, if it does.
type Block int

const (
	// NotBlocked: init converts as anywhere.
	NotBlocked Block = iota
	// BlockedUnderAPC: the current context is Astro Private Cloud.
	BlockedUnderAPC
	// BlockedUnresolved: a current context is named that the CLI cannot
	// resolve, so it cannot tell whether it is APC's.
	BlockedUnresolved
)

// Convert1xBlocked is the one decision of whether astro init refuses dir:
// under an APC or unresolved context, when dir is a 1.x project or lies
// inside one (scaffold.Find1xProject, the walk scaffold.Plan makes). It
// returns why and that project's directory, or NotBlocked and "". Every hint
// about a 1.x project asks it (this package's errors, when they are made; the
// astro dev stub), as init does, so none suggests astro init where init
// refuses.
func Convert1xBlocked(dir string) (why Block, root string) {
	switch c := currentContext(); {
	case c.APC:
		why = BlockedUnderAPC
	case c.Unresolved:
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
// init refuses for why: why it stays as it is, and how to convert it anyway,
// ending with astro init in root. How the context is changed depends on
// what chose it: ASTRO_DOMAIN, or the saved current context.
func Blocked1xMessage(why Block, root string) string {
	fromEnv := currentContext().FromASTRODomain
	if why == BlockedUnresolved {
		change := "Fix the context, or switch to one (astro context list, then astro context switch)"
		if fromEnv {
			change = "ASTRO_DOMAIN names a context the CLI cannot resolve: change it to a saved context, or unset it"
		}
		return root + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context " +
			"cannot be resolved, so whether the project deploys to Astro Private Cloud, where it is not converted yet, " +
			"cannot be told. " + change + ", then run " + initCommand + " in " + root
	}
	change := "switch to an Astro context first (astro context switch astronomer.io, or astro login to sign in to Astro)"
	if fromEnv {
		change = "change ASTRO_DOMAIN, which names this context, to an Astro domain or unset it first"
	}
	return scaffold.Project1xUnderAPCReason(root) + ". To convert it anyway, for Astro or for local development " +
		"only, " + change + ", then run " + initCommand + " in " + root
}
