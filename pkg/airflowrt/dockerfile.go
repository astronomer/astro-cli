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
//
// The reference comes back as written: a FROM built from a build argument
// keeps its '$' (IsUnresolvedRef). ReadDeclaredBase is the reader that
// expands one.
func ParseDockerfileAt(dockerfilePath string) (image, tag string, err error) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return "", "", fmt.Errorf("error reading Dockerfile: %w", err)
	}
	stages, err := readStages(data)
	if err != nil {
		return "", "", err
	}
	refs := make([]string, len(stages))
	for i, st := range stages {
		refs[i] = st.ref
	}
	image, tag = splitImageRef(refs[baseStage(stages, refs)])
	return image, tag, nil
}

// stage is one FROM: the reference it builds on, as written, and the alias it
// is given, lower-cased.
type stage struct{ alias, ref string }

// baseStage is the index of the stage whose reference the final stage builds
// on: the final one, followed back through any stage alias it names. refs[i]
// is stage i's reference as the caller reads it, so that a reference built
// from a build argument (`FROM build-${ENV}`) can name an alias once expanded.
// An empty ref names nothing.
//
// Each hop moves strictly earlier in the file, as a FROM can only name an
// earlier stage, so this ends.
func baseStage(stages []stage, refs []string) int {
	cur := len(stages) - 1
	for {
		earlier := -1
		for i := range cur {
			if stages[i].alias != "" && refs[cur] != "" && strings.EqualFold(stages[i].alias, refs[cur]) {
				earlier = i
			}
		}
		if earlier < 0 {
			return cur
		}
		cur = earlier
	}
}

// readStages reads every FROM in the Dockerfile in data, in order.
func readStages(data []byte) ([]stage, error) {
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
	// followed to the image they resolve to (baseStage).
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
		return nil, fmt.Errorf("no FROM instruction found in Dockerfile")
	}
	return stages, nil
}

// splitImageRef separates an image reference into its name and tag.
//
// The tag is what follows the last colon AFTER the last slash, not the first
// colon in the string: a registry host may carry a port, and splitting
// localhost:5000/astro-runtime:3.1-12 on the first one reads the image as
// "localhost".
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
// Another definition exists and is NOT this one. internal/platform/apc names
// a wider set — including astronomerinc/ap-airflow — for the v1 Software
// deploy path, which warns rather than refusing and does not write this
// compose file. It is not safe to fold in here without knowing that those
// images carry the `astro` user, which is what this gate is for.
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

// IsUnresolvedRef reports a FROM reference built from a build argument, which
// ParseDockerfileAt returns as written.
//
// `ARG BASE=...` then `FROM ${BASE}` is a real and supported shape, and its
// value can also come from the command line, so the text alone does not say
// what it builds on. ReadDeclaredBase expands it the way astro's own build
// does, from the defaults; a caller of ParseDockerfileAt has to treat it the
// way it treats a file it could not read: as no answer rather than a wrong one.
func IsUnresolvedRef(image string) bool {
	return strings.Contains(image, "$")
}

// DeclaredBase is what a project's own Dockerfile builds on, as far as the file
// alone can say: the final stage's image and tag, followed back through stage
// aliases, with its build arguments replaced by their defaults.
//
// One reader for every caller that asks, so the refusal of a foreign base, the
// generation a compose file describes, the check that the FROM agrees with
// the manifest's Airflow requirement, and astro init's decision whether to
// declare the file all read the same line the same way.
//
// The defaults, because nothing in astro passes a --build-arg: Docker mode,
// deploy and package all build the image the defaults name.
type DeclaredBase struct {
	Image, Tag string
	// Known is false when there is no declared Dockerfile, when the file could
	// not be parsed, and when its FROM uses a build argument that could not be
	// expanded (ArgProblem says why). All three mean the same thing to a
	// caller deciding whether to refuse: no answer, rather than a wrong one.
	Known bool
	// ArgProblem says why a From that uses build arguments could not be
	// expanded, and is ArgNone otherwise.
	ArgProblem ArgProblem
	// From is the final stage's reference as the file writes it, build
	// arguments unexpanded, for a message; "" when no FROM was read.
	From string
	// Expanded is From with its build arguments replaced by their defaults,
	// the reference astro's build pulls. It equals From when there are none,
	// and is "" when they could not be expanded.
	Expanded string
}

// ReadDeclaredBase reads the base out of a project's declared Dockerfile, at
// path. An empty path is no Dockerfile.
//
// A file that cannot be parsed is neither an answer nor a failure here. The
// build reports that with the path and the reason, and guessing at this depth
// would replace a good message with a worse one.
//
// A FROM built from build arguments is expanded from the ARGs declared before
// the first FROM, the only ones a FROM can use, each default expanded against
// the ones before it: $NAME, ${NAME}, ${NAME:-word} and ${NAME:+word}, nested,
// with quotes removed and an argument with no value empty, as Docker does.
// Every stage is expanded before stage aliases are followed. A result that is
// an image reference is the base; what is not is unknown, never some other
// base: empty or not a reference because an argument had no value
// (ArgUnset), or anything this does not expand (ArgUnsupported).
func ReadDeclaredBase(path string) DeclaredBase {
	if path == "" {
		return DeclaredBase{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DeclaredBase{}
	}
	stages, err := readStages(data)
	if err != nil {
		return DeclaredBase{}
	}
	// Every stage's reference is expanded before aliases are followed, so
	// `FROM build-${ENV}` and `FROM ${STAGE}` can name an earlier stage. One
	// that could not be expanded names none, and is the answer only if the
	// final stage reaches it.
	args := globalBuildArgs(data)
	refs := make([]string, len(stages))
	problems := make([]ArgProblem, len(stages))
	for i, st := range stages {
		refs[i], problems[i] = args.expandRef(st.ref)
	}
	i := baseStage(stages, refs)
	base := DeclaredBase{From: stages[i].ref, ArgProblem: problems[i], Expanded: refs[i]}
	if base.ArgProblem != ArgNone {
		return base
	}
	base.Image, base.Tag = splitImageRef(base.Expanded)
	base.Known = true
	return base
}

// RuntimeVersion is the Astro Runtime version the base names: its tag with any
// flavor suffix stripped ("3.3-8" from "3.3-8-python-3.12"). Empty when the
// base is unknown, is not an Astro Runtime image, or is pinned by digest, which
// names an exact image and no version.
func (b DeclaredBase) RuntimeVersion() string {
	if !b.Known || !IsAstroRuntimeImage(b.Image) || b.Tag == "" {
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
