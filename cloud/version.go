package cloud

import (
	"strings"
)

// GetShortVersion returns the major.minor portion of a version string.
// For example, "1.40.1" returns "1.40", "2.5.3-rc1" returns "2.5".
// If the version cannot be parsed, it returns the original string.
func GetShortVersion(version string) string {
	// Remove any leading "v" prefix
	version = strings.TrimPrefix(version, "v")

	// Split by "." to get version components
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		// If we don't have at least major.minor, return original
		return version
	}

	// Handle pre-release versions (e.g., "1.40.1-rc1")
	// We need to clean the patch version if it contains a hyphen
	if len(parts) >= 3 && strings.Contains(parts[2], "-") {
		// Keep only the major.minor
		return parts[0] + "." + parts[1]
	}

	// Return major.minor
	return parts[0] + "." + parts[1]
}
