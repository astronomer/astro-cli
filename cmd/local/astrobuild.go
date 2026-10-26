package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	pep440 "github.com/aquasecurity/go-pep440-version"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// refreshAstroBuild brings the [tool.uv] settings that hold a project to
// Astronomer's build of Airflow up to date with its Airflow pin and runtime
// build, before a standalone start syncs the environment from them
// (scaffold.SetAstroBuild). `astro init` writes them; this keeps them right
// after the pin moves, a new build ships, or a project predates them. Docker
// mode does not refresh them, but builds on the runtime build the seed records
// (imagebuild.ManifestBuild.LockedRuntime), and installs the lock it seeded,
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
	c.reportAstroBuild(r, c.writeAstroBuild(ctx, proj.Dir, false))
}

// reportAstroBuild says what writeAstroBuild did: warnings for what it could
// not do, and a line for each part of the manifest it changed. A standalone
// start reports every refresh; astro init, the refresh of a project that
// already was one.
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
	if res.writeErr != nil {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf("could not update %s for Astronomer's build of Airflow: %v", manifest.Marker, res.writeErr)})
	}
	switch {
	case res.updated:
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] updated\n", manifest.Marker)
	case res.airflowChanged && res.build.Airflow != "":
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] now installs Astronomer's Airflow %s\n", manifest.Marker, res.build.Airflow)
	case res.airflowChanged:
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer points Airflow at Astronomer's index\n", manifest.Marker)
	}
	c.reportSeed(r, res)
}

// reportSeed says what writeAstroBuild did with the project's astro-runtime
// group and its uv.lock: wrote the group, seeded the lock, found it apart
// from the image, or could not, and why. For a start, and for init, which
// has no to-do list of its own for it.
func (c *cli) reportSeed(r cliout.Renderer, res astroBuildWrite) {
	if res.seedWarn != nil {
		emitWarning(r, event{Event: "warning", Text: res.seedWarn.Error()})
	}
	if res.groupErr != nil {
		emitWarning(r, event{Event: "warning", Text: res.groupErr.Error()})
	}
	if res.refused {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"%s lists none of the packages the runtime image installs, and does not record which Astro Runtime uv.lock was seeded from: its [dependency-groups] is an inline table, which cannot hold the comments that head them. Write it as a [dependency-groups] table to have them", manifest.Marker)})
	}
	if res.groupChanged {
		if n := len(res.groupAfter); n > 0 {
			fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s now installs the %d packages Astro Runtime %s installs\n",
				manifest.Marker, manifest.RuntimeGroupName, n, res.groupBuild)
		} else {
			fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s now lists none of a runtime image's packages: none apply to this project\n", manifest.Marker, manifest.RuntimeGroupName)
		}
	}
	if res.groupChanged || res.seeded > 0 {
		c.reportGroupLeftOut(res)
	}
	if res.seeded > 0 && res.beforeBuild != "" && res.beforeBuild != res.seed.Build {
		// Docker mode, a package and a deploy build on the build the seed
		// records, so they follow on their next build.
		fmt.Fprintf(c.d.Stderr, "uv.lock: seeded from Astro Runtime %s now, not %s; Docker mode and astro package astro build on %s from now on, and astro deploy does once the Deployment offers it\n",
			res.seed.Build, res.beforeBuild, res.seed.Build)
	}
	switch {
	case res.seeded > 0:
		fmt.Fprintf(c.d.Stderr, "uv.lock: seeded with the versions Astro Runtime %s (Python %s) ships (%d packages)\n",
			res.seed.Build, res.seed.Python, res.seeded)
	case res.drift > 0:
		fmt.Fprintf(c.d.Stderr, "uv.lock: %d packages differ from Astro Runtime %s; run astro init to re-seed\n", res.drift, res.driftBuild)
	case res.recordChanged && res.after.RuntimeBuild == "":
		fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s no longer records an Astro Runtime uv.lock was seeded from\n", manifest.Marker, manifest.RuntimeGroupName)
	}
}

