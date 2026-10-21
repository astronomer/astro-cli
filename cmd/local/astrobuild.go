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
// mode does not refresh them, but builds on the runtime build they record
// (imagebuild.ManifestBuild.LockedRuntime), and installs the lock they shape,
// so it follows this refresh on its next build.
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
	c.reportAstroBuild(r, c.writeAstroBuild(ctx, proj.Dir))
}

// reportAstroBuild says what writeAstroBuild did: warnings for what it could
// not do, and a line for each part of [tool.uv] it changed. A standalone start
// reports every refresh; astro init, the refresh of a project that already was
// one.
func (c *cli) reportAstroBuild(r cliout.Renderer, res astroBuildWrite) {
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
				manifest.Marker, manifest.RuntimeGroupName, n, res.groupAfterBuild)
			c.reportGroupLeftOut(res)
		} else {
			fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s is gone: there is no runtime image to match\n", manifest.Marker, manifest.RuntimeGroupName)
		}
	}
	if res.pinsChanged {
		if res.beforeBuild != "" && res.after.RuntimeBuild != "" && res.beforeBuild != res.after.RuntimeBuild {
			// Docker mode, a package and a deploy build on the build the pins
			// name, so they follow on their next build.
			fmt.Fprintf(c.d.Stderr, "%s: runtime pins moved from Astro Runtime %s to %s; Docker mode and astro package astro build on %s from now on, and astro deploy does once the Deployment offers it\n",
				manifest.Marker, res.beforeBuild, res.after.RuntimeBuild, res.after.RuntimeBuild)
		}
		if res.after.RuntimeBuild != "" {
			fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] now holds %d packages to the versions Astro Runtime %s (Python %s) ships\n",
				manifest.Marker, len(res.after.RuntimePins), res.after.RuntimeBuild, res.after.RuntimePython)
		} else {
			fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer holds packages to an Astro Runtime's versions\n", manifest.Marker)
		}
	}
}

// reportGroupLeftOut says which of the image's packages the runtime group left
// out, and why: it, or a package it pulls in, would have to build from source
// on a platform a project runs on. Docker mode still has them, and uv add
// installs one where its build works.
func (c *cli) reportGroupLeftOut(res astroBuildWrite) {
	for _, l := range res.groupLeftOut {
		fmt.Fprintf(c.d.Stderr, "%s: %s leaves out %s, which the runtime image has: %s on Python %s. Docker mode still has it; uv add %s installs it anyway\n",
			manifest.Marker, manifest.RuntimeGroupName, l.Name, l.Why, res.after.RuntimePython, l.Name)
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
		if !runtimeversions.PinCovers(pin, astroRelease(version)) {
			return true
		}
	}
	return false
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
	// beforeBuild is the runtime build the pins were taken from before the
	// write, "" for none.
	beforeBuild string
	// pinsErr says what happened to the runtime pins, when it is worth a
	// warning, and refused which of the pins and the runtime group could not
	// be written at all.
	pinsErr error
	refused scaffold.Refused
	// pinsCutoff says the runtime pins came out for a cutoff, which keeps the
	// runtime group (runtimeGroup).
	pinsCutoff bool
	// groupErr says why the runtime group was not brought up to date, when it
	// is worth a warning. groupChanged says the write changed it, groupAfter
	// and groupAfterBuild are it as the write left it, and groupLeftOut the
	// image's packages it leaves out for having no wheel here.
	groupErr        error
	groupChanged    bool
	groupAfter      []string
	groupAfterBuild string
	groupLeftOut    []runtimecatalog.LeftOut
}

// runtimeImage is the build and Python scaffold.RuntimeBuildFor names for a
// project, or the error it gave.
type runtimeImage struct {
	build, python string
	err           error
	// buildPython is the Python an image of build itself runs for the
	// project (scaffold.BuildPython), when that is not python: the pins fall
	// back to its freeze when build publishes none for python. "" for a
	// declared Dockerfile, whose image runs python.
	buildPython string
}

