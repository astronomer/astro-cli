package airflowversions

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// The versions below are in ascending order, new-format versions are newer than old format ones
// 2.1.1
// 3.0.0
// 12.5.0  old ends here
// 3.0-7   new starts here
// 3.1-1
// this regex matches the new format only
var newFormatRegex = regexp.MustCompile(`^(\d+)\.(\d+)-(\d+)(?:[.-](.+))?$`)

// The two '.'-delimited numbers before the first '-' correspond with Airflow major/minor versions
// The first number after that is the Astronomer-controlled patch version
// Anything following the patch version (delimited by '-' or '.') is the prerelease indicator

// versionParts holds the decomposed parts of an astrover string
type versionParts struct {
	airflowMajor    string
	airflowMinor    string
	astronomerPatch string
	prerelease      string
}

// stripVersionPrefix removes the 'v' prefix if present
func stripVersionPrefix(version string) string {
	return strings.TrimPrefix(version, "v")
}

// isNewFormat checks if a version uses the new format (X.Y-Z)
func isNewFormat(v string) bool {
	return newFormatRegex.MatchString(v)
}

// parseNewFormat splits a version string into its constituent parts
// Returns nil if the version string does not have valid format
func parseNewFormat(v string) *versionParts {
	matches := newFormatRegex.FindStringSubmatch(v)
	if matches == nil {
		return nil
	}

	// matches[4] is the optional prerelease part (might be empty)
	return &versionParts{
		airflowMajor:    matches[1],
		airflowMinor:    matches[2],
		astronomerPatch: matches[3],
		prerelease:      matches[4],
	}
}

// normalizeVersion converts new format to semver format
// Returns the original string if it can't be normalized
func normalizeVersion(v string) string {
	parts := parseNewFormat(v)
	if parts == nil {
		// maybe it didn't parse because it's semver, then semver will handle it
		// maybe it didn't parse because it's neither valid semver nor valid astrover, still semver will handle it
		return v
	}

	// Convert to semver format: X.Y.Z[-prerelease]
	version := fmt.Sprintf("%s.%s.%s",
		parts.airflowMajor,
		parts.airflowMinor,
		parts.astronomerPatch)

	if parts.prerelease != "" {
		version += "-" + parts.prerelease
	}

	return version
}

// isValidRuntimeVersion checks if an Astro Runtime version string can be normalized to a semver string
// If so, it either requires no fancy parsing because it uses the old-format and is already semver compatible,
// or it uses the new-format, and the fancy parsing was successful
func isValidRuntimeVersion(version string) bool {
	version = stripVersionPrefix(version)
	return semver.IsValid("v" + normalizeVersion(version))
}

// CompareRuntimeVersions compares two version strings using the versioning rules for Astro Runtime
// It aims to be otherwise identical to https://pkg.go.dev/golang.org/x/mod/semver#CompareRuntimeVersions
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
// Invalid versions are considered less than valid ones and equal to each other
func CompareRuntimeVersions(v1, v2 string) int {
	// First check validity of both versions
	v1Valid := isValidRuntimeVersion(v1)
	v2Valid := isValidRuntimeVersion(v2)

	// If both are invalid, they're equal
	if !v1Valid && !v2Valid {
		return 0
	}

	// If only one is invalid, it's less than the valid one
	if !v1Valid {
		return -1
	}
	if !v2Valid {
		return 1
	}

	// Both are valid, proceed with normal comparison
	v1 = stripVersionPrefix(v1)
	v2 = stripVersionPrefix(v2)

	v1New := isNewFormat(v1)
	v2New := isNewFormat(v2)

	// If one is new and one is old, new format always wins
	if v1New && !v2New {
		return 1
	}
	if !v1New && v2New {
		return -1
	}

	// If both are the same format, use semver comparison on normalized versions
	return semver.Compare("v"+normalizeVersion(v1), "v"+normalizeVersion(v2))
}

// RuntimeVersionMajor returns the major version prefix of the version string
// For example, RuntimeVersionMajor("3.0-1") == "3"
// If v is an invalid version string, RuntimeVersionMajor returns the empty string
func RuntimeVersionMajor(version string) string {
	version = stripVersionPrefix(version)
	v := "v" + normalizeVersion(version)
	if !semver.IsValid(v) {
		return ""
	}
	return strings.TrimPrefix(semver.Major(v), "v")
}

func AirflowMajorVersionForRuntimeVersion(runtimeVersion string) string {
	if !isNewFormat(runtimeVersion) {
		version := stripVersionPrefix(runtimeVersion)
		if semver.IsValid("v" + version) {
			return "2"
		}
		return ""
	}
	return RuntimeVersionMajor(runtimeVersion)
}
