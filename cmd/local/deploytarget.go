package local

import (
	"errors"

	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// The values `astro init --deploy-target` takes.
const (
	deployTargetAstro = "astro"
	deployTargetAPC   = "apc"
	flagDeployTarget  = "deploy-target"
)

// deployTargetValue is `astro init --deploy-target`: the platform a converted
// project deploys to, when the current context is not the answer. It refuses
// any other value while cobra parses flags, so a typo is a usage error before
// anything is read or written, as --output's is.
type deployTargetValue string

func (v *deployTargetValue) String() string { return string(*v) }

func (v *deployTargetValue) Set(s string) error {
	switch s {
	case deployTargetAstro, deployTargetAPC:
		*v = deployTargetValue(s)
		return nil
	}
	return errors.New("must be astro or apc")
}

func (v *deployTargetValue) Type() string { return "string" }

// initDeployTarget decides the platform `astro init` converts for, and how the
// messages that turn on it say so: --deploy-target when it was given, else the
// current context, where no context is Astro. It is the whole decision; the
// project's files have no say (see scaffold.Options.DeploysToAPC).
func initDeployTarget(flag deployTargetValue, d *Deps) (apc bool, basis scaffold.DeployTargetBasis) {
	switch flag {
	case deployTargetAPC:
		apc = true
		basis.Why = "of --" + flagDeployTarget + " " + deployTargetAPC
	case deployTargetAstro:
		basis.Why = "of --" + flagDeployTarget + " " + deployTargetAstro
	default:
		apc = d.DeploysToAPC
		switch {
		case apc:
			basis.Why = "the current context is Astro Private Cloud" + inParens(d.ContextDomain)
		case d.ContextDomain == "":
			basis.Why = "no context is current"
		default:
			basis.Why = "the current context is Astro" + inParens(d.ContextDomain)
		}
	}
	other := deployTargetAPC
	if apc {
		other = deployTargetAstro
	}
	basis.Instead = "pass --" + flagDeployTarget + " " + other
	return apc, basis
}

// inParens is " (s)", or nothing for an empty s.
func inParens(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}
