package imagebuild

import (
	"fmt"
	"strings"
)

// LockedBuildError refuses an image that cannot start FROM the build its
// uv.lock was made for (ManifestBuild.LockedRuntime), and whose fallback, the
// build it would start FROM instead, carries another Airflow release than
// the lock: the lock's versions were resolved against the lock's Airflow, so
// installing them over another one would be untested at best. It is said
// before any build, in one message, rather than as a warning that the
// fallback goes ahead followed by a refusal of the lock.
type LockedBuildError struct {
	// Locked is the lock's build, and LockedAirflow the Airflow it carries.
	Locked, LockedAirflow string
	// Why says why the image cannot start FROM Locked, as a clause: "the
	// Deployment does not offer it".
	Why string
	// Fallback is the build the image would start FROM instead, and
	// FallbackAirflow the Airflow it carries.
	Fallback, FallbackAirflow string
	// Fix says what to do, as a sentence without its full stop.
	Fix string
}

func (e *LockedBuildError) Error() string {
	return fmt.Sprintf("uv.lock was made for Astro Runtime %s (Airflow %s), and %s, so the image would build on %s, which has Airflow %s, a release the lock was not resolved for. %s",
		e.Locked, e.LockedAirflow, e.Why, e.Fallback, e.FallbackAirflow, e.Fix)
}

// SameAirflowRelease reports whether two Airflow versions are one release,
// local versions aside: 3.1.3+astro.2, 3.1.3+astro.3 and PyPI's 3.1.3 are
// one. Two that cannot be compared, one of them unknown (""), are taken to
// agree, so nothing is refused on a version that was not read.
func SameAirflowRelease(a, b string) bool {
	release := func(v string) string {
		r, _, _ := strings.Cut(strings.TrimSpace(v), "+")
		return strings.ToLower(r)
	}
	return a == "" || b == "" || release(a) == release(b)
}
