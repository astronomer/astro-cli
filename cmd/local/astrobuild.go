package local

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// refreshAstroBuild brings the [tool.uv] settings that hold a project to
// Astronomer's build of Airflow up to date with its Airflow pin and runtime
// build, before a standalone start syncs the environment from them
// (scaffold.SetAstroBuild). `astro init` writes them; this keeps them right
// after the pin moves, a new build ships, or a project predates them. Docker
// mode builds from the runtime image and never reads them.
//
// Never fails a start. A manifest that does not load is plan.Build's to report.
// An index that cannot be read leaves the file as it is, since that says
// nothing about whether a build exists, unless the pin has moved off the
// build's release (stalePin). An index that has none takes the settings out,
// and Airflow then comes from PyPI.
func (c *cli) refreshAstroBuild(ctx context.Context, r cliout.Renderer, wd string) {
	proj, err := project.Discover(wd)
	if err != nil {
		return
	}
	res := c.writeAstroBuild(ctx, proj.Dir)
	switch {
	case errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild):
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"Airflow %s comes from PyPI: Astronomer's index has no build of it, so the environment may differ from a deployment's", res.pin)})
	case res.lookupErr != nil && res.airflowChanged:
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"Airflow %s comes from PyPI for now: the Airflow pin moved, and Astronomer's index could not be read for the new build: %v", res.pin, res.lookupErr)})
	case res.lookupErr != nil:
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"could not check %s against Astronomer's newest build of Airflow, so it is used as it stands: %v", manifest.Marker, res.lookupErr)})
	}
	if res.pinsErr != nil {
		emitWarning(r, event{Event: "warning", Text: res.pinsErr.Error()})
	}
	if res.refused.Pins {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"%s holds no package to the versions the runtime image ships: its [tool.uv] is an inline table, which cannot hold the comments that mark them. Write it as a [tool.uv] table to have them", manifest.Marker)})
	}
	if res.groupErr != nil {
		emitWarning(r, event{Event: "warning", Text: res.groupErr.Error()})
	}
	if res.refused.Group {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"%s lists none of the packages the runtime image installs: its [dependency-groups] is an inline table, which cannot hold the comments that head them. Write it as a [dependency-groups] table to have them", manifest.Marker)})
	}
	if res.writeErr != nil {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf("could not update %s for Astronomer's build of Airflow: %v", manifest.Marker, res.writeErr)})
	}
	switch {
	case res.airflowChanged && res.build.Airflow != "":
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] now installs Astronomer's Airflow %s\n", manifest.Marker, res.build.Airflow)
	case res.airflowChanged:
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer points Airflow at Astronomer's index\n", manifest.Marker)
	}
	if res.groupChanged {
		if n := len(res.groupAfter); n > 0 {
			fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s now installs the %d packages Astro Runtime %s installs\n",
				manifest.Marker, manifest.RuntimeGroupName, n, res.groupBuild)
		} else {
			fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s is gone: there is no runtime image to match\n", manifest.Marker, manifest.RuntimeGroupName)
		}
	}
	if res.pinsChanged {
		if res.after.RuntimeBuild != "" {
			fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] now holds %d packages to the versions Astro Runtime %s (Python %s) ships\n",
				manifest.Marker, len(res.after.RuntimePins), res.after.RuntimeBuild, res.after.RuntimePython)
		} else {
			fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer holds packages to an Astro Runtime's versions\n", manifest.Marker)
		}
	}
}

// stalePin reports a manifest whose pin to an Astronomer build of Airflow is of
// a release its requirement no longer covers: the pin moved while the index
// could not be read. Such a build pin makes the project unsatisfiable, so it
// is taken out without a lookup, and Airflow comes from PyPI until one works.
func stalePin(m *manifest.Manifest) bool {
	pin := m.Airflow().Pin
	for _, c := range m.UV.ConstraintDependencies {
		if !manifest.AstroPin(c) || manifest.DistName(c) != runtimeversions.DistAirflow {
			continue
		}
		_, version, _ := strings.Cut(c, "==")
		if !pinCovers(pin, astroRelease(version)) {
			return true
		}
	}
	return false
}

