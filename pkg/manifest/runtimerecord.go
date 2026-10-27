package manifest

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// astro holds a project's lockfile to what its Astro Runtime image's own
// install holds packages to: the runtime's constraints file, which pins
// Airflow, the Task SDK and, on Airflow 3, SQLAlchemy. The Airflow and Task
// SDK pins are the +astro build pins init already writes; any other line the
// file has follows them in [tool.uv] constraint-dependencies, under one
// comment that records the build and the Python they come from:
//
//	constraint-dependencies = [
//	    'apache-airflow==3.3.2+astro.2',
//	    'apache-airflow-task-sdk==1.3.2+astro.1',
//	    # Astro Runtime 3.3-11 (Python 3.14) constrains these. astro writes them.
//	    'SQLAlchemy==2.0.50',
//	]
//
// uv reads the array as it reads any other. The comment is the committed
// record of which image the project builds on, which a build of the project
// can name.

// RuntimeRecord is the comment that records the build and the Python a
// project's runtime constraints come from.
func RuntimeRecord(build, python string) string {
	return "Astro Runtime " + build + " (Python " + python + ") constrains these. astro writes them."
}

// runtimeRecordRe reads the build and the Python back out of a RuntimeRecord.
var runtimeRecordRe = regexp.MustCompile(`^Astro Runtime (\S+) \(Python (\d+\.\d+)\) constrains these\.`)

// IsRuntimeRecord reports a comment that is a RuntimeRecord, and what it
// names.
func IsRuntimeRecord(comment string) (build, python string, ok bool) {
	m := runtimeRecordRe.FindStringSubmatch(comment)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var constraintsKey = []string{"tool", "uv", "constraint-dependencies"}

// runtimeRecordOf reads the record out of a manifest's bytes, given the
// constraint-dependencies they decoded to, with the lines it heads: the exact
// name==version lines right after it. "" and none when there is no record.
func runtimeRecordOf(data []byte, constraints []string) (build, python string, lines []string) {
	// Every manifest load comes through here, so the comments are looked for
	// only when the record can be among them.
	if !bytes.Contains(data, []byte(") constrains these.")) {
		return "", "", nil
	}
	values := make([]any, len(constraints))
	for i, c := range constraints {
		values[i] = c
	}
	all := tomledit.ArrayLines(data, constraintsKey, values)
	for i, l := range all {
		c, isComment := l.(tomledit.Comment)
		if !isComment {
			continue
		}
		var ok bool
		if build, python, ok = IsRuntimeRecord(string(c)); !ok {
			continue
		}
		for _, next := range all[i+1:] {
			s, isString := next.(string)
			if !isString || !ExactPin(s) {
				break
			}
			lines = append(lines, s)
		}
		return build, python, lines
	}
	return "", "", nil
}

// ExactPin reports a plain name==version line with no marker, the only kind
// a runtime's constraints file holds.
func ExactPin(s string) bool {
	name, version, ok := strings.Cut(s, "==")
	return ok && name != "" && version != "" && !strings.ContainsAny(s, ";@ ")
}

// WithoutRuntimeConstraints is constraints without the runtime's lines (UV's
// RuntimeConstraints): for an environment that runs another Airflow than the
// runtime's, as a check against MWAA or Composer does.
func (u *UV) WithoutRuntimeConstraints(constraints []string) []string {
	var out []string
	for _, c := range constraints {
		if !slices.Contains(u.RuntimeConstraints, c) {
			out = append(out, c)
		}
	}
	return out
}