// reportGroupLeftOut says which of the image's packages the runtime group left
// out, and why: it, or a package it pulls in, would have to build from source
// on a platform a project runs on. Docker mode still has them, and uv add
// installs one where its build works.
func (c *cli) reportGroupLeftOut(res astroBuildWrite) {
	if !res.groupWritten {
		return
	}
	for _, l := range res.groupLeftOut {
		fmt.Fprintf(c.d.Stderr, "%s: %s leaves out %s, which the runtime image has: %s. Docker mode still has it; uv add %s installs it anyway\n",
			manifest.Marker, manifest.RuntimeGroupName, l.Name, l.Why, l.Name)
	}
}

// stalePin reports a manifest whose pin to an Astronomer build of Airflow is of
// a release its requirement no longer covers: the pin moved while the index
// could not be read. Such a build pin makes the project unsatisfiable, so it
// is taken out without a lookup, and Airflow comes from PyPI until one works.
func stalePin(m *manifest.Manifest) bool {
	return !manifest.AstroBuildCovered(m.Airflow().Pin, m.UV.ConstraintDependencies)
}

// astroBuildWrite is what writeAstroBuild did, for its caller to report.
type astroBuildWrite struct {
	pin       string
	build     runtimeversions.AstroBuild
	lookupErr error
	writeErr  error
	// airflowChanged, recordChanged and groupChanged say which part of the
	// manifest the write changed: the settings that hold Airflow to the
	// build, the record of the seed, or the astro-runtime group. after is
	// [tool.uv] as the write left it, and groupAfter the group.
	airflowChanged, recordChanged, groupChanged bool
	after                                       manifest.UV
	groupAfter                                  []string
	// beforeBuild is the runtime build the lock was seeded from before the
	// write, "" for none.
	beforeBuild string
	// seeded is how many of the image's versions the lock was seeded with,
	// and seed the build and Python they were; drift how many the lock holds
	// apart from driftBuild's, when it was not seeded.
	seeded     int
	seed       scaffold.Seed
	drift      int
	driftBuild string
	// seedWarn says what happened to the seed, when it is worth a warning,
	// and refused that the group, which holds its record, could not be
	// written at all.
	seedWarn error
	refused  bool
	// cutoff says nothing is seeded for a cutoff, which keeps the runtime
	// group (runtimeGroup).
	cutoff bool
	// groupWritten says the runtime group decided on was written, and
	// groupBuild is the build it is of. groupErr says why the group was not
	// brought up to date, when it is worth a warning, and groupLeftOut the
	// image's packages it leaves out for having no wheel here.
	groupWritten bool
	groupBuild   string
	groupErr     error
	groupLeftOut []runtimecatalog.LeftOut
	// updated is a write that changed the file, which could not be read back
	// to say what changed.
	updated bool
}

// runtimeImage is the build and Python scaffold.RuntimeBuildFor names for a
// project, or the error it gave.
type runtimeImage struct {
	build, python string
	err           error
	// otherPython reports a python the build is not the Python of
	// (scaffold.BuildPython): a patch pin's build, older than the series'
	// newest that decides the Python the project runs. A build with no list
	// for it is worth a warning then; one with no list at all (an Airflow 2
	// before 11.0.0) is not. False for a declared Dockerfile, whose image
	// runs python.
	otherPython bool
}

