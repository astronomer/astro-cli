package scaffold

import (
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// RuntimePins are the versions the image of one Astro Runtime build ships, as
// SetAstroBuild writes them into [tool.uv] constraint-dependencies: between
// the comments manifest.RuntimePinsComments and manifest.RuntimePinsEnd, one
// name==version a line, sorted by name, each limited to the image's Python
// (manifest.RuntimePin).
//
// They hold every package the project's lockfile shares with the image to the
// image's version on the image's Python, so standalone mode, `uv run` and the
// lockfile a build installs from all resolve what a deployment runs. A
// constraint adds no package, so the image's packages the project does not
// use stay out of it.
type RuntimePins struct {
	// Build and Python are the image's: "3.3-9" and "3.12".
	Build, Python string
	// Pins are the image's distributions, name==version, as the CLI's
	// runtimecatalog.RuntimePins reads them from the build's freeze. None
	// means there are none to write, and any runtime pins there are taken
	// out.
	Pins []string
}

// lines are the pins as SetAstroBuild writes them, between the comments that
// head them and manifest.RuntimePinsEnd, less the ones it never writes: a
// package the project gives a version the image's does not meet (own), which
// is its to choose; Airflow, the Task SDK and core, which b's pins hold; and
// any other of Astronomer's own builds (an +astro local version), which come
// from an index the project reads for Airflow alone. None at all when nothing
// is left.
func (p *RuntimePins) lines(own projectRequirements) []any {
	var pins []string
	for _, pin := range p.Pins {
		name := manifest.DistName(pin)
		if own.excludes(pin) || slices.Contains(astroDists, name) || strings.Contains(pin, "+astro.") {
			continue
		}
		pins = append(pins, pin)
	}
	if len(pins) == 0 {
		return nil
	}
	slices.SortStableFunc(pins, func(a, b string) int {
		return strings.Compare(manifest.DistName(a), manifest.DistName(b))
	})
	var out []any
	for _, c := range manifest.RuntimePinsComments(p.Build, p.Python) {
		out = append(out, tomledit.Comment(c))
	}
	for _, pin := range pins {
		out = append(out, manifest.RuntimePin(pin, p.Python))
	}
	return append(out, tomledit.Comment(manifest.RuntimePinsEnd))
}

// keptRuntimePins are the runtime pins already there, as manifest.RuntimePinsIn
// split them off, kept when the new ones are not known, less any the project
// now gives a version they do not meet (own): `uv add --frozen 'pandas>=3'`
// takes pandas over at the next write, online or not. A block written without
// the end comment gains it.
func keptRuntimePins(current []any, own projectRequirements) []any {
	var out []any
	values := 0
	ended := false
	for _, v := range current {
		switch v := v.(type) {
		case string:
			if own.excludes(v) {
				continue
			}
			values++
		case tomledit.Comment:
			ended = ended || string(v) == manifest.RuntimePinsEnd
		}
		out = append(out, v)
	}
	if values == 0 {
		return nil
	}
	if !ended {
		out = append(out, tomledit.Comment(manifest.RuntimePinsEnd))
	}
	return out
}

// Where a project names packages beyond [project] dependencies: tables of
// lists (its extras, and its dependency groups, the dev group `uv add --dev`
// writes among them) and lists (uv's overrides, and the dev dependencies
// older uv wrote).
var (
	optionalDepsKey     = []string{"project", "optional-dependencies"}
	dependencyGroupsKey = []string{"dependency-groups"}
	overrideDepsKey     = []string{"tool", "uv", "override-dependencies"}
	uvDevDepsKey        = []string{"tool", "uv", "dev-dependencies"}
)

// projectRequirements are what the project says of the versions of the
// packages it names, by distribution (manifest.DistName): every requirement
// it lists (listedRequirements), and its own lines of constraint-dependencies;
// and the packages [tool.uv] override-dependencies names, which replace any
// other requirement.
type projectRequirements struct {
	requirements map[string][]string
	overrides    map[string]bool
}

// excludes reports whether the project chooses a version of pin's package
// that pin's does not meet, so the pin would leave uv nothing to lock: an
// override of the package, or a requirement whose specifier excludes the
// pinned version (manifest.RequirementAdmits). "pandas" and "pandas>=2.3", as
// `uv add pandas` writes it, admit pandas 2.3.3 and keep the pin;
// "pandas>=3" takes it out. A requirement that cannot be read for certain
// takes it out too, which leaves uv to resolve as it would without it.
func (r projectRequirements) excludes(pin string) bool {
	name := manifest.DistName(pin)
	if r.overrides[name] {
		return true
	}
	head, _, _ := strings.Cut(pin, ";")
	_, version, ok := strings.Cut(head, "==")
	if !ok {
		return true
	}
	for _, req := range r.requirements[name] {
		if admits, known := manifest.RequirementAdmits(req, version); !known || !admits {
			return true
		}
	}
	return false
}

// listedRequirements are the requirements the project lists to install, by
// distribution: [project] dependencies, [project.optional-dependencies],
// [dependency-groups] but the astro-runtime group astro writes
// (RuntimeGroup), and the [tool.uv] dev-dependencies older uv wrote.
func listedRequirements(ed tomledit.Editor) map[string][]string {
	listed := map[string][]string{}
	addAll := func(list []any) {
		for _, v := range list {
			// A dependency group can include another, as a table; that one is
			// read where it is defined.
			if s, isString := v.(string); isString {
				name := manifest.DistName(s)
				listed[name] = append(listed[name], s)
			}
		}
	}
	for _, key := range [][]string{projectDepsKey, uvDevDepsKey} {
		value, _ := ed.Get(key)
		list, _ := value.([]any)
		addAll(list)
	}
	for _, key := range [][]string{optionalDepsKey, dependencyGroupsKey} {
		tables, _ := ed.Get(key)
		groups, _ := tables.(map[string]any)
		for name, group := range groups {
			// The runtime group is astro's list of what the image has, not the
			// project's: counted, it would hold every package in it to the
			// project's own version, and take its runtime pin out.
			if slices.Equal(key, dependencyGroupsKey) && name == manifest.RuntimeGroupName {
				continue
			}
			list, _ := group.([]any)
			addAll(list)
		}
	}
	return listed
}

// listedPackages are the distributions listedRequirements names.
func listedPackages(ed tomledit.Editor) map[string]bool {
	listed := map[string]bool{}
	for name := range listedRequirements(ed) {
		listed[name] = true
	}
	return listed
}

// ownRequirements reads projectRequirements out of the manifest open in ed,
// with constraints, the project's own lines of constraint-dependencies.
func ownRequirements(ed tomledit.Editor, constraints []any) projectRequirements {
	r := projectRequirements{requirements: listedRequirements(ed), overrides: map[string]bool{}}
	for _, v := range constraints {
		if s, isString := v.(string); isString {
			name := manifest.DistName(s)
			r.requirements[name] = append(r.requirements[name], s)
		}
	}
	overrides, _ := ed.Get(overrideDepsKey)
	list, _ := overrides.([]any)
	for _, v := range list {
		if s, isString := v.(string); isString {
			r.overrides[manifest.DistName(s)] = true
		}
	}
	return r
}

// SetRuntimePins writes pins as the runtime pins of the manifest in dir, as
// SetAstroBuild does, and group as its astro-runtime group, and changes
// nothing else: for a start that cannot reach Astronomer's index, which leaves
// the Airflow build as it is, but can still apply what needs no index. nil
// keeps the pins there, less any the project now gives a version they do not
// meet: after `uv add --frozen 'pandas>=3'`, pandas still goes, or uv has
// nothing to lock. nil group keeps the group, less what the project now lists.
func SetRuntimePins(dir string, wrap func(run func() error) error, pins *RuntimePins, group *RuntimeGroup) (refused Refused, err error) {
	err = EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		lines, _ := tomledit.GetLines(ed, constraintsKey)
		head, current, tail, _, _ := manifest.RuntimePinsIn(lines)
		var editErr error
		if refused.Pins, editErr = writeRuntimePins(ed, lines, head, current, tail, pins); editErr != nil || refused.Pins {
			// Refused pins leave no group either (SetAstroBuild).
			return editErr
		}
		refused.Group, editErr = setRuntimeGroup(ed, group)
		return editErr
	})
	return refused, err
}

