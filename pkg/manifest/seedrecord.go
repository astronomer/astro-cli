package manifest

import "regexp"

// astro seeds a project's uv.lock with the versions an Astro Runtime image
// ships: it resolves the lock once with them as constraints, in a scratch
// copy of the project, and uv keeps them from then on as its lock's
// preferences. pyproject.toml holds none of them. What it does hold is one
// comment, the first of the astro-runtime dependency group (RuntimeGroupName),
// saying which build and Python the lock was seeded from:
//
//	# astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.
//
// uv reads the group as it reads any other. The comment is astro's: which
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