// pinCovers reports whether an Airflow pin ("3.3", "3.3.2") covers a release.
func pinCovers(pin, release string) bool {
	return release == pin || strings.HasPrefix(release, pin+".")
}

// astroBuildWrite is what writeAstroBuild did, for its caller to report.
type astroBuildWrite struct {
	pin       string
	build     runtimeversions.AstroBuild
	lookupErr error
	writeErr  error
	// airflowChanged and pinsChanged say which part of [tool.uv] the write
	// changed: the settings that hold Airflow to the build, or the runtime
	// pins. after is [tool.uv] as the write left it.
	airflowChanged, pinsChanged bool
	after                       manifest.UV
	// pinsErr says what happened to the runtime pins, when it is worth a
	// warning, and refused which of the pins and the runtime group could not
	// be written at all.
	pinsErr error
	refused scaffold.Refused
	// groupErr says why the runtime group was not brought up to date, when it
	// is worth a warning. groupChanged says the write changed it, groupAfter
	// is it as the write left it, and groupBuild the build it was written of.
	groupErr     error
	groupChanged bool
	groupAfter   []string
	groupBuild   string
}

// runtimeImage is the build and Python scaffold.RuntimeBuildFor names for a
// project, or the error it gave.
type runtimeImage struct {
	build, python string
	err           error
}

// writeAstroBuild looks up the build for the manifest in dir and writes it,
// or takes the settings out when the index has none, with the runtime pins of
// the image the project builds on (runtimePins). When the lookup could not be
// made, the build stays as it is, and only the runtime pins of packages the
// project now names itself come out (scaffold.PruneRuntimePins), which needs
// no lookup. It writes nothing when there is no lookup to make.
//
// A declared Dockerfile's FROM names the image, so the lookup holds Airflow
// to the release that build carries, and both pins are of one image.
func (c *cli) writeAstroBuild(ctx context.Context, dir string) astroBuildWrite {
	if c.d.AstroBuild == nil {
		return astroBuildWrite{}
	}
	path := filepath.Join(dir, manifest.Marker)
	m, err := manifest.Load(path)
	if err != nil {
		return astroBuildWrite{}
	}
	airflow := m.Airflow()
	res := astroBuildWrite{pin: airflow.Pin}
	image := c.runtimeImage(ctx, dir, m)
	runtime := airflow.Runtime
	if m.Astro.Dockerfile != "" && image.err == nil {
		runtime = image.build
	}
	res.build, res.lookupErr = c.d.AstroBuild(ctx, airflow.Pin, runtime)
	if res.lookupErr != nil && !errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild) && !stalePin(m) {
		_, res.writeErr = watchManifest(dir, func(wrap func(run func() error) error) error {
			return scaffold.PruneRuntimePins(dir, wrap)
		})
		res.compare(m, path)
		return res
	}
	pins := c.runtimePins(ctx, m, image, &res)
	group := c.runtimeGroup(ctx, m, image, &res)
	if group != nil {
		res.groupBuild = group.Build
	}
	switch {
	case pins != nil:
		res.build = imageAirflowBuild(res.build, pins.Pins)
	case m.UV.RuntimeBuild != "":
		// The pins stay, so the Airflow build they were written beside does
		// too: it is the image's, which the index lookup does not know.
		res.build = imageAirflowBuild(res.build, m.UV.ConstraintDependencies)
	}
	_, res.writeErr = watchManifest(dir, func(wrap func(run func() error) error) error {
		var err error
		res.refused, err = scaffold.SetAstroBuild(dir, wrap, res.build, pins, group)
		return err
	})
	res.compare(m, path)
	return res
}

// compare reads the manifest at path as the write left it, and says which
// part of [tool.uv] changed from before. The write is judged on its parse
// already, but EditManifest keeps it to itself, so this is the one load.
func (res *astroBuildWrite) compare(before *manifest.Manifest, path string) {
	after, err := manifest.Load(path)
	if err != nil {
		return
	}
	res.after = after.UV
	res.airflowChanged = !reflect.DeepEqual(airflowSettings(before), airflowSettings(after))
	res.pinsChanged = before.UV.RuntimeBuild != after.UV.RuntimeBuild || before.UV.RuntimePython != after.UV.RuntimePython ||
		!slices.Equal(before.UV.RuntimePins, after.UV.RuntimePins)
	res.groupAfter = after.RuntimeGroup
	res.groupChanged = !slices.Equal(before.RuntimeGroup, after.RuntimeGroup)
}

