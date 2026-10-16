package airflowrt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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
// "localhost".
//
// A digest pins an exact image and is not a tag. A reference with a digest
// alone carries no tag: empty rather than the digest, so that a caller reading
// a generation off the tag is told it has nothing to read instead of being
// handed a hex string that parses as Airflow 2. One with both
// (runtime:3.1-12@sha256:...) keeps its tag, which is still what it says it is.
func splitImageRef(ref string) (image, tag string) {
	name, _, digested := strings.Cut(ref, "@")
	lastSlash := strings.LastIndex(name, "/")
	if colon := strings.LastIndex(name, ":"); colon > lastSlash {
		return name[:colon], name[colon+1:]
	}
	if digested {
		return name, ""
	}
	return name, "latest"
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

// The repositories Astronomer publishes Astro Runtime under: Airflow 3 and
// Airflow 2 images. Any tag on these is Runtime.
//
// A mirror — an exact copy pushed to another registry — keeps the repository's
// last path segment and its tag scheme, so it is recognized by both: a
// repository named runtime or astro-runtime AND a tag that is a Runtime
// version (mirrorRuntimeTagRe). The name alone would also take
// mcr.microsoft.com/dotnet/runtime:8.0 and ghcr.io/x/runtime:latest, which are
// not Astro Runtime and on which the compose file cannot start.
//
// pkg/imagebuild's isAstroBase is NOT this rule, on purpose: it decides whether
// a build may force a pull, which a mirror in a private or air-gapped registry
// must not get, so it names Astronomer's two registries only.
//
// Another definition exists and is NOT this one: internal/platform/apc names a
// wider set — including astronomerinc/ap-airflow — for the v1 Software deploy
// path, which warns rather than refusing and does not write this compose file.
var publishedRuntimeRepos = []string{
	"astrocrpublic.azurecr.io/runtime",
	"quay.io/astronomer/astro-runtime",
}

// runtimeRepoNames are the last path segments a mirror of a published
// repository keeps.
var runtimeRepoNames = []string{"runtime", "astro-runtime"}

// mirrorRuntimeTagRe is a Runtime version as a mirror's tag must spell it once
// ParseRuntimeTagPython has stripped a -python-X.Y, -base or -slim flavor: an
// exact Airflow 3 build (3.1-12) or an exact Airflow 2 runtime (12.1.0). A
// floating tag (3.1, 12, latest, stable) is not enough on a name that is not
// Astronomer's: it is what any other image called runtime carries too.
var mirrorRuntimeTagRe = regexp.MustCompile(`^(?:3\.\d+-\d+|(\d+)\.\d+\.\d+)$`)

// minAirflow2Runtime is the first Airflow 2 runtime major: runtimes 1 to 3
// predate every Airflow Docker mode runs (pkg/manifest's grammar says the same).
const minAirflow2Runtime = 4

// dockerHubHosts are the spellings of Docker Hub's registry, where a repository
// with no namespace, or under library/, is an official image and not a mirror.
var dockerHubHosts = []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"}

// ErrUnsupportedBase reports a declared Dockerfile whose final stage does not
// build on an Astro Runtime image.
//
// The generated compose file is written for that image and not for any image:
// it runs the services as the `astro` user and picks the service set from the
// runtime tag's generation. On another base the containers fail to start at
// all — "unable to find user astro: no matching entries in passwd file" — which
// names nothing the author wrote.
var ErrUnsupportedBase = errors.New("a declared Dockerfile must build on an Astro Runtime image")

// IsAstroRuntimeImage reports whether an image reference names an Astro
// Runtime image: IsAstroRuntimeRef on the reference split into its repository
// and tag. A reference given without a tag is only ever one of Astronomer's
// published repositories, since a mirror is recognized by its tag too.
func IsAstroRuntimeImage(ref string) bool {
	image, tag := splitImageRef(ref)
	return IsAstroRuntimeRef(image, tag)
}

// IsAstroRuntimeRef reports whether image, with tag ("" for none), is an Astro
// Runtime image: one of Astronomer's published repositories with any tag, or a
// mirror of one — a repository on another registry whose last path segment is
// exactly runtime or astro-runtime and whose tag is a Runtime version.
//
// Not a mirror: a Docker Hub official image (runtime, docker.io/runtime,
// docker.io/library/runtime), which no Astronomer image is; a segment that
// only contains the word (myco/our-runtime); a floating or foreign tag
// (dotnet/runtime:8.0, x/runtime:latest); and a mirror pinned by digest alone,
// which carries no tag to read. A reference built from a build argument is
// never one either: what it names is not in the text (IsUnresolvedRef). The
// comparison is exact, because Docker repository names are lowercase.
func IsAstroRuntimeRef(image, tag string) bool {
	if image == "" || IsUnresolvedRef(image) || IsUnresolvedRef(tag) {
		return false
	}
	if slices.Contains(publishedRuntimeRepos, image) {
		return true
	}
	repo := image
	for _, host := range dockerHubHosts {
		repo = strings.TrimPrefix(repo, host)
	}
	slash := strings.LastIndex(repo, "/")
	if slash < 0 || strings.HasPrefix(repo, "library/") {
		return false
	}
	if !slices.Contains(runtimeRepoNames, repo[slash+1:]) {
		return false
	}
	return isMirrorRuntimeTag(tag)
}

// isMirrorRuntimeTag reports a tag that spells an exact Runtime version, flavor
// suffixes aside (mirrorRuntimeTagRe).
func isMirrorRuntimeTag(tag string) bool {
	v, _ := ParseRuntimeTagPython(tag)
	m := mirrorRuntimeTagRe.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	if m[1] == "" {
		return true
	}
	major, err := strconv.Atoi(m[1])
	return err == nil && major >= minAirflow2Runtime
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

// DeclaredBase is what a project's own Dockerfile builds on, as far as the file
// alone can say: the final stage's image and tag, read by ParseDockerfileAt.
//
// One reader for every caller that asks, so the refusal of a foreign base, the
// generation a compose file describes and the check that the FROM agrees with
// the manifest's Airflow requirement read the same line the same way.
type DeclaredBase struct {
	Image, Tag string
	// Known is false when there is no declared Dockerfile, when the file could
	// not be parsed, and when its FROM is built from a build argument. All
	// three mean the same thing to a caller: no answer, rather than a wrong one.
	Known bool
}

// ReadDeclaredBase reads the base out of a project's declared Dockerfile, at
// path. An empty path is no Dockerfile.
//
// A file that cannot be parsed is neither an answer nor a failure here. The
// build reports that with the path and the reason, and guessing at this depth
// would replace a good message with a worse one.
func ReadDeclaredBase(path string) DeclaredBase {
	if path == "" {
		return DeclaredBase{}
	}
	image, tag, err := ParseDockerfileAt(path)
	if err != nil || IsUnresolvedRef(image) {
		return DeclaredBase{}
	}
	return DeclaredBase{Image: image, Tag: tag, Known: true}
}

// RuntimeVersion is the Astro Runtime version the base names: its tag with any
// flavor suffix stripped ("3.3-8" from "3.3-8-python-3.12"). Empty when the
// base is unknown, is not an Astro Runtime image, or is pinned by digest, which
// names an exact image and no version.
func (b DeclaredBase) RuntimeVersion() string {
	if !b.Known || !IsAstroRuntimeRef(b.Image, b.Tag) || b.Tag == "" {
		return ""
	}
	v, _ := ParseRuntimeTagPython(b.Tag)
	return v
}

// Ref is the base as a FROM line names it, image and tag, for a message.
func (b DeclaredBase) Ref() string {
	if b.Tag == "" {
		return b.Image
	}
	return b.Image + ":" + b.Tag
}