// writeAstroBuild looks up the build for the manifest in dir and writes it,
// or takes the settings out when the index has none, and seeds the project's
// uv.lock with what the image the project builds on ships (runtimePins,
// seedLock). It writes nothing when there is no lookup to make.
//
// init seeds the lock whatever it holds (seed). A start seeds one only when
// there is none; otherwise it says how many packages the lock holds apart
// from the image (lockDrift), and leaves it to `astro init` to seed again.
//
// When the lookup could not be made and the pin has not moved, the Airflow
// build stays as the manifest has it, and what needs no index still applies:
// the seed, and its record (scaffold.SetSeedRecord).
//
// A declared Dockerfile's FROM names the image, so the lookup holds Airflow
// to the release that build carries, and both are of one image.
func (c *cli) writeAstroBuild(ctx context.Context, dir string, seed bool) astroBuildWrite {
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
	// The image is resolved for the seed, and for a declared Dockerfile's
	// build, which the Airflow lookup is asked about. Under a cutoff there is
	// nothing to seed (runtimePins).
	cutoff := underCutoff(m)
	image := runtimeImage{err: errNoImage}
	if (c.d.RuntimePins != nil && !cutoff) || m.Astro.Dockerfile != "" {
		image = c.runtimeImage(ctx, dir, m)
	}
	runtime := airflow.Runtime
	if m.Astro.Dockerfile != "" && image.err == nil {
		runtime = image.build
	}
	res.build, res.lookupErr = c.d.AstroBuild(ctx, airflow.Pin, runtime)
	offline := res.lookupErr != nil && !errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild) && !stalePin(m)
	if offline {
		res.build = recordedAstroBuild(m)
	}
	pins := c.runtimePins(ctx, dir, m, image, cutoff, &res)
	group := c.runtimeGroup(ctx, pins, &res)
	if !offline {
		switch {
		case pins != nil:
			res.build = imageAirflowBuild(res.build, pins.Pins)
		case m.UV.RuntimeBuild != "":
			// The seed stays, so the Airflow build it was made beside does
			// too: it is the image's, which the index lookup does not know.
			res.build = imageAirflowBuild(res.build, m.UV.ConstraintDependencies)
		}
	}
	// Nothing to seed takes the record of an earlier seed out; a seed records
	// itself once the lock holds it.
	var record *scaffold.Seed
	if pins != nil && len(pins.Pins) == 0 {
		record = &scaffold.Seed{}
	}
	changed, writeErr := watchManifest(dir, func(wrap func(run func() error) error) error {
		var err error
		if offline {
			res.refused, err = scaffold.SetRuntimeGroup(dir, wrap, group, record)
			return err
		}
		res.refused, err = scaffold.SetAstroBuild(dir, wrap, res.build, group, record)
		return err
	})
	res.writeErr = writeErr
	res.groupWritten = group != nil && writeErr == nil && !res.refused
	if writeErr == nil && pins != nil && len(pins.Pins) > 0 {
		// A group with other packages in it than before has packages the lock
		// does not hold yet, which only a seed holds to the image's versions.
		recorded, recordErr := c.applySeed(ctx, dir, m, pins, seed || groupMoved(m, path), &res)
		changed = changed || recorded
		if recordErr != nil {
			res.writeErr = recordErr
		}
	}
	res.compare(m, path, changed, offline)
	return res
}

// groupMoved reports a runtime group the manifest at path lists other
// packages in than before did.
func groupMoved(before *manifest.Manifest, path string) bool {
	after, err := manifest.Load(path)
	return err == nil && !slices.Equal(before.RuntimeGroup, after.RuntimeGroup)
}

// applySeed seeds the lock of the project in dir with pins, and records the
// seed, when seed asks or there is no lock; otherwise it counts how far the
// lock is from pins. It reports whether it changed the manifest.
func (c *cli) applySeed(ctx context.Context, dir string, m *manifest.Manifest, pins *scaffold.RuntimePins, seed bool, res *astroBuildWrite) (bool, error) {
	if !seed && hasLock(dir) {
		if res.drift = lockDrift(dir, pins); res.drift > 0 {
			res.driftBuild = pins.Build
		}
		return false, nil
	}
	if c.d.Lock == nil {
		return false, nil
	}
	n, err := c.seedLock(ctx, dir, pins)
	switch {
	case errors.Is(err, uv.ErrNotFound):
		res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: uv is not installed", pins.Build)
		return false, nil
	case err != nil:
		res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: %w", pins.Build, err)
		return false, nil
	case n == 0:
		return false, nil
	}
	res.seeded, res.seed = n, scaffold.Seed{Build: pins.Build, Python: pins.Python}
	if m.UV.RuntimeBuild == pins.Build && m.UV.RuntimePython == pins.Python {
		return false, nil
	}
	var refused bool
	changed, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		var err error
		refused, err = scaffold.SetRuntimeGroup(dir, wrap, nil, &res.seed)
		return err
	})
	res.refused = res.refused || refused
	return changed, err
}