// runtimeImage resolves the image the project in dir builds on: for its
// runtime pins, and for a declared Dockerfile, the build the Airflow lookup
// is asked about. A Dockerfile's exact build needs no catalog.
func (c *cli) runtimeImage(ctx context.Context, dir string, m *manifest.Manifest) runtimeImage {
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build, python, err := scaffold.RuntimeBuildFor(dir, m, catalog)
	return runtimeImage{build: build, python: python, err: err}
}

// runtimePins are the runtime pins to write for the manifest m: the package
// list of image's build. None, which takes the pins there out, whenever they
// would no longer fit:
//
//   - no build carries the Airflow pin, or the Airflow pin's own build was
//     taken out (moved while the index could not be read);
//   - the project builds on no runtime, or its runtime publishes no list;
//   - under a cutoff, the project's own [tool.uv] exclude-newer or
//     UV_EXCLUDE_NEWER: a list is of the present, and pins newer than the
//     cutoff would leave uv nothing to lock;
//   - the list's Airflow is another release than the Airflow pin's;
//   - the list of a build other than the pins' could not be read.
//
// nil, which leaves them as they are less the project's own packages, only
// for what passes: the list of the pins' own build and Python could not be
// read, or the build cannot be told for want of a catalog. res.pinsErr says
// what happened when it is worth a warning.
func (c *cli) runtimePins(ctx context.Context, m *manifest.Manifest, image runtimeImage, res *astroBuildWrite) *scaffold.RuntimePins {
	none := &scaffold.RuntimePins{}
	switch {
	case c.d.RuntimePins == nil:
		return nil
	case m.UV.ExcludeNewer != "", res.build.Airflow == "", errors.Is(image.err, scaffold.ErrNoRuntimeBuild):
		return none
	case image.err != nil, image.python == "":
		// The build cannot be told, for want of a catalog. The pins there stay
		// while the Airflow they were written beside is one the pin still
		// covers; a pin that moved off it leaves them another Airflow's.
		recorded := releaseIn(m.UV.ConstraintDependencies, runtimeversions.DistAirflow)
		if m.UV.RuntimeBuild != "" && recorded != "" && !pinCovers(m.Airflow().Pin, recorded) {
			res.pinsErr = fmt.Errorf("%s holds no runtime pins: those of Astro Runtime %s are of Airflow %s, which the Airflow requirement no longer covers, and which build carries the new one cannot be told without the runtime catalog",
				manifest.Marker, m.UV.RuntimeBuild, recorded)
			return none
		}
		return nil
	}
	freeze, err := c.d.RuntimePins(ctx, image.build, image.python)
	switch {
	case errors.Is(err, runtimecatalog.ErrFreezeNotPublished), errors.Is(err, runtimecatalog.ErrPinnedResolution):
		return none
	case err != nil && m.UV.RuntimeBuild == image.build && m.UV.RuntimePython == image.python:
		res.pinsErr = fmt.Errorf("%s keeps its runtime pins: could not read what Astro Runtime %s ships: %w", manifest.Marker, image.build, err)
		return nil
	case err != nil:
		res.pinsErr = fmt.Errorf("%s holds no runtime pins until what Astro Runtime %s ships can be read: %w", manifest.Marker, image.build, err)
		return none
	}
	// The Airflow pin and the runtime pins have to be of one release, or the
	// lock mixes one Airflow with another's dependencies. A Dockerfile FROM a
	// build the requirement excludes is the case that reaches here.
	if carried, pinned := releaseIn(freeze, runtimeversions.DistAirflow), astroRelease(res.build.Airflow); carried != "" && carried != pinned {
		res.pinsErr = fmt.Errorf("%s holds no runtime pins: Astro Runtime %s carries Airflow %s, and the Airflow requirement holds it to %s; make the two agree",
			manifest.Marker, image.build, carried, pinned)
		return none
	}
	return &scaffold.RuntimePins{Build: image.build, Python: image.python, Pins: freeze}
}