// writeAstroBuild looks up the build for the manifest in dir and writes it,
// or takes the settings out when the index has none, with the runtime pins of
// the image the project builds on (runtimePins). It writes nothing when there
// is no lookup to make.
//
// When the lookup could not be made and the pin has not moved, the Airflow
// build stays as the manifest has it, and the runtime pins are still brought
// up to date (scaffold.SetRuntimePins), which needs no index: a new cutoff, a
// new FROM or requires-python, and a bound the project now gives a package the image ships.
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
		res.build = recordedAstroBuild(m)
		pins := c.runtimePins(ctx, dir, m, image, &res)
		group := c.runtimeGroup(ctx, pins, &res)
		changed, writeErr := watchManifest(dir, func(wrap func(run func() error) error) error {
			var err error
			res.refused, err = scaffold.SetRuntimePins(dir, wrap, pins, group)
			return err
		})
		res.writeErr = writeErr
		res.compare(m, path)
		// Only the runtime pins and the runtime group were written, so
		// whatever else changed is the pins': the comments around them, when
		// their lines are the same.
		res.airflowChanged, res.pinsChanged = false, res.pinsChanged || (changed && !res.groupChanged)
		return res
	}
	pins := c.runtimePins(ctx, dir, m, image, &res)
	group := c.runtimeGroup(ctx, pins, &res)
	switch {
	case pins != nil:
		res.build = imageAirflowBuild(res.build, pins.Pins)
	case m.UV.RuntimeBuild != "":
		// The pins stay, so the Airflow build they were written beside does
		// too: it is the image's, which the index lookup does not know.
		res.build = imageAirflowBuild(res.build, m.UV.ConstraintDependencies)
	}
	changed, writeErr := watchManifest(dir, func(wrap func(run func() error) error) error {
		var err error
		res.refused, err = scaffold.SetAstroBuild(dir, wrap, res.build, pins, group)
		return err
	})
	res.writeErr = writeErr
	res.compare(m, path)
	// A write that changed neither the Airflow pins, the runtime pins nor the
	// runtime group repaired another of the settings that hold Airflow to the
	// build: the index, its sources, or exclude-newer-package.
	res.airflowChanged = res.airflowChanged || (changed && !res.pinsChanged && !res.groupChanged)
	return res
}

// recordedAstroBuild is the Astronomer build of Airflow the manifest holds
// the project to now, read from its pins (manifest.AstroPin): what a start
// that cannot reach the index keeps.
func recordedAstroBuild(m *manifest.Manifest) runtimeversions.AstroBuild {
	var b runtimeversions.AstroBuild
	for _, c := range m.UV.ConstraintDependencies {
		if !manifest.AstroPin(c) {
			continue
		}
		_, version, _ := strings.Cut(c, "==")
		switch manifest.DistName(c) {
		case runtimeversions.DistAirflow:
			b.Airflow = strings.TrimSpace(version)
		case runtimeversions.DistTaskSDK:
			b.TaskSDK = strings.TrimSpace(version)
		}
	}
	if b.Airflow != "" {
		b.Index = runtimeversions.PublicIndexURL()
	}
	return b
}

