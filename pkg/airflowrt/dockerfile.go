package airflowrt

import (
	"errors"
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
		// Tokenized rather than prefix-matched. docker splits an instruction on
		// any run of whitespace, so `FROM<tab>image` is valid and a "FROM "
		// prefix misses it — and a missed FROM is not a missed opportunity for
		// a caller that refuses on what this returns.
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		rest := fields[1:]
		// Per-stage flags come before the reference. `FROM --platform=$BUILDPLATFORM
		// <image>` is the ordinary workaround for building an amd64 image on an
		// Apple-silicon machine, and treating the flag as the image name reads
		// every such file as an unknown base.
		for len(rest) > 0 && strings.HasPrefix(rest[0], "--") {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			continue
		}
		ref, alias := rest[0], ""
		if len(rest) >= 3 && strings.EqualFold(rest[1], "AS") {
			alias = strings.ToLower(rest[2])
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
			if st.alias != "" && strings.EqualFold(st.alias, ref) {
				earlier = i
			}
		}
		if earlier < 0 {
			break
		}
		ref = stages[earlier].ref
	}

	image, tag = splitImageRef(ref)
	return image, tag, nil
}

// splitImageRef separates an image reference into its name and tag.
//
// The tag is what follows the last colon AFTER the last slash, not the first
// colon in the string: a registry host may carry a port, and splitting
// localhost:5000/astro-runtime:3.1-12 on the first one reads the image as
// "localhost". localdocker's postgresMajor states the same rule for the same
// reason.
//
// A digest reference pins an exact image and carries no tag. Empty rather than
// the digest, so that a caller reading a generation off the tag is told it has
// nothing to read instead of being handed a hex string that parses as Airflow 2.
func splitImageRef(ref string) (image, tag string) {
	if at := strings.Index(ref, "@"); at >= 0 {
		return ref[:at], ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > lastSlash {
		return ref[:colon], ref[colon+1:]
	}
	return ref, "latest"
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

// The registries an Astro Runtime image is published to.
//
// pkg/imagebuild carries its own copy of these two hostnames and must: it
// requires only the contract leaf, deliberately, so it cannot import this
// module. Its isAstroBase is the same rule, checked on its own side.
//
// Two other definitions exist and are NOT this one. pkg/scaffold matches the
// substring "runtime" when reading a version out of a v1 Dockerfile, so it
// reads one from a private myco/our-runtime that docker mode then refuses.
// internal/platform/apc names a wider set — including astronomerinc/ap-airflow
// — for the v1 Software deploy path, which warns rather than refusing and does
// not write this compose file. Neither is safe to fold in here without knowing
// that those images carry the `astro` user, which is what this gate is for.
const (
	astroRegistryHost  = "astrocrpublic.azurecr.io"
	quayAstronomerRepo = "quay.io/astronomer"
)

// ErrUnsupportedBase reports a declared Dockerfile whose final stage does not
// build on an Astro Runtime image.
//
// The generated compose file is written for that image and not for any image:
// it runs the services as the `astro` user and picks the service set from the
// runtime tag's generation. On another base the containers fail to start at
// all — "unable to find user astro: no matching entries in passwd file" — which
// names nothing the author wrote.
var ErrUnsupportedBase = errors.New("a declared Dockerfile must build on an Astro Runtime image")

// IsAstroRuntimeImage reports whether an image reference names an image
// published as Astro Runtime.
//
// The reference without its tag, as ParseDockerfileAt returns it. Matched
// against the registries rather than by looking for "runtime" in the name,
// which also accepts a private `myco/our-runtime` that shares none of the
// conventions the compose file depends on.
func IsAstroRuntimeImage(image string) bool {
	for _, base := range []string{astroRegistryHost, quayAstronomerRepo} {
		// A repository under the registry, not the registry alone: `FROM
		// astrocrpublic.azurecr.io` names docker.io/library/astrocrpublic.azurecr.io,
		// an unrelated image that shares none of the conventions here.
		if rest, ok := strings.CutPrefix(image, base+"/"); ok && rest != "" {
			return true
		}
	}
	return false
}

// IsUnresolvedRef reports a FROM reference this package cannot resolve on its
// own, because it is built from a build argument.
//
// `ARG BASE=...` then `FROM ${BASE}` is a real and supported shape, and its
// value can also come from the command line, so nothing here can say what it
// builds on. A caller deciding whether to refuse has to treat this the way it
// treats a file it could not read: as no answer rather than a wrong one.
func IsUnresolvedRef(image string) bool {
	return strings.Contains(image, "$")
}