// writeRuntimePins writes constraint-dependencies as head, the runtime pins
// and tail, the pins being pins or, when nil, the current ones kept; lines is
// the array as it stands. refused reports pins left out because the array
// cannot carry the comments that mark them (tomledit.ErrCommentsUnsupported).
func writeRuntimePins(ed tomledit.Editor, lines, head, current, tail []any, pins *RuntimePins) (refused bool, err error) {
	own := ownRequirements(ed, slices.Concat(head, tail))
	var managed []any
	if pins == nil {
		managed = keptRuntimePins(current, own)
	} else {
		managed = pins.lines(own)
	}
	out := slices.Concat(head, managed, tail)
	if reflect.DeepEqual(out, lines) {
		return false, nil
	}
	err = setConstraints(ed, out)
	if errors.Is(err, tomledit.ErrCommentsUnsupported) && len(managed) > 0 {
		return true, setConstraints(ed, slices.Concat(head, tail))
	}
	return false, err
}

// ErrNoRuntimeBuild reports a project whose image is no Astro Runtime build
// the catalog has: a declared Dockerfile on another image, or an Airflow pin
// no published build carries. Such a project has no runtime pins.
var ErrNoRuntimeBuild = errors.New("this project builds on no Astro Runtime the catalog lists")

// errRuntimeUnknown reports a project whose build, or whose image's Python,
// cannot be told here: no catalog, or a Dockerfile whose FROM is built from a
// build argument or pinned by digest. Its runtime pins are left as they are.
var errRuntimeUnknown = errors.New("which Astro Runtime this project builds on cannot be told")

