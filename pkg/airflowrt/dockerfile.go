package airflowrt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// RuntimePythonRe matches the optional -python-X.Y (and optional -base) suffix on a runtime tag.
	RuntimePythonRe = regexp.MustCompile(`-python-(\d+\.\d+)(-base)?$`)
	// FullRuntimeTagRe matches a pinned runtime tag in the new format (X.Y-Z).
	FullRuntimeTagRe = regexp.MustCompile(`^\d+\.\d+-\d+`)
)

// ParseDockerfile extracts the runtime image name and tag from the Dockerfile at
// the root of projectPath. It is ParseDockerfileAt for the file's one v1
// location, kept because most callers only ever ask about that one.
func ParseDockerfile(projectPath string) (image, tag string, err error) {
	return ParseDockerfileAt(filepath.Join(projectPath, "Dockerfile"))
}

// ParseDockerfileAt is ParseDockerfile against a named file, for a project that
// declared its Dockerfile somewhere other than the root ([tool.astro]
// dockerfile).
//
// The path is needed because the generation a caller reads here has to come from
// the file that will actually be built. Taking it from the manifest's pin
// instead looks equivalent and is not: a conversion writes the declaration
// itself and may have DEFAULTED the pin, so a project whose Dockerfile sits on
// an Airflow 2 base can carry a pin that says 3 — and a caller trusting the pin
// then picks the compose service set for the wrong generation.
func ParseDockerfileAt(dockerfilePath string) (image, tag string, err error) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return "", "", fmt.Errorf("error reading Dockerfile: %w", err)
	}

	// The LAST stage, not the first, and then back through any alias it names.
	//
	// docker builds the final image from the final FROM, so the first one answers
	// a different question — and gets it wrong for exactly the file this parsing
	// exists to read. A multi-stage build is the headline reason a project
	// declares its own Dockerfile, and those start `FROM python:3.12-slim AS
	// builder`: taking the first FROM reported the builder's base, so an Airflow
	// 3 project read as Airflow 2 and its compose file got the Airflow 2 service
	// set. The old code even stripped the ` AS ` alias, so it knew multi-stage
	// existed and still took the wrong stage.
	//
	// A final stage may name an earlier one (`FROM base`), so aliases are
	// followed to the image they resolve to. Bounded by the number of stages,
	// since each hop moves strictly earlier in the file.
	type stage struct{ alias, ref string }
	var stages []stage
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			continue
		}
		ref, alias := strings.TrimSpace(line[5:]), ""
		if idx := strings.Index(strings.ToUpper(ref), " AS "); idx >= 0 {
			alias = strings.ToLower(strings.TrimSpace(ref[idx+4:]))
			ref = strings.TrimSpace(ref[:idx])
		}
		stages = append(stages, stage{alias: alias, ref: ref})
	}
	if len(stages) == 0 {
		return "", "", fmt.Errorf("no FROM instruction found in Dockerfile")
	}

	ref := stages[len(stages)-1].ref
	for hop := 0; hop < len(stages); hop++ {
		earlier := -1
		for i, st := range stages[:len(stages)-1] {
			if st.alias != "" && st.alias == strings.ToLower(ref) {
				earlier = i
			}
		}
		if earlier < 0 {
			break
		}
		ref = stages[earlier].ref
	}

	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return parts[0], "latest", nil
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
