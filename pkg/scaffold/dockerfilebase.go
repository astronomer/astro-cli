package scaffold

import (
	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// baseKind is what a found Dockerfile's final stage builds on, as far as
// deciding whether to declare the file as the project's build goes.
type baseKind int

const (
	// baseUnknown is a file this run cannot answer for: no FROM, a file it
	// could not parse, or a FROM built from a build argument that could not be
	// expanded. Declared, as before this check existed, with a note when the
	// reason is the build argument.
	baseUnknown baseKind = iota
	// baseRuntime is an Astro Runtime image, the only base Docker mode starts.
	baseRuntime
	// baseForeign is any other image: python, docker, apache/airflow. Not
	// declared.
	baseForeign
)

// dockerfileBase classifies the base a Dockerfile's final image builds on, as
// airflowrt.ReadDeclaredBase read it. ref is the base for a message: the FROM
// reference, with what its build arguments expanded to where it used any.
//
// The rule is the one Docker mode's start enforces, deliberately, and not a
// "runtime" substring: ReadDeclaredBase reads the final stage (following
// aliases, expanding build arguments from their defaults) and
// airflowrt.IsAstroRuntimeImage decides, which is what localdocker's
// refuseUnsupportedBase asks before every start. A Dockerfile init declared
// that start then refuses is the failure this exists to prevent, so the two
// read the file with the same reader; a `-base` or `-python-X.Y` tag and a
// digest pin are all Runtime, because the image decides and not the tag.
//
// A FROM ReadDeclaredBase could not expand is unknown, never foreign: an
// argument with no default may be one a person passes with --build-arg, and a
// form it does not read may name anything.
func dockerfileBase(base airflowrt.DeclaredBase) (kind baseKind, ref string) {
	ref = base.From
	if !base.Known {
		return baseUnknown, ref
	}
	switch {
	case base.Expanded == base.From:
	case airflowrt.IsUnresolvedRef(base.From):
		ref += " (" + base.Expanded + " by default)"
	default:
		// Only quotes were removed.
		ref = base.Expanded
	}
	if airflowrt.IsAstroRuntimeImage(base.Image) {
		return baseRuntime, ref
	}
	return baseForeign, ref
}
