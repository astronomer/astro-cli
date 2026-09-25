package scaffold

import (
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The Airflow version is the apache-airflow requirement in [project]
// dependencies, and pkg/manifest owns reading it (manifest.AirflowPin). What
// is left here is choosing the version for a new or converted project and the
// interpreter bound that goes with it.

// airflowRequirement is the [project.dependencies] entry that installs the
// Airflow a new or converted project pins. See manifest.AirflowRequirement.
func airflowRequirement(version string) string {
	return manifest.AirflowRequirement(version)
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
		if v, found := manifest.AirflowPin(spec); found {
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

// pinsAirflow reports whether [project.dependencies] already names
// apache-airflow, however it is pinned.
func pinsAirflow(deps []string) bool {
	return slices.ContainsFunc(deps, manifest.NamesAirflow)
}
