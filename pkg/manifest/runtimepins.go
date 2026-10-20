package manifest

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// The runtime pins are the versions an Astro Runtime image ships, which astro
// writes into [tool.uv] constraint-dependencies below a comment naming the
// build, so the project's lockfile resolves to what the image runs:
//
//	constraint-dependencies = [
//	    'apache-airflow==3.3.2+astro.2',
//	    'apache-airflow-task-sdk==1.3.2+astro.1',
//	    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.
//	    # astro writes them. To choose a version yourself, list the package in [project] dependencies.
//	    'a2wsgi==1.10.10',
//	    …
//	]
//
// The comment is how they are told apart from constraints the project wrote:
// every constraint after it is a runtime pin, and each one before it is the
// project's, or an AstroPin. uv reads the array as it reads any other; this
// is for astro, which rewrites the pins when the build changes, and for a
// platform check that runs another Airflow, which leaves them out.

// RuntimePinsComments are the comment lines that head the runtime pins of
// build, running python: the first names them, the second says how to step
// out of one.
func RuntimePinsComments(build, python string) []string {
	return []string{
		"Astro Runtime " + build + " (Python " + python + ") ships these versions, so the lockfile resolves to what the image runs.",
		"astro writes them. To choose a version yourself, list the package in [project] dependencies.",
	}
}

// runtimePinsHeaderRe reads the build and the Python back out of the first of
// RuntimePinsComments.
var runtimePinsHeaderRe = regexp.MustCompile(`^Astro Runtime (\S+) \(Python (\d+\.\d+)\) ships these versions`)

var constraintsKey = []string{"tool", "uv", "constraint-dependencies"}

// RuntimePinsIn splits constraint-dependencies as tomledit's GetLines reads it
// at the comment that heads the runtime pins: before is everything ahead of
// it, and pins is that comment and everything after it. build and python are
// what the comment names, "" when there is no such comment, and then pins is
// empty.
func RuntimePinsIn(lines []any) (before, pins []any, build, python string) {
	for i, l := range lines {
		c, ok := l.(tomledit.Comment)
		if !ok {
			continue
		}
		if m := runtimePinsHeaderRe.FindStringSubmatch(string(c)); m != nil {
			return lines[:i], lines[i:], m[1], m[2]
		}
	}
	return lines, nil, "", ""
}

// runtimePinsOf reads the runtime pins out of a manifest's bytes. A document
// that does not parse here has already been reported by toml.Unmarshal, so it
// reads as none.
func runtimePinsOf(data []byte) (build, python string, pins []string) {
	// Every manifest load comes through here, and few carry the pins, so the
	// document is parsed again only when the comment can be in it.
	if !bytes.Contains(data, []byte("# Astro Runtime ")) {
		return "", "", nil
	}
	ed, err := tomledit.NewSurgical(data)
	if err != nil {
		return "", "", nil
	}
	lines, ok := ed.GetLines(constraintsKey)
	if !ok {
		return "", "", nil
	}
	_, after, build, python := RuntimePinsIn(lines)
	for _, l := range after {
		if s, isString := l.(string); isString {
			pins = append(pins, s)
		}
	}
	return build, python, pins
}

// ProjectConstraints are the constraints the project states for itself:
// ConstraintDependencies without the AstroPin entries and without the runtime
// pins, for an environment that runs neither Astronomer's Airflow nor the
// runtime's packages: MWAA's, or Composer's.
func (u *UV) ProjectConstraints() []string {
	runtime := make(map[string]bool, len(u.RuntimePins))
	for _, p := range u.RuntimePins {
		runtime[strings.TrimSpace(p)] = true
	}
	out := make([]string, 0, len(u.ConstraintDependencies))
	for _, c := range u.ConstraintDependencies {
		if !AstroPin(c) && !runtime[strings.TrimSpace(c)] {
			out = append(out, c)
		}
	}
	return out
}
