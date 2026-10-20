package scaffold

import (
	"errors"
	"os"
	"path/filepath"
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
// package the project lists or constrains itself (own), which it chooses the
// version of; Airflow, the Task SDK and core, which b's pins hold; and any
// other of Astronomer's own builds (an +astro local version), which come from
// an index the project reads for Airflow alone. None at all when nothing is
// left.
func (p *RuntimePins) lines(own map[string]bool) []any {
	var pins []string
	for _, pin := range p.Pins {
		name := manifest.DistName(pin)
		if own[name] || slices.Contains(astroDists, name) || strings.Contains(pin, "+astro.") {
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
// split them off, kept when the new ones are not known, less any package the
// project now lists or constrains itself (own): a `uv add` of one of them,
// which could only lock beside the pin, takes the version over at the next
// write, online or not. A block written without the end comment gains it.
func keptRuntimePins(current []any, own map[string]bool) []any {
	var out []any
	values := 0
	ended := false
	for _, v := range current {
		switch v := v.(type) {
		case string:
			if own[manifest.DistName(v)] {
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

// ListedPackages are the distributions the project in dir lists to install,
// normalized (manifest.DistName): [project] dependencies,
// [project.optional-dependencies], [dependency-groups] and the [tool.uv]
// dev-dependencies older uv wrote. The manifest carries only the first, so
// the file is read here; one rule for the runtime pins, which leave these to
// the project, and for `astro local check`'s hint, which does not ask the
// project to add one of them.
func ListedPackages(dir string) (map[string]bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return nil, err
	}
	ed, err := tomledit.NewSurgical(data)
	if err != nil {
		return nil, err
	}
	return listedPackages(ed), nil
}

// listedPackages is ListedPackages for a document already open.
func listedPackages(ed tomledit.Editor) map[string]bool {
	listed := map[string]bool{}
	addAll := func(list []any) {
		for _, v := range list {
			// A dependency group can include another, as a table; that one is
			// read where it is defined.
			if s, isString := v.(string); isString {
				listed[manifest.DistName(s)] = true
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
		for _, group := range groups {
			list, _ := group.([]any)
			addAll(list)
		}
	}
	return listed
}

// ownPackages are the distributions the project chooses the version of: the
// ones it lists (listedPackages), the ones [tool.uv] override-dependencies
// names, and the ones constraints, its own lines of constraint-dependencies,
// name. A runtime pin on one of them would leave `uv add --dev 'pytest>=9'`
// nothing to lock where the image ships pytest 8.
func ownPackages(ed tomledit.Editor, constraints []any) map[string]bool {
	own := listedPackages(ed)
	overrides, _ := ed.Get(overrideDepsKey)
	list, _ := overrides.([]any)
	for _, v := range slices.Concat(list, constraints) {
		if s, isString := v.(string); isString {
			own[manifest.DistName(s)] = true
		}
	}
	return own
}

// PruneRuntimePins takes out of the runtime pins in the manifest in dir any
// package the project now names itself (ownPackages), and changes nothing
// else. It needs no lookup, so it runs when the build cannot be looked up
// too: after `uv add --frozen 'pandas>=3'`, a start with Astronomer's index
// out of reach must still let pandas go, or uv has nothing to lock.
func PruneRuntimePins(dir string, wrap func(run func() error) error) error {
	return EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		lines, _ := tomledit.GetLines(ed, constraintsKey)
		head, current, tail, _, _ := manifest.RuntimePinsIn(lines)
		if len(current) == 0 {
			return nil
		}
		out := slices.Concat(head, keptRuntimePins(current, ownPackages(ed, slices.Concat(head, tail))), tail)
		if reflect.DeepEqual(out, lines) {
			return nil
		}
		return setConstraints(ed, out)
	})
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
// For the last two the Python is the one the generated image runs
// (runtimeversions.Runtime.ImagePython): the build's default when
// requires-python admits it, else the newest it ships that it admits. An
// Airflow 2 build lists no Python, and runs its own. python is "" when the
// build is known and its Python is not: an Airflow 3 build without a catalog.
//
// ErrNoRuntimeBuild means the project has no build; any other error means it
// cannot be told, as without a catalog (c nil) and no build named. One
// answer for every caller that asks which image a project builds on: the
// runtime pins, and `astro local check`'s hint about a package the image
// ships.
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
		if r, ok := c.Runtime(build); ok {
			if python, decided := r.ImagePython(m.Project.RequiresPython); decided {
				if python == "" {
					// An image of this build is refused, which says so.
					return "", "", errRuntimeUnknown
				}
				return build, python, nil
			}
		}
	}
	return build, imagePython(build, "", c), nil
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
