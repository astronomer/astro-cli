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
	// to change it is to set ASTRO_DOMAIN.
	FromASTRODomain bool
	// UnsetIsAstro is set, with FromASTRODomain, when the context the home
	// config saves as current is Astro's, or there is none: unsetting
	// ASTRO_DOMAIN then makes the context Astro, so the advice offers it.
	UnsetIsAstro bool
	// UnreadableConfig is the CLI's settings file when it exists and cannot be
	// read, which is why the context is Unresolved; the advice is then to fix
	// or move that file, not to change the context.
	UnreadableConfig string
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
// returns why and that project's directory, or NotBlocked and "". The context
// is checked first, so under Astro nothing is walked; a hint that names the
// root under Astro walks for itself. Every hint about a 1.x project gets its
// answer from here or from blockedFor (this package's errors, when they are
// made; the astro dev stub), as init does, so none suggests astro init where
// init refuses.
func Convert1xBlocked(dir string) (why Block, root string) {
	if c := currentContext(); !c.APC && !c.Unresolved {
		return NotBlocked, ""
	}
	root = scaffold.Find1xProject(dir)
	return blockedFor(root), root
}

// blockedFor is why astro init refuses the 1.x project in root, if it does:
// under an APC or unresolved context. NotBlocked for no root.
func blockedFor(root string) Block {
	if root == "" {
		return NotBlocked
	}
	switch c := currentContext(); {
	case c.APC:
		return BlockedUnderAPC
	case c.Unresolved:
		return BlockedUnresolved
	}
	return NotBlocked
}

// Blocked1xMessage is the account of the 1.x project in root that astro
// init refuses for why: why it stays as it is, and how to convert it anyway,
// ending with astro init in root. How the context is changed depends on
// what chose it: an unreadable settings file, ASTRO_DOMAIN, or the saved
// current context.
func Blocked1xMessage(why Block, root string) string {
	c := currentContext()
	then := ", then run " + initCommand + " in " + scaffold.ShellQuote(root)
	held := root + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/)"
	if why == BlockedUnresolved {
		if c.UnreadableConfig != "" {
			return held + ", and the CLI's settings file " + c.UnreadableConfig + " cannot be read, so whether the " +
				"current context deploys to Astro Private Cloud, where the project is not converted yet, cannot be told. " +
				"Fix or move " + c.UnreadableConfig + then
		}
		change := "Fix the context, or switch to one (astro context list, then astro context switch)"
		if c.FromASTRODomain {
			change = "ASTRO_DOMAIN names a context the CLI cannot resolve: set it to a saved context or to an Astro " +
				"domain (astronomer.io)" + orUnset(c)
		}
		return held + ", and the current context cannot be resolved, so whether the project deploys to Astro " +
			"Private Cloud, where it is not converted yet, cannot be told. " + change + then
	}
	change := scaffold.SwitchToAstro
	if c.FromASTRODomain {
		change = "set ASTRO_DOMAIN, which names this context, to an Astro domain (astronomer.io)" + orUnset(c) + " first"
	}
	return scaffold.Project1xUnderAPCReason(root) + ". To convert it anyway, for Astro or for local development " +
		"only, " + change + then
}

// orUnset offers unsetting ASTRO_DOMAIN when that makes the context Astro.
func orUnset(c Context) string {
	if c.UnsetIsAstro {
		return ", or unset it"
	}
	return ""
}

// NewProjectNotice is what astro init says on stderr, in text mode, after it
// made a project under a context that does not deploy pyproject.toml
// projects, or may not: APC's, or one the CLI cannot resolve. "" under Astro.
func NewProjectNotice() string {
	c := currentContext()
	const runs = " This project runs locally; to deploy it, "
	switch {
	case c.APC && c.FromASTRODomain:
		return "Note: the current context is Astro Private Cloud, which does not yet deploy pyproject.toml projects." +
			runs + "set ASTRO_DOMAIN, which names this context, to an Astro domain (astronomer.io)" + orUnset(c) + "."
	case c.APC:
		return "Note: the current context is Astro Private Cloud, which does not yet deploy pyproject.toml projects." +
			runs + scaffold.SwitchToAstro + "."
	case c.Unresolved && c.UnreadableConfig != "":
		return "Note: the CLI's settings file " + c.UnreadableConfig + " cannot be read, so whether the current " +
			"context deploys pyproject.toml projects cannot be told." + runs + "fix or move that file."
	case c.Unresolved && c.FromASTRODomain:
		return "Note: ASTRO_DOMAIN names a context the CLI cannot resolve, so whether it deploys pyproject.toml " +
			"projects cannot be told." + runs + "set ASTRO_DOMAIN to a saved context or to an Astro domain " +
			"(astronomer.io)" + orUnset(c) + "."
	case c.Unresolved:
		return "Note: the current context cannot be resolved, so whether it deploys pyproject.toml projects cannot " +
			"be told." + runs + "fix the context, or switch to one (astro context list, then astro context switch)."
	}
	return ""
}
