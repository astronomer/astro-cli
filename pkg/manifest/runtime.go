package manifest

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// [tool.astro] runtime picks one Astro Runtime build for the image: Docker
// mode, astro deploy and astro package build FROM runtime:<tag> rather than the
// newest build of the requirement's series. The requirement cannot say which
// build (3.3-3 to 3.3-7 all carry Airflow 3.3.1 and differ in everything else),
// so this is information the requirement lacks rather than a second statement
// of it. It never stands in for the requirement: standalone installs the
// requirement, and a manifest whose requirement pins nothing is refused whether
// runtime is set or not.
//
// The overlap between the two, the Airflow the build carries, is checked here
// as far as the tag alone can say. An Airflow 3 tag carries its series ("3.3"
// in "3.3-8"), so the series is compared. An Airflow 2 tag names a runtime
// version ("13.11.0" carries Airflow 2.11.2), so only the generation can be
// read offline; the series, the exact patch and a yanked build are the runtime
// catalog's to check (pkg/runtimeversions, Catalog.CheckRuntime).
//
// The same comparison serves a declared Dockerfile's FROM line
// (DockerfileRuntimeProblem): both are a runtime tag that has to agree with the
// requirement, or Docker mode and standalone run different Airflows.

const runtimeKey = astroRoot + ".runtime"

// RuntimeTag is an Astro Runtime version, read.
type RuntimeTag struct {
	// Major is the Airflow generation the runtime carries: "3" or "2".
	Major string
	// Series is the Airflow series an Airflow 3 tag carries, "3.3" for
	// "3.3-8" and for the floating "3.3". Empty for an Airflow 2 tag, which
	// names a runtime version rather than an Airflow one, and for a tag that
	// names only the generation.
	Series string
	// Build reports that the tag names one build: "3.3-8" or "13.11.0". A
	// floating tag ("3.3", "13") names whatever the registry serves under it
	// today.
	Build bool
}

var (
	// airflow3TagRe is the runtime format Airflow 3 introduced:
	// "<airflow-major>.<airflow-minor>-<build>", or the floating series tag
	// without the build.
	airflow3TagRe = regexp.MustCompile(`^3\.(\d+)(-\d+)?$`)
	// airflow2TagRe is the runtime format before Airflow 3: a runtime version,
	// one to three segments, whose first segment is 4 or more. Runtimes 1 to 3
	// predate every Airflow Docker mode runs, and a 3.x.y tag would read as
	// neither generation with confidence.
	airflow2TagRe = regexp.MustCompile(`^(\d+)(\.\d+){0,2}$`)
)

// minAirflow2Runtime is the first runtime version an Airflow 2 tag can carry
// here; see airflow2TagRe.
const minAirflow2Runtime = 4

// ParseRuntimeTag reads an Astro Runtime version: "3.3-8" and the floating
// "3.3" for Airflow 3, "13.11.0" and the floating "13" for Airflow 2. Anything
// else, a flavor suffix included ("3.3-8-python-3.12", "13.7.0-slim"),
// reports false; a caller reading a Dockerfile's tag strips the flavor first.
func ParseRuntimeTag(tag string) (RuntimeTag, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "3" {
		return RuntimeTag{Major: "3"}, true
	}
	if m := airflow3TagRe.FindStringSubmatch(tag); m != nil {
		return RuntimeTag{Major: "3", Series: "3." + m[1], Build: m[2] != ""}, true
	}
	if m := airflow2TagRe.FindStringSubmatch(tag); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= minAirflow2Runtime {
			return RuntimeTag{Major: "2", Build: strings.Count(tag, ".") == 2}, true
		}
	}
	return RuntimeTag{}, false
}

// Agrees reports whether the tag agrees with a requirement's pin as far as the
// tag can tell: the same generation, and for an Airflow 3 tag the same series.
// It is the comparison Parse applies to [tool.astro] runtime and
// DockerfileRuntimeProblem to a FROM line, for a writer about to decide whether
// a runtime line survives a change of pin.
func (t RuntimeTag) Agrees(pin string) bool {
	return t.disagreement(pin) == ""
}

// The ways a runtime tag can disagree with a requirement's pin.
const (
	otherGeneration = "generation"
	otherSeries     = "series"
)

// disagreement says how a runtime tag and a requirement's pin disagree, as the
// tag alone can tell: otherGeneration, otherSeries, or "" when they agree or
// the tag says too little to compare.
func (t RuntimeTag) disagreement(pin string) string {
	a := Airflow{Pin: pin}
	switch {
	case t.Major != a.Major():
		return otherGeneration
	case t.Series != "" && strings.Contains(pin, ".") && t.Series != a.Series():
		return otherSeries
	}
	return ""
}

// describe names the Airflow a tag carries, as far as the tag says.
func (t RuntimeTag) describe() string {
	if t.Series != "" {
		return "Airflow " + t.Series
	}
	return "an Airflow " + t.Major
}

// requirementLine is the Airflow requirement as written, for a message.
func (m *Manifest) requirementLine() string {
	for _, spec := range m.Project.Dependencies {
		if _, ok := AirflowPin(spec); ok {
			return strings.TrimSpace(spec)
		}
	}
	return AirflowRequirement(m.Airflow().Pin)
}