// runtimeGroup is the astro-runtime dependency group to write beside pins,
// what runtimePins decided the lock is seeded with, and follows their
// decision: pins of one build and Python give that image's top-level
// packages, read from the same list; none empty the group; pins kept as they
// are keep it. The one exception is a cutoff, which seeds nothing and keeps
// the group: it holds names alone, which any cutoff resolves.
//
// A list of top-level packages that cannot be had keeps the group as it is,
// and res.groupErr says why.
func (c *cli) runtimeGroup(ctx context.Context, pins *scaffold.RuntimePins, res *astroBuildWrite) *scaffold.RuntimeGroup {
	switch {
	case c.d.RuntimeRoots == nil, pins == nil, res.cutoff:
		return nil
	case pins.Build == "":
		return &scaffold.RuntimeGroup{}
	}
	// The pins' own Python and list: the group is worked out from the same
	// list the lock is seeded with.
	roots, err := c.d.RuntimeRoots(ctx, pins.Build, pins.Python, pins.Pins)
	if err != nil {
		res.groupErr = fmt.Errorf("%s keeps its %s dependency group as it is: %w", manifest.Marker, manifest.RuntimeGroupName, err)
		return nil
	}
	res.groupLeftOut, res.groupBuild = roots.LeftOut, pins.Build
	return &scaffold.RuntimeGroup{Names: roots.Names}
}

// errNoImage is the runtimeImage of a write that has nothing to resolve it
// for: under a cutoff, or with no seed to make and no Dockerfile.
var errNoImage = errors.New("the runtime image was not resolved")

// underCutoff reports a project uv resolves as of a cutoff: its own [tool.uv]
// exclude-newer, or UV_EXCLUDE_NEWER, a date or a duration alike. A runtime's
// package list is of the present, so such a project's lock is not seeded.
func underCutoff(m *manifest.Manifest) bool {
	return m.UV.ExcludeNewer != "" || strings.TrimSpace(os.Getenv("UV_EXCLUDE_NEWER")) != ""
}

// recordedAstroBuild is the Astronomer build of Airflow the manifest holds
// the project to now, read from its Airflow pins (manifest.AstroPin): what a
// start that cannot reach the index keeps.
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
// part of [tool.uv] changed from before: the Airflow pins, or the seed's
// record, whose build the notices name, which is why this reads the manifest
// back rather than going by changed, watchManifest's word that the file
// changed, alone. recordOnly is a write of the group and its record alone
// (a start that cannot reach the index), so nothing else changed.
//
// When the file cannot be read back, nothing is attributed: a write that
// changed it says only that it did (updated).
func (res *astroBuildWrite) compare(before *manifest.Manifest, path string, changed, recordOnly bool) {
	after, err := manifest.Load(path)
	if err != nil {
		res.updated = changed
		return
	}
	res.after, res.groupAfter, res.beforeBuild = after.UV, after.RuntimeGroup, before.UV.RuntimeBuild
	res.recordChanged = before.UV.RuntimeBuild != after.UV.RuntimeBuild || before.UV.RuntimePython != after.UV.RuntimePython
	res.groupChanged = !slices.Equal(before.RuntimeGroup, after.RuntimeGroup)
	if recordOnly {
		return
	}
	res.airflowChanged = !reflect.DeepEqual(airflowSettings(before), airflowSettings(after))
	// A write that changed neither the Airflow pins, the record nor the group
	// repaired another of the settings that hold Airflow to the build: the
	// index, its sources, or exclude-newer-package. Taking out the runtime
	// pins an older astro wrote is such a write too.
	res.airflowChanged = res.airflowChanged || (changed && !res.recordChanged && !res.groupChanged)
}

