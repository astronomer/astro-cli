package scaffold

import (
	"errors"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// RuntimePins are the versions the image of one Astro Runtime build ships, as
// SetAstroBuild writes them into [tool.uv] constraint-dependencies: under the
// comment manifest.RuntimePinsComments heads them with, one name==version a
// line, sorted by name.
//
// They hold every package the project's lockfile shares with the image to the
// image's version, so standalone mode, `uv run` and the lockfile a build
// installs from all resolve what a deployment runs. A constraint adds no
// package, so the image's packages the project does not use stay out of it.
type RuntimePins struct {
	// Build and Python are the image's: "3.3-9" and "3.12".
	Build, Python string
	// Pins are the image's distributions as runtimeversions.LookupFreeze
	// reads them. None means the build has no published list, and then any
	// runtime pins there are taken out.
	Pins []string
}

// lines are the pins as SetAstroBuild writes them, the comments first, less
// the ones it never writes: a package the project lists or constrains itself
// (own), which it chooses the version of; Airflow, the Task SDK and core,
// which b's pins hold; and any other of Astronomer's own builds (an +astro
// local version), which come from an index the project reads for Airflow
// alone. None at all when nothing is left.
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
	out := make([]any, 0, len(pins)+2)
	for _, c := range manifest.RuntimePinsComments(p.Build, p.Python) {
		out = append(out, tomledit.Comment(c))
	}
	return append(out, asAny(pins)...)
}

// keptRuntimePins are the runtime pins already there, as manifest.RuntimePinsIn
// split them off, kept when the new ones are not known, less any package the
// project now lists or constrains itself (own): a `uv add` of one of them,
// which could only lock beside the pin, takes the version over at the next
// write, online or not.
func keptRuntimePins(current []any, own map[string]bool) []any {
	var out []any
	values := 0
	for _, v := range current {
		if s, isString := v.(string); isString {
			if own[manifest.DistName(s)] {
				continue
			}
			values++
		}
		out = append(out, v)
	}
	if values == 0 {
		return nil
	}
	return out
}

// ownPackages are the distributions the project chooses the version of: the
// ones [project] dependencies lists, and the ones constraints, the
// constraint-dependencies kept ahead of the runtime pins, name.
func ownPackages(before *manifest.Manifest, constraints []any) map[string]bool {
	own := map[string]bool{}
	for _, dep := range before.Project.Dependencies {
		own[manifest.DistName(dep)] = true
	}
	for _, v := range constraints {
		if s, isString := v.(string); isString {
			own[manifest.DistName(s)] = true
		}
	}
	return own
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
//   - the Airflow pin: the newest published build of an Airflow 3 pin's
//     series, which is what the series tag serves, or of an Airflow 2 pin, as
//     the generated image picks it.
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
		pin := airflow.Pin
		if airflow.Major() == "3" {
			pin = airflow.Series()
		}
		var ok bool
		if build, ok = c.NewestPublishedRuntimeFor(pin); !ok || pin == "" {
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