// RuntimeBuildFor is the Astro Runtime build a project's image is built from,
// and the Python that image runs: the build its runtime pins are of. It is
// named in one place, by the first of these that applies:
//
//   - a declared Dockerfile's FROM: its exact build, or for a floating tag
//     ("3.3", "12") the newest published build the tag serves; the Python
//     its tag names, or else the build's own;
//   - [tool.astro] runtime;
//   - the Airflow pin: the newest published build carrying a release the pin
//     covers, an exact patch when the pin names one. That is the release
//     runtimeversions.LookupAstroBuild holds Airflow to, so the runtime pins
//     and the Airflow pin are of one release.
//
// For the last two the Python is the one the project's standalone venv and
// its generated image run (runtimeversions.ProjectPython, which the venv's
// StandalonePython and the image's RuntimeImageForPython both follow), so the
// pins hold the Python the project runs: for a patch pin, whose build is not
// the series' newest, that can be a Python the build is not the default of,
// and the caller reads the build's freeze for it. Where ProjectPython has no
// answer (an Airflow 2 pin, no requires-python), it is the build's own
// (BuildPython). python is "" when the build is known and its Python is not:
// an Airflow 3 build without a catalog.
//
// ErrNoRuntimeBuild means the project has no build; any other error means it
// cannot be told, as without a catalog (c nil) and no build named. One
// answer for every caller that asks which image a project builds on: the
// runtime pins, and the Airflow lookup a declared Dockerfile's build names.
func RuntimeBuildFor(dir string, m *manifest.Manifest, c *runtimeversions.Catalog) (build, python string, err error) {
	if path := dockerfilePath(dir, m); path != "" {
		return dockerfileRuntimeBuild(airflowrt.ReadDeclaredBase(path), c)
	}
	airflow := m.Airflow()
	build = strings.TrimSpace(airflow.Runtime)
	if build == "" {
		if c == nil {
			return "", "", errRuntimeUnknown
		}
		var ok bool
		if build, ok = c.NewestPublishedRuntimeFor(airflow.Pin); !ok || airflow.Pin == "" {
			return "", "", ErrNoRuntimeBuild
		}
	}
	if c != nil {
		python, _, err := runtimeversions.ProjectPython(airflow.Pin, airflow.Runtime, m.Project.RequiresPython, func() *runtimeversions.Catalog { return c })
		if err == nil && python != "" {
			return build, python, nil
		}
	}
	return build, BuildPython(build, m.Project.RequiresPython, c), nil
}

// BuildPython is the Python a generated image of build runs for a project's
// requires-python (runtimeversions.Runtime.ImagePython): the build's default
// when requires-python admits it, else the newest it ships that it admits. An
// Airflow 2 build lists no Python, and runs its own. "" when it cannot be
// told, or the build ships none requires-python admits, which refuses an
// image of it.
func BuildPython(build, requiresPython string, c *runtimeversions.Catalog) string {
	if c != nil {
		if r, ok := c.Runtime(build); ok {
			if python, decided := r.ImagePython(requiresPython); decided {
				return python
			}
		}
	}
	return imagePython(build, "", c)
}

// dockerfileRuntimeBuild is RuntimeBuildFor for a declared Dockerfile's base.
func dockerfileRuntimeBuild(base airflowrt.DeclaredBase, c *runtimeversions.Catalog) (build, python string, err error) {
	if !base.Known || base.Tag == "" {
		return "", "", errRuntimeUnknown
	}
	version := base.RuntimeVersion()
	if version == "" {
		return "", "", ErrNoRuntimeBuild
	}
	tag, ok := manifest.ParseRuntimeTag(version)
	if !ok {
		return "", "", errRuntimeUnknown
	}
	build = version
	if !tag.Build {
		if c == nil {
			return "", "", errRuntimeUnknown
		}
		if build, ok = c.NewestPublishedRuntimeTagged(version); !ok {
			return "", "", ErrNoRuntimeBuild
		}
	}
	_, tagPython := airflowrt.ParseRuntimeTagPython(base.Tag)
	return build, imagePython(build, tagPython, c), nil
}

// imagePython is the Python the image of build runs: tagPython when its tag
// named one, else the build's own (runtimeversions.ImagePythonOf), which is ""
// for an Airflow 3 build without a catalog.
func imagePython(build, tagPython string, c *runtimeversions.Catalog) string {
	if tagPython != "" {
		return tagPython
	}
	return runtimeversions.ImagePythonOf(c, build)
}