// runtimeGroup is the astro-runtime dependency group to write for the
// manifest m: the top-level packages of the image's build. None, which takes
// the group out, when there is no image to match: no build carries the
// Airflow pin, the project builds on no runtime, or the runtime publishes no
// freeze to know them by.
//
// nil, which leaves it as it is less the packages the project lists, when
// they cannot be told: no catalog, a cutoff (the group is names alone, which
// any cutoff resolves, so one written before stays), or a list that could not
// be read or worked out. res.groupErr says so when it is worth a warning.
func (c *cli) runtimeGroup(ctx context.Context, m *manifest.Manifest, image runtimeImage, res *astroBuildWrite) *scaffold.RuntimeGroup {
	switch {
	case c.d.RuntimeRoots == nil, m.UV.ExcludeNewer != "":
		return nil
	case res.build.Airflow == "", errors.Is(image.err, scaffold.ErrNoRuntimeBuild):
		return &scaffold.RuntimeGroup{}
	case image.err != nil, image.python == "":
		return nil
	}
	roots, err := c.d.RuntimeRoots(ctx, image.build, image.python)
	switch {
	case errors.Is(err, runtimecatalog.ErrFreezeNotPublished):
		return &scaffold.RuntimeGroup{}
	case errors.Is(err, runtimecatalog.ErrPinnedResolution):
		return nil
	case err != nil:
		res.groupErr = fmt.Errorf("%s keeps its %s dependency group as it is: %w", manifest.Marker, manifest.RuntimeGroupName, err)
		return nil
	}
	return &scaffold.RuntimeGroup{Build: image.build, Names: roots}
}

// releaseIn is the release, without its +astro build, that freeze pins dist
// to, or "".
func releaseIn(freeze []string, dist string) string {
	for _, pin := range freeze {
		if name, version, ok := strings.Cut(pin, "=="); ok && manifest.DistName(name) == dist {
			return astroRelease(version)
		}
	}
	return ""
}

// astroRelease is a version without its +astro build: 3.3.2 for
// 3.3.2+astro.2.
func astroRelease(version string) string {
	release, _, _ := strings.Cut(strings.TrimSpace(version), "+")
	return release
}

// imageAirflowBuild is b with the Airflow and Task SDK builds the image's
// package list names, where they are builds of the same releases. The index
// lookup takes the newest +astro.N of a release, and Astronomer can publish a
// newer one than a runtime image carries; the image's own list says which.
func imageAirflowBuild(b runtimeversions.AstroBuild, freeze []string) runtimeversions.AstroBuild {
	for _, pin := range freeze {
		name, version, _ := strings.Cut(pin, "==")
		switch manifest.DistName(name) {
		case runtimeversions.DistAirflow:
			if sameRelease(b.Airflow, version) {
				b.Airflow = version
			}
		case runtimeversions.DistTaskSDK:
			if sameRelease(b.TaskSDK, version) {
				b.TaskSDK = version
			}
		}
	}
	return b
}

// sameRelease reports two +astro builds of one release: 3.3.2+astro.1 and
// 3.3.2+astro.2.
func sameRelease(a, b string) bool {
	ra, _, oka := strings.Cut(a, "+astro.")
	rb, _, okb := strings.Cut(b, "+astro.")
	return oka && okb && ra == rb
}

// airflowSettings are the parts of a manifest SetAstroBuild changes to hold
// Airflow to a build, apart from the runtime pins: what "now installs
// Astronomer's Airflow" reports a change of.
func airflowSettings(m *manifest.Manifest) any {
	var pins []string
	for _, c := range m.UV.ConstraintDependencies {
		if manifest.AstroPin(c) {
			pins = append(pins, c)
		}
	}
	return struct {
		Pins, Deps, Environments []string
		Sources                  map[string]string
	}{pins, m.Project.Dependencies, m.UV.Environments, m.UV.Sources}
}
