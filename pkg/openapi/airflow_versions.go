// Package openapi provides utilities for working with OpenAPI specifications.
package openapi

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// AirflowGitHubRawURLBase is the base URL for fetching raw files from the Airflow GitHub repo.
	AirflowGitHubRawURLBase = "https://raw.githubusercontent.com/apache/airflow/refs/tags"
)

// NormalizeAirflowVersion strips build metadata (everything after "+") to get the base version.
// Prerelease info (after "-") is preserved.
// For example, "3.1.7+astro.1" becomes "3.1.7", but "3.0.0-beta" stays "3.0.0-beta".
func NormalizeAirflowVersion(version string) string {
	// Strip build metadata (everything after "+")
	if idx := strings.Index(version, "+"); idx != -1 {
		version = version[:idx]
	}
	return version
}

// VersionRange maps an Airflow version range to a spec file path.
// The range starts at Version (inclusive) and extends up to (but not including)
// the next range's Version. The last range is open-ended.
type VersionRange struct {
	Version  string // inclusive start version, e.g. "2.0.0"
	SpecPath string // path within the repo
}

// AirflowVersionRanges defines the mapping from version ranges to spec paths.
// Ranges must be ordered from oldest to newest. Each range covers versions from
// its Version up to (but not including) the next range's Version.
var AirflowVersionRanges = []VersionRange{
	{Version: "2.0.0", SpecPath: "airflow/api_connexion/openapi/v1.yaml"}, //nolint:misspell // "connexion" is the actual Airflow module name
	{Version: "3.0.0", SpecPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v1-rest-api-generated.yaml"},
	{Version: "3.0.3", SpecPath: "airflow-core/src/airflow/api_fastapi/core_api/openapi/v2-rest-api-generated.yaml"},
}

// GetSpecPathForVersion returns the spec path for a given Airflow version.
// Returns an error if the version cannot be parsed or is not in any known range.
// Ranges are matched by iterating in reverse order and returning the first range
// whose Version is <= the given version.
func GetSpecPathForVersion(version string) (string, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("invalid version %q: %w", version, err)
	}

	// Iterate in reverse: the first range where v >= range.Version is the match.
	for i := len(AirflowVersionRanges) - 1; i >= 0; i-- {
		r := AirflowVersionRanges[i]
		rangeV, err := semver.NewVersion(r.Version)
		if err != nil {
			continue // Skip invalid range definitions
		}
		if v.Compare(rangeV) >= 0 {
			return r.SpecPath, nil
		}
	}

	return "", fmt.Errorf("no spec path found for version %q", version)
}

// BuildAirflowSpecURL constructs the full GitHub raw URL for a given Airflow version.
// The version is normalized to strip build metadata (e.g., "3.1.7+astro.1" -> "3.1.7").
func BuildAirflowSpecURL(version string) (string, error) {
	// Normalize the version to strip build metadata for the GitHub tag
	normalizedVersion := NormalizeAirflowVersion(version)

	specPath, err := GetSpecPathForVersion(normalizedVersion)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s/%s", AirflowGitHubRawURLBase, normalizedVersion, specPath), nil
}