// compare reads the manifest at path as the write left it, and says which
// part of [tool.uv] changed from before: the Airflow pins, and the runtime
// pins, whose build, Python and count the notice names, which is why this
// reads the manifest back rather than going by watchManifest's changed alone.
func (res *astroBuildWrite) compare(before *manifest.Manifest, path string) {
	after, err := manifest.Load(path)
	if err != nil {
		return
	}
	res.after = after.UV
	res.beforeBuild = before.UV.RuntimeBuild
	res.airflowChanged = !reflect.DeepEqual(airflowSettings(before), airflowSettings(after))
	res.pinsChanged = before.UV.RuntimeBuild != after.UV.RuntimeBuild || before.UV.RuntimePython != after.UV.RuntimePython ||
		!slices.Equal(before.UV.RuntimePins, after.UV.RuntimePins)
	res.groupAfter, res.groupAfterBuild = after.RuntimeGroup, after.RuntimeGroupBuild
	res.groupChanged = before.RuntimeGroupBuild != after.RuntimeGroupBuild || !slices.Equal(before.RuntimeGroup, after.RuntimeGroup)
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
	image := runtimeImage{build: build, python: python, err: err}
	if err == nil && m.Astro.Dockerfile == "" {
		if own := scaffold.BuildPython(build, m.Project.RequiresPython, catalog); own != python {
			image.buildPython = own
		}
	}
	return image
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
func (c *cli) runtimePins(ctx context.Context, dir string, m *manifest.Manifest, image runtimeImage, res *astroBuildWrite) *scaffold.RuntimePins {
	none := &scaffold.RuntimePins{}
	switch {
	case c.d.RuntimePins == nil:
		return nil
	case m.UV.ExcludeNewer != "":
		res.pinsCutoff = true
		return none
	case res.build.Airflow == "", errors.Is(image.err, scaffold.ErrNoRuntimeBuild):
		return none
	case image.err != nil, image.python == "":
		// The build cannot be told, for want of a catalog. The pins there stay
		// while the Airflow they were written beside is one the pin still
		// covers; a pin that moved off it leaves them another Airflow's.
		recorded := releaseIn(m.UV.ConstraintDependencies, runtimeversions.DistAirflow)
		if m.UV.RuntimeBuild != "" && recorded != "" && !runtimeversions.PinCovers(m.Airflow().Pin, recorded) {
			res.pinsErr = fmt.Errorf("%s holds no runtime pins: those of Astro Runtime %s are of Airflow %s, which the Airflow requirement no longer covers, and which build carries the new one cannot be told without the runtime catalog",
				manifest.Marker, m.UV.RuntimeBuild, recorded)
			return none
		}
		return nil
	}
	freeze, err := c.d.RuntimePins(ctx, image.build, image.python)
	if errors.Is(err, runtimecatalog.ErrFreezeNotPublished) && image.buildPython != "" {
		// The build publishes no list for the Python the project runs (a
		// patch pin's build, older than the series' newest): its own Python's
		// list is the nearest there is, said so.
		res.pinsErr = fmt.Errorf("Astro Runtime %s publishes no package list for Python %s, the one this project runs, so the runtime pins are of its Python %s",
			image.build, image.python, image.buildPython)
		image.python = image.buildPython
		freeze, err = c.d.RuntimePins(ctx, image.build, image.python)
	}
	switch {
	case errors.Is(err, runtimecatalog.ErrPinnedResolution):
		res.pinsCutoff = true
		return none
	case errors.Is(err, runtimecatalog.ErrFreezeNotPublished):
		return none
	case err != nil && m.UV.RuntimeBuild == image.build && m.UV.RuntimePython == image.python:
		res.pinsErr = fmt.Errorf("%s keeps its runtime pins: could not read what Astro Runtime %s ships: %w", manifest.Marker, image.build, err)
		return nil
	case err != nil:
		// The build moved on and its list cannot be read yet: the pins there
		// still describe the build the images use, so they stay while the
		// requirement covers their Airflow.
		return keepRecordedPins(m, res,
			fmt.Sprintf("could not read what Astro Runtime %s ships: %v", image.build, err),
			fmt.Errorf("%s holds no runtime pins until what Astro Runtime %s ships can be read: %w", manifest.Marker, image.build, err))
	}
	// The Airflow pin and the runtime pins have to be of one release, or the
	// lock mixes one Airflow with another's dependencies.
	if carried, pinned := releaseIn(freeze, runtimeversions.DistAirflow), astroRelease(res.build.Airflow); carried != "" && carried != pinned {
		if m.Astro.Dockerfile == "" && m.Airflow().Runtime == "" {
			// Nothing the user wrote names the build: the lookup and the
			// catalog's newest build disagree on their own, so the pins
			// follow the build carrying the release the lookup chose.
			return c.pinsOfRelease(ctx, dir, m, pinned, res)
		}
		// A Dockerfile FROM, or a runtime, the requirement excludes.
		res.pinsErr = fmt.Errorf("%s holds no runtime pins: Astro Runtime %s carries Airflow %s, and the Airflow requirement holds it to %s; make the two agree",
			manifest.Marker, image.build, carried, pinned)
		return none
	}
	return &scaffold.RuntimePins{Build: image.build, Python: image.python, Pins: freeze}
}

// pinsOfRelease are the runtime pins of the newest published build carrying
// Airflow release, for a project that names no build itself. With none
// published yet, the pins there stay while the requirement covers their
// Airflow, and come out otherwise; either way the warning blames nothing the
// user wrote.
func (c *cli) pinsOfRelease(ctx context.Context, dir string, m *manifest.Manifest, release string, res *astroBuildWrite) *scaffold.RuntimePins {
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	if catalog != nil {
		if build, ok := catalog.NewestPublishedRuntimeFor(release); ok {
			named := *m
			named.Astro.Runtime = build
			if b, python, err := scaffold.RuntimeBuildFor(dir, &named, catalog); err == nil && python != "" {
				if freeze, err := c.d.RuntimePins(ctx, b, python); err == nil && releaseIn(freeze, runtimeversions.DistAirflow) == release {
					return &scaffold.RuntimePins{Build: b, Python: python, Pins: freeze}
				}
			}
		}
	}
	why := fmt.Sprintf("no published Astro Runtime image carries Airflow %s, the build Astronomer's index has for the requirement, yet", release)
	return keepRecordedPins(m, res, why, fmt.Errorf("%s holds no runtime pins: %s", manifest.Marker, why))
}

// keepRecordedPins leaves the runtime pins there as they are when the Airflow
// requirement still covers the Airflow they were written beside, and holds
// res.build to that Airflow build too, so the lock does not pair one release
// with another's dependencies. It says why with kept. Otherwise, or with no
// pins there, they come out, with dropped as the warning.
func keepRecordedPins(m *manifest.Manifest, res *astroBuildWrite, kept string, dropped error) *scaffold.RuntimePins {
	recorded := recordedAstroBuild(m)
	if m.UV.RuntimeBuild == "" || recorded.Airflow == "" || !runtimeversions.PinCovers(m.Airflow().Pin, recorded.Airflow) {
		res.pinsErr = dropped
		return &scaffold.RuntimePins{}
	}
	if res.build.Index != "" {
		recorded.Index = res.build.Index
	}
	res.build = recorded
	res.pinsErr = fmt.Errorf("%s keeps the runtime pins of Astro Runtime %s: %s", manifest.Marker, m.UV.RuntimeBuild, kept)
	return nil
}

// runtimeGroup is the astro-runtime dependency group to write beside pins,
// the runtime pins runtimePins decided on, and follows their decision: pins
// written are of one build and Python, and the group is that image's
// top-level packages, read from the same list; pins taken out take the group
// out; pins kept as they are keep it. The one exception is a cutoff, which
// takes the pins out and keeps the group: it holds names alone, which any
// cutoff resolves.
//
// A list of top-level packages that cannot be had keeps the group as it is,
// and res.groupErr says why.
func (c *cli) runtimeGroup(ctx context.Context, pins *scaffold.RuntimePins, res *astroBuildWrite) *scaffold.RuntimeGroup {
	switch {
	case c.d.RuntimeRoots == nil, pins == nil, res.pinsCutoff:
		return nil
	case pins.Build == "":
		return &scaffold.RuntimeGroup{}
	}
	roots, err := c.d.RuntimeRoots(ctx, pins.Build, pins.Python, pins.Pins)
	if err != nil {
		res.groupErr = fmt.Errorf("%s keeps its %s dependency group as it is: %w", manifest.Marker, manifest.RuntimeGroupName, err)
		return nil
	}
	res.groupLeftOut = roots.LeftOut
	return &scaffold.RuntimeGroup{Build: pins.Build, Names: roots.Names}
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
