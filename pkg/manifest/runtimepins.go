package manifest

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// The runtime pins are the versions an Astro Runtime image ships, which astro
// writes into [tool.uv] constraint-dependencies between two comments, so the
// project's lockfile resolves to what the image runs:
//
//	constraint-dependencies = [
//	    'apache-airflow==3.3.2+astro.2',
//	    'apache-airflow-task-sdk==1.3.2+astro.1',
//	    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.
//	    # astro writes them. To choose a version yourself, give the package a bound the image's version doesn't meet.
//	    "a2wsgi==1.10.10; python_version == '3.12'",
//	    …
//	    # End of what astro writes. Constraints below this line are yours.
//	]
//
// The comments are how they are told apart from constraints the project
// wrote, ahead of them or after them. uv reads the array as it reads any
// other; this is for astro, which rewrites the pins when the build changes,
// and for a platform check that runs another Airflow, which leaves them out.

// RuntimePinsComments are the comment lines that head the runtime pins of
// build, running python: the first names them, the second says how to step
// out of one.
func RuntimePinsComments(build, python string) []string {
	return []string{
		"Astro Runtime " + build + " (Python " + python + ") ships these versions, so the lockfile resolves to what the image runs.",
		"astro writes them. To choose a version yourself, give the package a bound the image's version doesn't meet.",
	}
}

// RuntimePinsEnd is the comment line that ends the runtime pins. A constraint
// after it is the project's.
const RuntimePinsEnd = "End of what astro writes. Constraints below this line are yours."

// runtimePinsHeaderRe reads the build and the Python back out of the first of
// RuntimePinsComments.
var runtimePinsHeaderRe = regexp.MustCompile(`^Astro Runtime (\S+) \(Python (\d+\.\d+)\) ships these versions`)

// RuntimePin is pin, a name==version line of the image's freeze, as the
// runtime pins write it: limited to the Python the image runs. A lockfile
// resolves for every Python requires-python allows, and the image's versions
// are of one: numpy 2.3 needs Python 3.11, so held on every Python it leaves
// a project that allows 3.10 nothing to lock there.
func RuntimePin(pin, python string) string {
	return pin + "; python_version == '" + python + "'"
}

// exactPinRe is a runtime pin as it is written: name==version, with or
// without RuntimePin's marker, nothing more.
var exactPinRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*==[^\s;,@]+(\s*;\s*python_version\s*==\s*['"]\d+\.\d+['"])?$`)

var constraintsKey = []string{"tool", "uv", "constraint-dependencies"}

// RuntimePinsIn splits constraint-dependencies as tomledit's GetLines reads it
// into the project's lines ahead of the runtime pins (before), the pins with
// the comments around them (pins), and the project's lines after them
// (after). build and python are what the heading comment names, "" when
// there is none, and then pins and after are empty.
//
// The pins end at RuntimePinsEnd. Without it they end at the first line after
// the heading comments that is not a name==version pin, so a constraint the
// project wrote after them is never taken for one of them.
func RuntimePinsIn(lines []any) (before, pins, after []any, build, python string) {
	start := -1
	for i, l := range lines {
		if c, ok := l.(tomledit.Comment); ok {
			if m := runtimePinsHeaderRe.FindStringSubmatch(string(c)); m != nil {
				start, build, python = i, m[1], m[2]
				break
			}
		}
	}
	if start < 0 {
		return lines, nil, nil, "", ""
	}
	for i := start + 1; i < len(lines); i++ {
		if c, ok := lines[i].(tomledit.Comment); ok && string(c) == RuntimePinsEnd {
			return lines[:start], lines[start : i+1], lines[i+1:], build, python
		}
	}
	end := start + 1
	for end < len(lines) {
		if _, isComment := lines[end].(tomledit.Comment); !isComment {
			break
		}
		end++
	}
	for end < len(lines) {
		if s, ok := lines[end].(string); !ok || !exactPinRe.MatchString(strings.TrimSpace(s)) {
			break
		}
		end++
	}
	return lines[:start], lines[start:end], lines[end:], build, python
}

// runtimePinsOf reads the runtime pins out of a manifest's bytes, given the
// constraint-dependencies they decoded to.
func runtimePinsOf(data []byte, constraints []string) (build, python string, pins []string) {
	// Every manifest load comes through here, and few carry the pins, so the
	// comments are looked for only when the heading can be among them.
	if len(constraints) == 0 || !bytes.Contains(data, []byte("# Astro Runtime ")) {
		return "", "", nil
	}
	values := make([]any, len(constraints))
	for i, c := range constraints {
		values[i] = c
	}
	_, block, _, build, python := RuntimePinsIn(tomledit.ArrayLines(data, constraintsKey, values))
	for _, l := range block {
		if s, isString := l.(string); isString {
			pins = append(pins, s)
		}
	}
	return build, python, pins
}

// ProjectConstraints are the constraints the project states for itself:
// ConstraintDependencies without the AstroPin entries (WithoutAstroPins) and
// without the runtime pins, for an environment that runs neither Astronomer's
// Airflow nor the runtime's packages: MWAA's, or Composer's.
func (u *UV) ProjectConstraints() []string {
	runtime := make(map[string]bool, len(u.RuntimePins))
	for _, p := range u.RuntimePins {
		runtime[strings.TrimSpace(p)] = true
	}
	var out []string
	for _, c := range WithoutAstroPins(u.ConstraintDependencies) {
		if !runtime[strings.TrimSpace(c)] {
			out = append(out, c)
		}
	}
	return out
}
