// Package platformversions holds the Apache Airflow versions the managed
// platforms (Amazon MWAA, Google Cloud Composer) offer, and the rules for
// mapping a project's Airflow pin onto them. Two callers share it: internal/
// pack, which pins the constraints file for a build artifact, and internal/
// checks, which picks the version a target-aware pre-flight check runs
// against. Keeping the data in one place means a refresh lands in one file.
package platformversions

import "strings"

// Version is one Apache Airflow release a managed platform offers. Python is
// the tag the constraints file is named for (MWAA builds a constraints URL
// from it); Composer resolves constraints against the environment itself, so
// its entries leave Python empty.
type Version struct {
	Airflow string
	Python  string
}

// The Python each MWAA image runs, from the same table as the Airflow
// versions.
const (
	py311 = "3.11"
	py312 = "3.12"
)

// MWAA is the set of Apache Airflow versions Amazon MWAA offers, newest first,
// each with the Python it runs. Callers compare the manifest pin against this
// list and build the --constraint URL from the match.
//
// Source: https://docs.aws.amazon.com/mwaa/latest/userguide/airflow-versions.html
// (checked 2026-07). MWAA adds a version and retires an old one a few times a
// year, so this is data to refresh, not logic to rewrite.
var MWAA = []Version{
	{Airflow: "3.2.1", Python: py312},
	{Airflow: "3.0.6", Python: py312},
	{Airflow: "2.11.0", Python: py312},
	{Airflow: "2.10.3", Python: py311},
	{Airflow: "2.10.1", Python: py311},
	{Airflow: "2.9.2", Python: py311},
	{Airflow: "2.8.1", Python: py311},
	{Airflow: "2.7.2", Python: py311},
}

// Composer is the set of Apache Airflow versions Cloud Composer 3 offers,
// newest first. Composer installs PyPI packages onto the environment and
// resolves their constraints there, so no Python tag is carried.
//
// Source: https://docs.cloud.google.com/composer/docs/composer-versions
// (checked 2026-07). As with MWAA, refresh the data; the logic does not
// change.
var Composer = []Version{
	{Airflow: "3.1.8"},
	{Airflow: "3.1.7"},
	{Airflow: "3.1.0"},
	{Airflow: "2.11.1"},
	{Airflow: "2.10.5"},
	{Airflow: "2.10.2"},
	{Airflow: "2.9.3"},
}

// Resolve matches a manifest Airflow pin against a platform's supported list
// (newest first). An exact pin wins; a partial pin ("3", "3.1") matches the
// newest supported version under it, so "3.1" resolves to "3.1.8" where the
// platform offers it. The bool reports whether the pin is supported directly —
// an unsupported pin still builds a valid artifact, only with a warning.
func Resolve(pin string, supported []Version) (Version, bool) {
	pin = strings.TrimSpace(pin)
	for _, v := range supported {
		if v.Airflow == pin {
			return v, true
		}
	}
	// The list is newest first, so the first prefix hit is the newest version
	// under the partial pin.
	for _, v := range supported {
		if strings.HasPrefix(v.Airflow, pin+".") {
			return v, true
		}
	}
	return Version{}, false
}

// Match picks the version a pre-flight check runs against. A pin the platform
// offers resolves directly (downgraded false). A pin the platform does not
// offer maps down to the closest lower supported version — the whole point of
// the check is to test against what would actually run — and downgraded is
// true. ok is false only when the pin sits below the platform's floor, so
// nothing supported is at or under it; the caller reports that as an error.
func Match(pin string, supported []Version) (v Version, downgraded, ok bool) {
	if direct, found := Resolve(pin, supported); found {
		return direct, false, true
	}
	// Resolve has already ruled out anything inside the pin's version band, so
	// every remaining version is wholly above or wholly below it. The list is
	// newest first, so the first version below the pin is the closest lower
	// one.
	pinKey := parse(strings.TrimSpace(pin))
	for _, cand := range supported {
		if compare(parse(cand.Airflow), pinKey) < 0 {
			return cand, true, true
		}
	}
	return Version{}, false, false
}

// List renders a platform's Airflow versions as a comma-separated string for a
// warning or a requirements-file comment.
func List(supported []Version) string {
	out := make([]string, len(supported))
	for i, v := range supported {
		out[i] = v.Airflow
	}
	return strings.Join(out, ", ")
}

// MWAAConstraintURL is MWAA's documented constraint convention: the Apache
// Airflow constraints file for the environment's Airflow and Python versions.
// Source: https://docs.aws.amazon.com/mwaa/latest/userguide/best-practices-dependencies.html
func MWAAConstraintURL(v Version) string {
	return "https://raw.githubusercontent.com/apache/airflow/constraints-" + v.Airflow + "/constraints-" + v.Python + ".txt"
}

// parse turns "3.1.8" (or a partial "3.1") into a fixed-width key for ordering.
// A missing or non-numeric segment counts as zero, so "3.1" reads as 3.1.0 —
// the floor of its version band, which is what Match compares against.
func parse(v string) [3]int {
	var out [3]int
	// Split into at most four parts: three version segments plus any trailing
	// pre-release or build tail we do not read.
	const maxParts = 4
	for i, seg := range strings.SplitN(v, ".", maxParts) {
		if i > 2 {
			break
		}
		n := 0
		for _, r := range seg {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out
}

// compare orders two version keys, returning -1, 0, or 1.
func compare(a, b [3]int) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}
