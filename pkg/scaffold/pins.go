package scaffold

import (
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Reading and writing the Airflow pin is the one bit of packaging grammar this
// package needs: it derives [project.dependencies]'s apache-airflow entry from
// [tool.astro].airflow, and reads that entry back when adopting a manifest
// someone else wrote. Nothing here rewrites a requirement — rewriting would be
// guessing.

// airflowDist is the core Airflow distribution's normalized (PEP 503) name.
const airflowDist = "apache-airflow"

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

// requiresPython is the [project] requires-python for a project pinned to this
// Airflow: the interpreters that Airflow can actually run under.
//
// It matters because, once it is written, nothing else chooses the
// interpreter. A start passes no PythonVersion for a manifest that states
// requires-python — uv reads it from the manifest — so this string is the only
// thing standing between the pin and whatever Python the machine happens to
// have newest. airflowrt.PythonFallback covers a manifest that states none,
// with the same ceiling as below; the two are kept in step by hand, because
// this module does not import that one.
//
// Airflow 2 is the case that bites. It never supported Python 3.13, and its
// Flask stack reaches for stdlib that 3.12 removed, but its published metadata
// carries no upper bound — so uv installs it on a 3.14 and the project fails at
// start rather than at resolve, with:
//
//	AttributeError: module 'ast' has no attribute 'Str'
//
// from inside werkzeug, which names neither Airflow nor Python. Every v1
// conversion lands on an Airflow 2 pin, so this is the path a migrating user
// takes.
//
// Airflow 3 is deliberately left open. It tracks new interpreters, and capping
// it here would mean editing this file every time one ships — the failure that
// bound would cause (refusing a Python that works) is worse than the one it
// would prevent.
//
// Its floor follows the Astro Runtime rather than Airflow's own metadata, so
// standalone mode builds on a Python the runtime image also ships. Runtime 3.2
// and later offer 3.12 to 3.14 while Airflow itself still declares >=3.10, so
// 3.2 and later, a bare "3" (the newest 3) and anything past 3 get >=3.12. 3.0
// and 3.1 keep >=3.10.
func requiresPython(airflow string) string {
	const floor = ">=3.10"
	major, rest, _ := strings.Cut(airflow, ".")
	minor, _, _ := strings.Cut(rest, ".")
	if major != "2" {
		if major == "3" && (minor == "0" || minor == "1") {
			return floor
		}
		return ">=3.12"
	}
	// Python 3.12 support arrived in Airflow 2.9. Before that the ceiling is
	// 3.11, which docs/install.md states in the same words: "Airflow 2.7 wants
	// 3.11 or lower; later 2.x releases reach further." A flat <3.13 would let
	// uv pick 3.12 for a 2.7 project and produce the failure this is here to
	// prevent, one minor version along.
	//
	// A bare "2" means the newest Airflow 2, which is past 2.9, so it takes the
	// wider bound — Atoi fails on the empty minor and falls through.
	if minor != "" {
		if n, err := strconv.Atoi(minor); err == nil && n < 9 {
			return floor + ",<3.12"
		}
	}
	return floor + ",<3.13"
}

// onlyTheDistribution reports whether the text before a requirement's "=="
// is the distribution name and nothing else — extras allowed, another
// specifier not.
func onlyTheDistribution(head string) bool {
	h := strings.TrimSpace(head)
	if i := strings.Index(h, "["); i >= 0 {
		j := strings.Index(h, "]")
		if j < i {
			return false
		}
		h = strings.TrimSpace(h[:i] + h[j+1:])
	}
	return !strings.ContainsAny(h, "<>!~,=")
}

// pinsAirflow reports whether [project.dependencies] already names
// apache-airflow, however it is pinned.
func pinsAirflow(deps []string) bool {
	return slices.ContainsFunc(deps, namesAirflow)
}

// pinnedPastTheManifest reports that the manifest names Airflow in a shape no
// single version reads out of — a range — while the pin written to
// [tool.astro] came from a Dockerfile or a requirements.txt.
//
// Nothing downstream compares the two: the CLI runs the [tool.astro] one and
// uv resolves the dependency one, so a disagreement surfaces later as a
// failure with no obvious cause. Saying so at adoption is the only cheap
// moment.
//
// Not the only way two halves can disagree, and the doc should not claim
// otherwise: a project whose declared Dockerfile carries a different Airflow
// generation from the pin is another, and this does not see it. What that one
// needs is a comparison against the Dockerfile's own FROM, which belongs with
// the Dockerfile reading rather than here.
//
// fromFlag suppresses it. --airflow-version is the user stating the version
// themselves, so "make the two agree, or set airflow explicitly" would be
// advising them to do the thing they just did.
func pinnedPastTheManifest(deps []string, defaulted, fromFlag bool) bool {
	if defaulted || fromFlag || !pinsAirflow(deps) {
		return false
	}
	_, readable := pinFromDeps(deps)
	return !readable
}

// distName extracts and normalizes the distribution name from a PEP 508
// requirement: the leading name, before any extras, version, marker, or URL.
// Mirrors pkg/checks.distName, internal/pack.distName and
// pkg/imagebuild.distName; the four stay separate rather than couple
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
	head, rest, found := strings.Cut(s, "==")
	if !found {
		return "", false
	}
	// Everything before the "==" has to be just the distribution name. Cut
	// keeps only what follows the FIRST one, so a requirement carrying another
	// specifier ahead of it — "apache-airflow>=2.9,==2.9.*" — arrives below as
	// a clean "2.9.*" with the ">=2.9," already discarded. That used to be
	// refused for the wrong reason, by the star check catching the wildcard;
	// reading the series made the wrong reason stop working.
	if !onlyTheDistribution(head) {
		return "", false
	}
	v := strings.TrimSpace(rest)
	// A trailing ".*" names a series, which is exactly what tool.astro.airflow
	// means by "3.1": the newest 3.1. So it is read rather than refused.
	//
	// It is also the shape this package writes — a scaffolded project pins
	// "apache-airflow==3.1.*" — so refusing it meant the scaffold could not
	// read back its own output. Adopting a project pinned that way fell
	// through to the next source, and where that source was a v1 Dockerfile
	// the manifest ended up with a [tool.astro] pin its own dependency
	// contradicts: airflow = '2' beside apache-airflow==3.0.*. Nothing
	// reconciles those afterwards, and the project runs one Airflow while
	// declaring another.
	if strings.Contains(v, ",") { // another specifier after this one
		return "", false
	}
	v = strings.TrimSuffix(v, ".*")
	if strings.Contains(v, "*") { // a wildcard anywhere but the tail
		return "", false
	}
	if !manifest.ValidAirflowVersion(v) {
		return "", false
	}
	return v, true
}
