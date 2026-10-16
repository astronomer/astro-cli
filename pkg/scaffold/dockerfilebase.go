package scaffold

import (
	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// baseKind is what a found Dockerfile's final stage builds on, as far as
// deciding whether to declare the file as the project's build goes.
type baseKind int

const (
	// baseUnknown is a file this run cannot answer for: no FROM, a file it
	// could not parse, or a final FROM built from a build argument. Declared,
	// as before this check existed, with a note when the reason is the build
	// argument.
	baseUnknown baseKind = iota
	// baseRuntime is an Astro Runtime image, the only base Docker mode starts.
	baseRuntime
	// baseForeign is any other image: python, docker, apache/airflow. Not
	// declared.
	baseForeign
)

// dockerfileBase classifies the base a Dockerfile builds its final image on,
// from what airflowrt.ParseDockerfileAt read of it: image and tag, or err.
// ref is the base for a message: the FROM reference as written.
//
// The rule is the one Docker mode's start enforces, deliberately, and not
// files1x's "runtime" substring: airflowrt.ReadDeclaredBase reads the final
// stage (following aliases) and airflowrt.IsAstroRuntimeRef decides, which
// is what localdocker's refuseUnsupportedBase asks before every start. A
// Dockerfile init declared that start then refuses is the failure this
// exists to prevent, so the two must agree. On Astronomer's published
// repositories any tag is Runtime — a `-base` or `-python-X.Y` flavor, a
// digest pin. A mirror on another registry is Runtime when its repository is
// named runtime or astro-runtime and its tag is a Runtime version.
//
// A FROM built from a build argument (`ARG BASE=...` then `FROM ${BASE}`) is
// unknown, as it is to start: what it names depends on ARG defaults, their
// scoping and Docker's expansion rules, so this does not guess from the text.
func dockerfileBase(image, tag string, err error) (kind baseKind, ref string) {
	if err != nil {
		return baseUnknown, ""
	}
	ref = image
	if tag != "" {
		ref += ":" + tag
	}
	if airflowrt.IsUnresolvedRef(ref) {
		// ParseDockerfileAt supplies "latest" for a reference with no tag,
		// which for `FROM ${BASE}` is not what the file says.
		if tag == "latest" {
			ref = image
		}
		return baseUnknown, ref
	}
	if airflowrt.IsAstroRuntimeRef(image, tag) {
		return baseRuntime, ref
	}
	return baseForeign, ref
}
