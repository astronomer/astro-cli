package manifest

import (
	"fmt"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// The Airflow a project runs is the apache-airflow (or apache-airflow-core)
// requirement in [project] dependencies. It is the line every tool reads — uv,
// pip, ty, Dependabot, an IDE — and the one standalone mode installs from, so
// every reader in the CLI and in Astro Desktop derives the version from it
// through Manifest.Airflow rather than from a second key of its own.

// The two distributions that state the Airflow version, by normalized (PEP 503)
// name. apache-airflow-core is the slimmer one Airflow 3 publishes without the
// bundled providers, and pins the version just the same.
const (
	airflowDist     = "apache-airflow"
	airflowCoreDist = "apache-airflow-core"
)

// Airflow is the Airflow a project runs, as its manifest states it.
type Airflow struct {
	// Pin is the version the requirement pins: the series "3.3" from
	// ==3.3.* and "3" from ==3.*, or the release "3.3.2" from ==3.3.2 and
	// "3.3.0" from ==3.3. A series resolves to a concrete release downstream,
	// not here.
	Pin string
}

// Airflow returns the Airflow the manifest's requirement pins. A manifest that
// loaded always has one, because Parse refuses a manifest whose requirement is
// missing or pins no single series, so its callers need no empty case. A
// Manifest built by hand without a pinned requirement returns the zero value.
func (m *Manifest) Airflow() Airflow {
	for _, spec := range m.Project.Dependencies {
		if v, ok := AirflowPin(spec); ok {
			return Airflow{Pin: v}
		}
	}
	return Airflow{}
}

// Major is the pin's Airflow generation: "3" for 3.3 and for 3.3.2.
func (a Airflow) Major() string {
	major, _, _ := strings.Cut(a.Pin, ".")
	return major
}

// Series is the pin to its minor: "3.3" for 3.3 and for 3.3.2, and "3" for a
// pin that names only the generation.
func (a Airflow) Series() string {
	return series(a.Pin)
}

func series(v string) string {
	major, rest, found := strings.Cut(v, ".")
	if !found {
		return major
	}
	minor, _, _ := strings.Cut(rest, ".")
	return major + "." + minor
}

// AirflowRequirement is the [project] dependencies entry that pins version: a
// partial version ("3", "3.3") becomes a prefix match ("apache-airflow==3.3.*")
// so the project tracks patch releases, and a full "3.3.2" stays exact.
// AirflowPin reads every requirement this writes back as the version it was
// given.
func AirflowRequirement(version string) string {
	return requirementFor(airflowDist, version)
}

func requirementFor(dist, version string) string {
	if strings.Count(version, ".") < 2 {
		return dist + "==" + version + ".*"
	}
	return dist + "==" + version
}

// WithoutAirflow returns deps without the requirements that state the Airflow
// version (apache-airflow and apache-airflow-core), for a caller whose Airflow
// comes from somewhere else: a runtime image, a managed platform, or a check
// environment that installs the Airflow it is checking against. Every other
// entry, the providers and the task SDK included, is kept in order.
func WithoutAirflow(deps []string) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if !NamesAirflow(d) {
			out = append(out, d)
		}
	}
	return out
}