// runtimeImage resolves the image the project in dir builds on: for the seed
// of its lock, and for a declared Dockerfile, the build the Airflow lookup is
// asked about. A Dockerfile's exact build needs no catalog.
func (c *cli) runtimeImage(ctx context.Context, dir string, m *manifest.Manifest) runtimeImage {
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build, python, err := scaffold.RuntimeBuildFor(dir, m, catalog)
	image := runtimeImage{build: build, python: python, err: err}
	if err == nil && m.Astro.Dockerfile == "" {
		image.otherPython = scaffold.BuildPython(build, m.Project.RequiresPython, catalog) != python
	}
	return image
}

// runtimePins are what the lock of the project m describes is seeded with:
// the package list of image's build. None, which seeds nothing and takes the
// record of an earlier seed out, whenever they would no longer fit:
//
//   - under a cutoff (underCutoff): a list is of the present, and versions
//     newer than the cutoff would leave uv nothing to lock. Nothing is
//     resolved or read for them then;
//   - the Airflow pin's own build was taken out (moved while the index could
//     not be read);
//   - the project builds on no runtime, or its runtime publishes no list for
//     the Python the project runs;
//   - the list's Airflow is another release than the Airflow pin's, or, with
//     no Astronomer build of the pin (Airflow then comes from PyPI), one the
//     pin does not cover;
//   - the list of a build other than the recorded one could not be read.
//
// nil, which leaves the lock and its record as they are, only for what
// passes: the list of the recorded build and Python could not be read, or the
// build cannot be told for want of a catalog. res.seedWarn says what happened
// when it is worth a warning.
func (c *cli) runtimePins(ctx context.Context, dir string, m *manifest.Manifest, image runtimeImage, cutoff bool, res *astroBuildWrite) *scaffold.RuntimePins {
	none := &scaffold.RuntimePins{}
	noBuild := errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild)
	switch {
	case c.d.RuntimePins == nil:
		return nil
	case cutoff:
		res.cutoff = true
		return none
	case res.build.Airflow == "" && !noBuild, errors.Is(image.err, scaffold.ErrNoRuntimeBuild):
		return none
	case image.err != nil, image.python == "":
		// The build cannot be told, for want of a catalog. The record stays
		// while the Airflow it was made beside is one the pin still covers; a
		// pin that moved off it leaves it another Airflow's.
		recorded := releaseIn(m.UV.ConstraintDependencies, runtimeversions.DistAirflow)
		if m.UV.RuntimeBuild != "" && recorded != "" && !manifest.PinCovers(m.Airflow().Pin, recorded) {
			res.seedWarn = fmt.Errorf("uv.lock is no longer recorded as seeded from Astro Runtime %s: its Airflow %s is one the Airflow requirement no longer covers, and which build carries the new one cannot be told without the runtime catalog",
				m.UV.RuntimeBuild, recorded)
			return none
		}
		return nil
	}
	freeze, err := c.d.RuntimePins(ctx, image.build, image.python)
	switch {
	case errors.Is(err, runtimecatalog.ErrFreezeNotPublished):
		// A list of another Python would seed versions of a Python the venv
		// never runs, so there is none. Worth saying for a build that ships a
		// list for its own Python (a patch pin's, older than the series'
		// newest); not for one with none at all.
		if image.otherPython {
			res.seedWarn = fmt.Errorf("uv.lock is not seeded: Astro Runtime %s publishes no package list for Python %s, the one this project runs",
				image.build, image.python)
		}
		return none
	case errors.Is(err, runtimecatalog.ErrPinnedResolution):
		res.cutoff = true
		return none
	case err != nil && m.UV.RuntimeBuild == image.build && m.UV.RuntimePython == image.python:
		res.seedWarn = fmt.Errorf("uv.lock stays as it is: could not read what Astro Runtime %s ships: %w", image.build, err)
		return nil
	case err != nil:
		// The build moved on and its list cannot be read yet: the seed there
		// still describes the build the images use, so it stays while the
		// requirement covers its Airflow.
		return keepRecordedSeed(m, res,
			fmt.Sprintf("could not read what Astro Runtime %s ships: %v", image.build, err),
			fmt.Errorf("uv.lock is not seeded until what Astro Runtime %s ships can be read: %w", image.build, err))
	}
	// The Airflow pin and the seed have to be of one release, or the lock
	// mixes one Airflow with another's dependencies. With no Astronomer build
	// of the pin, Airflow comes from PyPI at the release the pin covers, so
	// the build's list applies while it carries one of those.
	carried := releaseIn(freeze, runtimeversions.DistAirflow)
	if noBuild {
		if carried == "" || !manifest.PinCovers(m.Airflow().Pin, carried) {
			return none
		}
	} else if pinned := astroRelease(res.build.Airflow); carried != "" && carried != pinned {
		if m.Astro.Dockerfile == "" && m.Airflow().Runtime == "" {
			// Nothing the user wrote names the build: the lookup and the
			// catalog's newest build disagree on their own, so the seed
			// follows the build carrying the release the lookup chose.
			return c.pinsOfRelease(ctx, dir, m, pinned, res)
		}
		// A Dockerfile FROM, or a runtime, the requirement excludes.
		res.seedWarn = fmt.Errorf("uv.lock is not seeded: Astro Runtime %s carries Airflow %s, and the Airflow requirement holds it to %s; make the two agree",
			image.build, carried, pinned)
		return none
	}
	return &scaffold.RuntimePins{Build: image.build, Python: image.python, Pins: freeze}
}

