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
// constraint-dependencies they decoded to, with the runtime's lines it heads
// (SplitRuntimeBlock). "" and none when there is no record.
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
	_, block := SplitRuntimeBlock(tomledit.ArrayLines(data, constraintsKey, values), RuntimeConstrained)
	for _, l := range block {
		switch v := l.(type) {
		case tomledit.Comment:
			build, python, _ = IsRuntimeRecord(string(v))
		case string:
			lines = append(lines, v)
		}
	}
	return build, python, lines
}

// runtimeConstrainedDists are the distributions a runtime's constraints file
// holds besides the Airflow ones, whose +astro pins init writes on their own:
// SQLAlchemy, on Airflow 3.
var runtimeConstrainedDists = []string{"sqlalchemy"}

// RuntimeConstrained reports a distribution a runtime's constraints file
// holds besides the Airflow ones: what the runtime's block in [tool.uv]
// constraint-dependencies may own.
func RuntimeConstrained(dist string) bool {
	return slices.Contains(runtimeConstrainedDists, DistName(dist))
}

// SplitRuntimeBlock splits an array's lines into the rest and the runtime's
// block: its record comment and the exact name==version lines right after it
// whose distribution owned accepts, up to the first line that is not one. A
// project's own line written under the block, an exact pin of another
// distribution included, is never the runtime's.
func SplitRuntimeBlock(lines []any, owned func(dist string) bool) (rest, block []any) {
	for i := 0; i < len(lines); i++ {
		c, isComment := lines[i].(tomledit.Comment)
		if _, _, ok := IsRuntimeRecord(string(c)); !isComment || !ok {
			rest = append(rest, lines[i])
			continue
		}
		block = append(block, lines[i])
		for i+1 < len(lines) {
			s, isString := lines[i+1].(string)
			if !isString || !exactPin(s) || !owned(DistName(s)) {
				break
			}
			block = append(block, s)
			i++
		}
	}
	return rest, block
}

// exactPin reports a plain name==version line with no marker, the only kind
// a runtime's constraints file holds.
func exactPin(s string) bool {
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
