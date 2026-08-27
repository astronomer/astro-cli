package scaffold

import (
	"regexp"
	"slices"
	"strings"
)

// Reading and writing the Airflow pin is the one bit of packaging grammar this
// package needs: it derives [project.dependencies]'s apache-airflow entry from
// [tool.astro].airflow, and reads that entry back when adopting a manifest
// someone else wrote. Nothing here rewrites a requirement — rewriting would be
// guessing.

// airflowDist is the core Airflow distribution's normalized (PEP 503) name.
const airflowDist = "apache-airflow"

// airflowPinRe accepts the version shapes tool.astro.airflow allows.
var airflowPinRe = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)

// airflowRequirement is the [project.dependencies] entry that installs the
// Airflow the manifest pins. It mirrors [tool.astro].airflow one-to-one: a
// partial pin ("3", "3.1") becomes a prefix match ("apache-airflow==3.1.*") so
// the project tracks patch releases — the same "resolution to a concrete
// release happens later" the pin itself promises — while a full "3.1.2" pin
// stays exact.
func airflowRequirement(version string) string {
	if strings.Count(version, ".") < 2 {
		return airflowDist + "==" + version + ".*"
	}
	return airflowDist + "==" + version
}

// pinFromDeps returns the version the first clean apache-airflow pin in a
// dependency list states. Reading an existing pin keeps a project that runs 2.9
// on 2.9 instead of moving it without being asked.
//
// The precedence chain that uses this lives in scaffold.go's pickAirflowVersion,
// which is now the only place the ordering is written down. It used to be spread
// between a resolveAirflowVersion here and a pickAirflowVersion there, and the
// two disagreed about which source outranked which.
func pinFromDeps(deps []string) (version string, ok bool) {
	for _, spec := range deps {
		if v, found := pinFromSpec(spec); found {
			return v, true
		}
	}
	return "", false
}

// airflowExtrasNote reports the extras lost when an apache-airflow requirement
// is replaced by the one the pin generates.
//
// Both arms drop a v1 apache-airflow entry, on the grounds that it is where the
// pin came from and the generated requirement says the same thing. That is only
// true when the entry carries no extras: "apache-airflow[celery,statsd]==2.9.1"
// also names two installed distributions, and the replacement
// "apache-airflow==2.9.*" does not. They were vanishing silently.
func airflowExtrasNote(spec string) []string {
	lb := strings.Index(spec, "[")
	rb := strings.Index(spec, "]")
	if lb < 0 || rb < lb {
		return nil
	}
	return []string{"requirements.txt: " + spec + " declares extras " + spec[lb:rb+1] +
		", which the generated Airflow requirement does not carry: add them to the apache-airflow entry in [project.dependencies]"}
}

// pinsAirflow reports whether [project.dependencies] already names
// apache-airflow, however it is pinned.
func pinsAirflow(deps []string) bool {
	return slices.ContainsFunc(deps, namesAirflow)
}

// distName extracts and normalizes the distribution name from a PEP 508
// requirement: the leading name, before any extras, version, marker, or URL.
// Mirrors internal/checks.distName, internal/pack.distName and
// internal/imagebuild.distName; the four stay separate rather than couple
// these packages over one small helper.
func distName(req string) string {
	s := strings.TrimSpace(req)
	if i := strings.IndexAny(s, "[ \t<>=!~;@("); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

// namesAirflow reports whether a requirement is the core Airflow
// distribution, however it is pinned — or whether it is pinned at all.
func namesAirflow(spec string) bool {
	return distName(spec) == airflowDist
}

// pinFromSpec returns the version an apache-airflow requirement pins, when it
// carries a clean "==" pin. A range, a wildcard, or extra specifiers yield no
// pin: the caller falls back to the default.
func pinFromSpec(spec string) (version string, ok bool) {
	if !namesAirflow(spec) {
		return "", false
	}
	s := spec
	// An environment marker or a direct URL follows the specifier, so the pin
	// is whatever comes before it.
	if i := strings.IndexAny(s, ";@"); i >= 0 {
		s = s[:i]
	}
	_, rest, found := strings.Cut(s, "==")
	if !found {
		return "", false
	}
	v := strings.TrimSpace(rest)
	if strings.ContainsAny(v, ", *") { // more than a single exact pin
		return "", false
	}
	if !airflowPinRe.MatchString(v) {
		return "", false
	}
	return v, true
}
