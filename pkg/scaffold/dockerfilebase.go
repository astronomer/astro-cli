package scaffold

import (
	"os"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// baseKind is what a found Dockerfile's final stage builds on, as far as
// deciding whether to declare the file as the project's build goes.
type baseKind int

const (
	// baseUnknown is a file this run cannot answer for: no FROM, a file it
	// could not parse, or a FROM built from a build argument in a form it does
	// not expand. Declared, as before this check existed, with a note when the
	// reason is the build argument.
	baseUnknown baseKind = iota
	// baseRuntime is an Astro Runtime image, the only base Docker mode starts.
	baseRuntime
	// baseForeign is any other image: python, docker, apache/airflow. Not
	// declared.
	baseForeign
)

// dockerfileBase classifies the base the Dockerfile at path builds its final
// image on, from data, its contents. ref is the base for a message: the FROM
// reference, with what a build argument resolved to where it used one.
//
// The rule is the one Docker mode's start enforces, deliberately, and not
// files1x's "runtime" substring: airflowrt.ReadDeclaredBase reads the final
// stage (following aliases) and airflowrt.IsAstroRuntimeImage decides, which
// is what localdocker's refuseUnsupportedBase asks before every start. A
// Dockerfile init declared that start then refuses is the failure this
// exists to prevent, so the two must agree; a `-base` or `-python-X.Y` tag
// and a digest pin are all Runtime, because the image decides and not the tag.
//
// A FROM built from a build argument (`ARG BASE=...` then `FROM ${BASE}`) is
// the one shape ReadDeclaredBase gives no answer for, because a --build-arg
// can replace the default. astro's own build passes none, so the image Docker
// mode builds is the one the defaults name, and that is what is classified:
// the ARGs declared before the first FROM (the only ones a FROM can use),
// with an ARG that has no default expanding to empty, as Docker expands it.
// `${VAR:-word}` and `${VAR:+word}` are expanded too. Any other modifier is
// left unknown rather than guessed at.
func dockerfileBase(path string, data []byte) (kind baseKind, ref string) {
	image, tag, err := airflowrt.ParseDockerfileAt(path)
	if err != nil {
		return baseUnknown, ""
	}
	ref = image
	if tag != "" {
		ref += ":" + tag
	}
	if airflowrt.IsUnresolvedRef(ref) {
		defaults := globalArgDefaults(data)
		// ParseDockerfileAt splits the reference before anything expands it, so
		// its halves are rejoined first: `${BASE:-python:3.12}` is split inside
		// the braces. Except that it supplies "latest" for a reference with no
		// tag, which for `FROM ${BASE}` is a tag the argument may carry itself,
		// so there the image alone is expanded. Only the image decides, so a
		// genuine ":latest" lost this way changes nothing.
		if tag == "latest" {
			ref = image
		}
		resolved, ok := expandBuildArgs(ref, defaults)
		if !ok {
			return baseUnknown, ref
		}
		if resolved == "" {
			ref += " (a build argument with no default)"
		} else {
			ref += " (" + resolved + " by default)"
		}
		image = resolved
	}
	if airflowrt.IsAstroRuntimeImage(image) {
		return baseRuntime, ref
	}
	return baseForeign, ref
}

// globalArgDefaults reads the ARG instructions before the first FROM, name to
// default. An ARG with no default is absent from the map.
func globalArgDefaults(data []byte) map[string]string {
	defaults := map[string]string{}
	for _, line := range logicalLines(data) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			return defaults
		case "ARG":
			for _, f := range fields[1:] {
				name, value, ok := strings.Cut(f, "=")
				if !ok {
					continue
				}
				defaults[name] = strings.Trim(value, `"'`)
			}
		}
	}
	return defaults
}

// expandBuildArgs expands the build arguments in s as Docker expands them in a
// FROM: $NAME, ${NAME}, ${NAME:-word} and ${NAME:+word}, with an unset name
// expanding to empty. ok is false for any other modifier.
func expandBuildArgs(s string, defaults map[string]string) (expanded string, ok bool) {
	ok = true
	expanded = os.Expand(s, func(key string) string {
		if name, word, found := strings.Cut(key, ":-"); found {
			if v := defaults[name]; v != "" {
				return v
			}
			return word
		}
		if name, word, found := strings.Cut(key, ":+"); found {
			if defaults[name] != "" {
				return word
			}
			return ""
		}
		if strings.ContainsAny(key, ":-+?#%/") {
			ok = false
			return ""
		}
		return defaults[key]
	})
	return expanded, ok
}
