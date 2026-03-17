package airflowrt

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	// RuntimePythonRe matches the optional -python-X.Y (and optional -base) suffix on a runtime tag.
	RuntimePythonRe = regexp.MustCompile(`-python-(\d+\.\d+)(-base)?$`)
	// FullRuntimeTagRe matches a pinned runtime tag in the new format (X.Y-Z).
	FullRuntimeTagRe = regexp.MustCompile(`^\d+\.\d+-\d+`)
)

// ParseDockerfile extracts the runtime image name and tag from a project's Dockerfile.
func ParseDockerfile(projectPath string) (image, tag string, err error) {
	data, err := os.ReadFile(projectPath + "/Dockerfile")
	if err != nil {
		return "", "", fmt.Errorf("error reading Dockerfile: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			ref := strings.TrimSpace(line[5:])
			// Remove AS alias
			if idx := strings.Index(strings.ToUpper(ref), " AS "); idx >= 0 {
				ref = ref[:idx]
			}
			parts := strings.SplitN(ref, ":", 2)
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
			return parts[0], "latest", nil
		}
	}
	return "", "", fmt.Errorf("no FROM instruction found in Dockerfile")
}

// ParseRuntimeTagPython extracts the base runtime tag and the Python version from a
// full image tag. Returns an empty pythonVersion when the tag has no explicit
// -python-X.Y suffix so the caller can fall back to other sources.
//
//	"3.1-12"                    → base="3.1-12", python=""
//	"3.1-12-python-3.11"       → base="3.1-12", python="3.11"
//	"3.1-12-python-3.11-base"  → base="3.1-12", python="3.11"
func ParseRuntimeTagPython(tag string) (baseTag, pythonVersion string) {
	loc := RuntimePythonRe.FindStringSubmatchIndex(tag)
	if loc == nil {
		return strings.TrimSuffix(tag, "-base"), ""
	}
	return tag[:loc[0]], tag[loc[2]:loc[3]]
}

// IsValidRuntimeTag checks if a tag looks like a valid pinned runtime version (X.Y-Z).
func IsValidRuntimeTag(tag string) bool {
	return FullRuntimeTagRe.MatchString(tag)
}

// IsRuntime3 checks if a runtime tag is for Airflow 3 (runtime 3.x).
func IsRuntime3(baseTag string) bool {
	return strings.HasPrefix(baseTag, "3.")
}
