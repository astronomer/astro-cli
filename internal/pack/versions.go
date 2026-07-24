package pack

import "strings"

// platformVersion is one Apache Airflow release a managed platform offers.
// python is the tag the constraints file is named for (MWAA builds a
// constraints URL from it); Composer resolves constraints against the
// environment itself, so its entries leave python empty.
type platformVersion struct {
	airflow string
	python  string
}

// The Python each MWAA image runs, from the same table as the Airflow versions.
const (
	py311 = "3.11"
	py312 = "3.12"
)

// mwaaAirflowVersions is the set of Apache Airflow versions Amazon MWAA offers,
// newest first, each with the Python it runs. The mwaa target compares the
// manifest pin against this list and builds the --constraint URL from the match.
//
// Source: https://docs.aws.amazon.com/mwaa/latest/userguide/airflow-versions.html
// (checked 2026-07). MWAA adds a version and retires an old one a few times a
// year, so this is data to refresh, not logic to rewrite.
var mwaaAirflowVersions = []platformVersion{
	{airflow: "3.2.1", python: py312},
	{airflow: "3.0.6", python: py312},
	{airflow: "2.11.0", python: py312},
	{airflow: "2.10.3", python: py311},
	{airflow: "2.10.1", python: py311},
	{airflow: "2.9.2", python: py311},
	{airflow: "2.8.1", python: py311},
	{airflow: "2.7.2", python: py311},
}

// composerAirflowVersions is the set of Apache Airflow versions Cloud Composer 3
// offers, newest first. Composer installs PyPI packages onto the environment and
// resolves their constraints there, so no Python tag is carried.
//
// Source: https://docs.cloud.google.com/composer/docs/composer-versions
// (checked 2026-07). As with MWAA, refresh the data; the target logic does not
// change.
var composerAirflowVersions = []platformVersion{
	{airflow: "3.1.8"},
	{airflow: "3.1.7"},
	{airflow: "3.1.0"},
	{airflow: "2.11.1"},
	{airflow: "2.10.5"},
	{airflow: "2.10.2"},
	{airflow: "2.9.3"},
}

// resolveVersion matches a manifest Airflow pin against a platform's supported
// list (newest first). An exact pin wins; a partial pin ("3", "3.1") matches the
// newest supported version under it, so "3.1" resolves to "3.1.8" where the
// platform offers it. The bool reports whether the pin is supported at all — an
// unsupported pin still builds a valid artifact, only with a warning.
func resolveVersion(pin string, supported []platformVersion) (platformVersion, bool) {
	pin = strings.TrimSpace(pin)
	for _, v := range supported {
		if v.airflow == pin {
			return v, true
		}
	}
	// The list is newest first, so the first prefix hit is the newest version
	// under the partial pin.
	for _, v := range supported {
		if strings.HasPrefix(v.airflow, pin+".") {
			return v, true
		}
	}
	return platformVersion{}, false
}

// supportedList renders a platform's Airflow versions as a comma-separated
// string for a warning or a requirements-file comment.
func supportedList(supported []platformVersion) string {
	out := make([]string, len(supported))
	for i, v := range supported {
		out[i] = v.airflow
	}
	return strings.Join(out, ", ")
}
