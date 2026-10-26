package manifest

import (
	"bytes"
	"regexp"

	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// astro seeds a project's uv.lock with the versions an Astro Runtime image
// ships: it resolves the lock once with the image's versions as constraints,
// in a scratch copy of the project, and uv keeps them from then on as its
// lock's preferences. pyproject.toml holds none of them. What it does hold is
// one comment in [tool.uv] constraint-dependencies saying which build and
// Python the lock was seeded from:
//
//	constraint-dependencies = [
//	    'apache-airflow==3.3.2+astro.2',
//	    'apache-airflow-task-sdk==1.3.2+astro.1',
//	    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.
//	]
//
// uv reads the array as it reads any other. The comment is astro's: which
// image the lock was made for, a committed record a build of the project (and
// a raw `docker build .`) can name.

// SeedRecord is the comment that records the build and the Python a lock was
// seeded from.
func SeedRecord(build, python string) string {
	return "astro seeded uv.lock from Astro Runtime " + build + " (Python " + python + "). Run astro init to seed it again."
}

// seedRecordRe reads the build and the Python back out of a SeedRecord.
var seedRecordRe = regexp.MustCompile(`^astro seeded uv\.lock from Astro Runtime (\S+) \(Python (\d+\.\d+)\)`)

// IsSeedRecord reports a comment that is a SeedRecord, and what it names.
func IsSeedRecord(comment string) (build, python string, ok bool) {
	m := seedRecordRe.FindStringSubmatch(comment)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var constraintsKey = []string{"tool", "uv", "constraint-dependencies"}

// seedRecordOf reads the seed record out of a manifest's bytes, given the
// constraint-dependencies they decoded to. "" when there is none.
func seedRecordOf(data []byte, constraints []string) (build, python string) {
	// Every manifest load comes through here, so the comments are looked for
	// only when the record can be among them.
	if !bytes.Contains(data, []byte("# astro seeded uv.lock from ")) {
		return "", ""
	}
	values := make([]any, len(constraints))
	for i, c := range constraints {
		values[i] = c
	}
	for _, l := range tomledit.ArrayLines(data, constraintsKey, values) {
		if c, isComment := l.(tomledit.Comment); isComment {
			if build, python, ok := IsSeedRecord(string(c)); ok {
				return build, python
			}
		}
	}
	return "", ""
}
