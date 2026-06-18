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
// Image-variant flavors (-slim, -base) are stripped from the base tag: they
// identify a Docker base image, not a runtime version. The CDN constraints/
// freeze files and the runtime index key on the bare version, so leaving the
// flavor in would 404 the constraints fetch and miss the index lookup.
//
//	"3.1-12"                    → base="3.1-12", python=""
//	"3.1-12-python-3.11"       → base="3.1-12", python="3.11"
//	"3.1-12-python-3.11-base"  → base="3.1-12", python="3.11"
//	"13.7.0-slim"              → base="13.7.0", python=""
//	"13.7.0-slim-python-3.12"  → base="13.7.0", python="3.12"
func ParseRuntimeTagPython(tag string) (baseTag, pythonVersion string) {
	if loc := RuntimePythonRe.FindStringSubmatchIndex(tag); loc != nil {
		baseTag, pythonVersion = tag[:loc[0]], tag[loc[2]:loc[3]]
	} else {
		baseTag = tag
	}
	baseTag = strings.TrimSuffix(baseTag, "-base")
	baseTag = strings.TrimSuffix(baseTag, "-slim")
	return baseTag, pythonVersion
}

// IsValidRuntimeTag checks if a tag looks like a valid pinned runtime version (X.Y-Z).
func IsValidRuntimeTag(tag string) bool {
	return FullRuntimeTagRe.MatchString(tag)
}

// IsRuntime3 checks if a runtime tag is for Airflow 3 (runtime 3.x).
func IsRuntime3(baseTag string) bool {
	return strings.HasPrefix(baseTag, "3.")
}