// runtime checks [tool.astro] runtime: its shape, that no dockerfile is
// declared beside it, and that the build agrees with the requirement.
//
// The comparison is skipped when the requirement itself is unclear, because
// there is nothing to compare against and the requirement's own problem
// already says what to fix first.
func (p *parser) runtime(m *Manifest) {
	tag := m.Astro.Runtime
	if tag == "" {
		return
	}
	if m.Astro.Dockerfile != "" {
		p.add(CodeRuntimeWithDockerfile, runtimeKey, fmt.Sprintf(
			"picks a runtime build, and the declared dockerfile %s names its own base image in its FROM line, so this would pick nothing. "+
				"Delete the runtime line, or change the FROM line", m.Astro.Dockerfile))
		return
	}
	t, ok := ParseRuntimeTag(tag)
	switch {
	case !ok:
		p.add(CodeRuntimeInvalid, runtimeKey, fmt.Sprintf(
			"%q is not an Astro Runtime build. Name one like 3.3-8 (Airflow 3) or 13.11.0 (Airflow 2), or delete the line", tag))
		return
	case !t.Build:
		p.add(CodeRuntimeInvalid, runtimeKey, fmt.Sprintf(
			"%q names whatever the registry serves under that tag, not one build, and the requirement already builds from the newest one. "+
				"Name one build, like %s, or delete the line", tag, exampleBuild(t)))
		return
	}
	pin := m.Airflow().Pin
	if pin == "" || p.airflowUnclear() {
		return
	}
	req := m.requirementLine()
	switch t.disagreement(pin) {
	case otherGeneration:
		want := Airflow{Pin: pin}.Major()
		p.add(CodeRuntimeMismatch, runtimeKey, fmt.Sprintf(
			"%q is an Astro Runtime build for %s, and the requirement %s runs Airflow %s. "+
				"Name an Airflow %s build, or delete the runtime line to build from the requirement's own series",
			tag, t.describe(), req, want, want))
	case otherSeries:
		want := Airflow{Pin: pin}.Series()
		p.add(CodeRuntimeMismatch, runtimeKey, fmt.Sprintf(
			"%q is an Astro Runtime build of %s, and the requirement %s runs Airflow %s. "+
				"Name a %s build (%s-<build>), or delete the runtime line to build from the newest %s runtime",
			tag, t.describe(), req, want, want, want, want))
	}
}

// exampleBuild is a build tag of the shape t has, for a message.
func exampleBuild(t RuntimeTag) string {
	if t.Major == "2" {
		return "13.11.0"
	}
	if t.Series != "" {
		return t.Series + "-1"
	}
	return exampleSeries + "-8"
}

// airflowUnclear reports that the requirement states no single version, from
// the problems recorded so far.
func (p *parser) airflowUnclear() bool {
	for _, pr := range p.problems {
		if slices.Contains(unclearAirflowCodes, pr.Code) {
			return true
		}
	}
	return false
}

// unclearAirflowCodes are the problems that leave a requirement stating no
// single version.
var unclearAirflowCodes = []ProblemCode{CodeAirflowMissing, CodeAirflowUnpinned, CodeAirflowAmbiguous}

// DockerfileRuntimeProblem compares a declared Dockerfile's base image with the
// Airflow requirement, and returns the problem when they disagree.
//
// from is the final stage's image reference as the file writes it, for the
// message, and tag is its Astro Runtime version with any flavor suffix
// stripped ("3.3-8" from "3.3-8-python-3.12"). The caller reads both from the
// file and establishes that the image is an Astro Runtime image: this package
// does no I/O and knows no registries. pkg/scaffold's CheckDockerfileAirflow is
// that caller, and the one every run path calls.
//
// The series is compared for an Airflow 3 tag and the generation for an
// Airflow 2 one, as for [tool.astro] runtime. A tag ParseRuntimeTag cannot
// read is no answer rather than a wrong one, and reports false.
//
// This is a refusal rather than a note because the two disagreeing is the
// silent split the requirement exists to end: standalone installs the
// requirement, and Docker mode runs whatever the file's FROM names.
func (m *Manifest) DockerfileRuntimeProblem(from, tag string) (Problem, bool) {
	t, ok := ParseRuntimeTag(tag)
	pin := m.Airflow().Pin
	if !ok || pin == "" {
		return Problem{}, false
	}
	req := m.requirementLine()
	var fix string
	switch t.disagreement(pin) {
	case otherGeneration:
		if t.Major == "2" {
			fix = "change the requirement to the Airflow 2 series that runtime carries (apache-airflow==2.<minor>.*), or base the file on an Airflow 3 runtime"
		} else {
			series := t.Series
			if series == "" {
				series = exampleSeries
			}
			fix = fmt.Sprintf("change the requirement to %s, or base the file on an Airflow 2 runtime", requirementFor(DistName(req), series))
		}
	case otherSeries:
		fix = fmt.Sprintf("change the requirement to %s, or change the FROM line to a %s runtime", requirementFor(distFor(DistName(req), t.Series), t.Series), Airflow{Pin: pin}.Series())
	default:
		return Problem{}, false
	}
	return Problem{
		Code: CodeDockerfileAirflowMismatch,
		Key:  astroRoot + ".dockerfile",
		Reason: fmt.Sprintf("%s builds FROM %s, which is %s, and the requirement %s in [project] dependencies is Airflow %s: "+
			"Docker mode would run one and standalone the other. To fix it, %s",
			m.Astro.Dockerfile, from, t.describe(), req, describePin(pin), fix),
	}, true
}

// describePin is a pin as a message names it: its series, or its generation
// when it names no minor.
func describePin(pin string) string {
	return Airflow{Pin: pin}.Series()
}