// DistName extracts and normalizes the distribution name from a PEP 508
// requirement: the leading name, before any extras, version, marker, or URL.
func DistName(req string) string {
	s := strings.TrimSpace(req)
	if i := strings.IndexAny(s, "[ \t<>=!~;@("); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

// NamesAirflow reports whether a requirement is one of the two distributions
// that state the Airflow version, however it is pinned, or whether it is
// pinned at all. The providers and the task SDK are their own distributions.
func NamesAirflow(spec string) bool {
	switch DistName(spec) {
	case airflowDist, airflowCoreDist:
		return true
	}
	return false
}

// AirflowPin returns the version an Airflow requirement pins, when it carries a
// clean "==" pin: "apache-airflow==3.3.*" is the series "3.3",
// "apache-airflow==3.3.2" is "3.3.2", and "apache-airflow==3.3" is "3.3.0",
// the one release PEP 440 says it matches. Extras are allowed and an
// environment marker is ignored. A range, a direct URL, a wildcard anywhere
// but the tail, or a second specifier yield no pin, since none of them names
// one series or release.
func AirflowPin(spec string) (version string, ok bool) {
	if !NamesAirflow(spec) {
		return "", false
	}
	s := spec
	// An environment marker or a direct URL follows the specifier, so the pin
	// is whatever comes before it. A URL is then refused below, because the
	// name alone carries no "==".
	if i := strings.IndexAny(s, ";@"); i >= 0 {
		s = s[:i]
	}
	head, rest, found := strings.Cut(s, "==")
	if !found {
		return "", false
	}
	// Everything before the "==" has to be just the distribution name. Cut
	// keeps only what follows the FIRST one, so a requirement carrying another
	// specifier ahead of it, "apache-airflow>=2.9,==2.9.*", arrives below as a
	// clean "2.9.*" with the ">=2.9," already discarded.
	if !onlyTheDistribution(head) {
		return "", false
	}
	v := strings.TrimSpace(rest)
	if strings.Contains(v, ",") { // another specifier after this one
		return "", false
	}
	// A trailing ".*" names a series, which is the shape AirflowRequirement
	// writes, so it is read rather than refused.
	series := strings.HasSuffix(v, ".*")
	v = strings.TrimSuffix(v, ".*")
	if strings.Contains(v, "*") { // a wildcard anywhere but the tail
		return "", false
	}
	if !ValidAirflowVersion(v) {
		return "", false
	}
	if !series {
		v = exactVersion(v)
	}
	return v, true
}

// exactVersion spells out the release a "==" pin without a wildcard names.
// PEP 440 pads the missing parts with zeros, so "==3.1" is exactly 3.1.0 and
// "==3" is 3.0.0, not the newest of a series: reading them as one would build
// the image from runtime:3.1's newest patch while standalone installs 3.1.0.
func exactVersion(v string) string {
	for strings.Count(v, ".") < 2 {
		v += ".0"
	}
	return v
}

// onlyTheDistribution reports whether the text before a requirement's "=="
// is the distribution name and nothing else: extras allowed, another
// specifier not.
func onlyTheDistribution(head string) bool {
	h := strings.TrimSpace(head)
	if i := strings.Index(h, "["); i >= 0 {
		j := strings.Index(h, "]")
		if j < i {
			return false
		}
		h = strings.TrimSpace(h[:i] + h[j+1:])
	}
	return !strings.ContainsAny(h, "<>!~,=")
}

// exampleSeries is the series an error's suggested fix names. It is an
// example of the shape, not a default: the default is chosen where a project
// is created, and this package never writes a version.
const exampleSeries = "3.3"

const dependenciesKey = "project.dependencies"

// airflow checks the Airflow requirement and reports a leftover [tool.astro]
// airflow.
//
// A requirement that pins no single series is refused in every mode, not
// only the one that installs from it. Docker mode needs the series for the
// runtime image and standalone needs the generation for its process layout,
// both before anything is installed, so one rule serves both, and a manifest
// valid in one mode is valid in the other.
func (p *parser) airflow(m *Manifest) {
	var (
		named    int
		pinned   []string // the requirement lines that pin, as written
		pins     []string // what each one pins, deduplicated
		dists    = map[string]bool{}
		unpinned bool
	)
	for i, spec := range m.Project.Dependencies {
		if !NamesAirflow(spec) {
			continue
		}
		named++
		dist := DistName(spec)
		dists[dist] = true
		v, ok := AirflowPin(spec)
		if !ok {
			unpinned = true
			p.add(CodeAirflowUnpinned, fmt.Sprintf("%s[%d]", dependenciesKey, i),
				fmt.Sprintf("%s does not pin an Airflow series. Pin one, like %s",
					strings.TrimSpace(spec), requirementFor(dist, exampleSeries)))
			continue
		}
		if dist == airflowCoreDist && (Airflow{Pin: v}).Major() == "2" {
			p.add(CodeAirflowCoreBeforeThree, fmt.Sprintf("%s[%d]", dependenciesKey, i),
				fmt.Sprintf("%s pins an Airflow 2, and apache-airflow-core is published only for Airflow 3. Use %s",
					strings.TrimSpace(spec), requirementFor(airflowDist, v)))
		}
		pinned = append(pinned, strings.TrimSpace(spec))
		if !slices.Contains(pins, v) {
			pins = append(pins, v)
		}
	}
	if p.dynamicDeps {
		p.add(CodeDependenciesDynamic, "project.dynamic",
			"lists dependencies, and an Astro project states its Airflow version as the requirement in [project] dependencies: "+
				"list the dependencies there, starting with the apache-airflow requirement, and drop dependencies from [project] dynamic")
	}
	switch {
	case named == 0:
		p.add(CodeAirflowMissing, dependenciesKey,
			"names no Airflow. Add one, like "+AirflowRequirement(exampleSeries))
	case len(dists) > 1:
		p.add(CodeAirflowAmbiguous, dependenciesKey,
			"lists both apache-airflow and apache-airflow-core, and the Airflow version comes from one requirement. Keep one of them")
	case len(pins) > 1:
		p.add(CodeAirflowAmbiguous, dependenciesKey,
			fmt.Sprintf("pins Airflow more than once, to different versions (%s). Pin it once", strings.Join(pins, ", ")))
	}
	if p.airflowKey == nil {
		return
	}
	stated, _ := p.airflowKey.(string)
	if !ValidAirflowVersion(stated) {
		stated = ""
	}
	if unpinned || len(pinned) == 0 || len(dists) > 1 || len(pins) > 1 {
		// The distribution the project already names, so a core project is not
		// told to add the full one beside it.
		dist := airflowDist
		if len(dists) == 1 {
			for d := range dists {
				dist = d
			}
		}
		reason := "is no longer read: the Airflow version is the " + dist + " requirement in [project] dependencies. Delete the airflow line."
		if stated != "" {
			reason += fmt.Sprintf(" It said %s; to keep running %s, the requirement is %s.", stated, stated, requirementFor(distFor(dist, stated), stated))
		}
		p.add(CodeAirflowRemoved, astroRoot+".airflow", reason)
		return
	}
	reason := fmt.Sprintf("is no longer read: the Airflow version is the requirement %s in [project] dependencies. Delete the airflow line.", pinned[0])
	if runs := pins[0]; stated != "" && series(stated) != series(runs) {
		reason += fmt.Sprintf(" It said %s, and this project runs %s; to run %s, change the requirement to %s.",
			stated, runs, stated, requirementFor(distFor(DistName(pinned[0]), stated), stated))
	}
	p.add(CodeAirflowRemoved, astroRoot+".airflow", reason)
}

// distFor is the distribution a requirement for version should name, given
// the one the project uses: apache-airflow-core exists only for Airflow 3.
func distFor(dist, version string) string {
	if dist == airflowCoreDist && (Airflow{Pin: version}).Major() == "2" {
		return airflowDist
	}
	return dist
}

// AirflowStatement is what a manifest says about its Airflow version, read
// whatever else is wrong with it.
type AirflowStatement struct {
	// Requirement is the version the Airflow requirement pins, "" when it
	// states none: missing, pinning no single series, or pinning two.
	Requirement string
	// Key is what a leftover [tool.astro] airflow line says, "" when there is
	// none or it is not a version.
	Key string
}

// ReadAirflowStatement reads what data says about its Airflow version and
// nothing else, so a manifest with an unrelated problem still answers. It is
// for a caller identifying an Airflow already running from the manifest, not
// for one about to run it: that caller loads the manifest, and is refused.
// The errors are the ones that leave nothing to read: a file that is not
// TOML (*ParseError) and one with no [tool.astro] (ErrNoAstroSection).
func ReadAirflowStatement(data []byte) (AirflowStatement, error) {
	var f wireFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return AirflowStatement{}, &ParseError{Err: err}
	}
	if f.Tool.Astro == nil {
		return AirflowStatement{}, ErrNoAstroSection
	}
	var out AirflowStatement
	if key, ok := (*f.Tool.Astro)["airflow"].(string); ok && ValidAirflowVersion(key) {
		out.Key = key
	}
	if f.Project == nil {
		return out, nil
	}
	p := &parser{}
	m := &Manifest{Project: Project{Dependencies: f.Project.Dependencies}}
	p.airflow(m)
	for _, pr := range p.problems {
		if pr.Code == CodeAirflowMissing || pr.Code == CodeAirflowUnpinned || pr.Code == CodeAirflowAmbiguous {
			return out, nil
		}
	}
	out.Requirement = m.Airflow().Pin
	return out, nil
}