// pinsOfRelease is the list of the newest published build carrying Airflow
// release, to seed with, for a project that names no build itself. With none
// published yet, the seed there stays while the requirement covers its
// Airflow, and its record comes out otherwise; either way the warning blames
// nothing the user wrote.
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
	return keepRecordedSeed(m, res, why, fmt.Errorf("uv.lock is not seeded: %s", why))
}

// keepRecordedSeed leaves the lock and its record as they are when the
// Airflow requirement still covers the Airflow the lock was seeded beside, and
// holds res.build to that Airflow build too, so the lock does not pair one
// release with another's dependencies. It says why with kept. Otherwise, or
// with no seed recorded, the record comes out, with dropped as the warning.
func keepRecordedSeed(m *manifest.Manifest, res *astroBuildWrite, kept string, dropped error) *scaffold.RuntimePins {
	recorded := recordedAstroBuild(m)
	if m.UV.RuntimeBuild == "" || recorded.Airflow == "" || !manifest.PinCovers(m.Airflow().Pin, recorded.Airflow) {
		res.seedWarn = dropped
		return &scaffold.RuntimePins{}
	}
	if res.build.Index != "" {
		recorded.Index = res.build.Index
	}
	res.build = recorded
	res.seedWarn = fmt.Errorf("uv.lock stays seeded from Astro Runtime %s: %s", m.UV.RuntimeBuild, kept)
	return nil
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

// astroRelease is a version's public part, without its +astro build: 3.3.2
// for 3.3.2+astro.2. "" for a version PEP 440 does not spell.
func astroRelease(version string) string {
	v, err := pep440.Parse(strings.TrimSpace(version))
	if err != nil {
		return ""
	}
	return v.Public()
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
// 3.3.2+astro.2, one public version with an astro local part each.
func sameRelease(a, b string) bool {
	va, errA := pep440.Parse(strings.TrimSpace(a))
	vb, errB := pep440.Parse(strings.TrimSpace(b))
	return errA == nil && errB == nil && va.Public() == vb.Public() &&
		strings.HasPrefix(va.Local(), "astro.") && strings.HasPrefix(vb.Local(), "astro.")
}

// airflowSettings are the parts of a manifest SetAstroBuild changes to hold
// Airflow to a build, apart from the seed's record: what "now installs
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
