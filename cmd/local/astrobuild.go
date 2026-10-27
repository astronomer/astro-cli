package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	pep440 "github.com/aquasecurity/go-pep440-version"
	"github.com/pelletier/go-toml/v2"

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
	res := c.writeAstroBuild(ctx, proj.Dir, false)
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

// reportSeed says what writeAstroBuild did with the project's uv.lock: seeded
// it, found it apart from the image, or could not, and why. For a start, and
// for init, which has no to-do list of its own for it.
func (c *cli) reportSeed(r cliout.Renderer, res astroBuildWrite) {
	if res.seedWarn != nil {
		emitWarning(r, event{Event: "warning", Text: res.seedWarn.Error()})
	}
	if res.recordRefused {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"%s does not record which Astro Runtime uv.lock was seeded from: its [tool.uv] is an inline table, which cannot hold the comment that records it. Write it as a [tool.uv] table to have it", manifest.Marker)})
	}
	switch {
	case res.seeded > 0:
		fmt.Fprintf(c.d.Stderr, "uv.lock: seeded with the versions Astro Runtime %s (Python %s) ships (%d packages)%s\n",
			res.seed.Build, res.seed.Python, res.seeded, narrowedNote(res.seed.Python, res.seedNarrowed))
	case res.drift > 0:
		fmt.Fprintf(c.d.Stderr, "uv.lock: %d packages differ from Astro Runtime %s; run astro init to re-seed\n", res.drift, res.driftBuild)
	case res.recordChanged && res.after.RuntimeBuild == "":
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer records an Astro Runtime uv.lock was seeded from\n", manifest.Marker)
	}
}

// narrowedNote is what the seeded notice adds when the seed held the image's
// versions on its Python alone: which packages kept uv from holding them on
// every Python requires-python allows, when uv named them.
func narrowedNote(python string, narrowed error) string {
	if narrowed == nil {
		return ""
	}
	why := "uv could not hold them on every Python requires-python allows"
	var resolution *uv.ResolutionError
	if errors.As(narrowed, &resolution) && len(resolution.Packages) > 0 {
		pkgs := resolution.Packages
		if len(pkgs) > 3 {
			pkgs = pkgs[:3]
		}
		why = "uv could not hold " + strings.Join(pkgs, ", ") + " on every Python requires-python allows"
	}
	return "; held for Python " + python + " only: " + why
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
		if !manifest.PinCovers(pin, astroRelease(version)) {
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
	// airflowChanged and recordChanged say which part of [tool.uv] the write
	// changed: the settings that hold Airflow to the build, or the record of
	// the seed. after is [tool.uv] as the write left it.
	airflowChanged, recordChanged bool
	after                         manifest.UV
	// seeded is how many of the image's versions the lock was seeded with,
	// and seed the build and Python they were; drift how many the lock holds
	// apart from driftBuild's, when it was not seeded.
	seeded     int
	seed       scaffold.Seed
	drift      int
	driftBuild string
	// seedNarrowed is why the seed held the image's versions on its Python
	// alone rather than on every Python requires-python allows; nil when it
	// held them everywhere.
	seedNarrowed error
	// seedWarn says what happened to the seed, when it is worth a warning,
	// and recordRefused that its record could not be written at all.
	seedWarn      error
	recordRefused bool
	// offline is a write whose Airflow lookup could not be made, which
	// leaves the Airflow build as the manifest has it: nothing it cannot
	// tell for want of the index changes the seed.
	offline bool
	// updated is a write that changed the file, which could not be read back
	// to say what changed.
	updated bool
	// wouldSeed is a write that seeds the lock if it can: init's, or a
	// start's with no uv.lock. Only then is a list that cannot be read worth
	// a warning; a start with a lock only counts drift.
	wouldSeed bool
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
// the seed, and its record (scaffold.SetSeedRecord), which also takes out
// what an older astro wrote.
//
// The record changes only after a seed of its build succeeds, or on a
// definitive answer that there is nothing to seed (runtimePins); a failure
// to read or reach anything leaves it, and the lock, as they are.
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
	res := astroBuildWrite{pin: airflow.Pin, wouldSeed: seed || !hasLock(dir)}
	// The image is resolved for the seed, and for a declared Dockerfile's
	// build, which the Airflow lookup is asked about. Under a cutoff there is
	// nothing to seed (runtimePins), so nothing is read for one.
	cut := underCutoff(dir, m)
	image := runtimeImage{err: errNoImage}
	if (c.d.RuntimePins != nil && cut == noCutoff) || m.Astro.Dockerfile != "" {
		image = c.runtimeImage(ctx, dir, m)
	}
	runtime := airflow.Runtime
	if m.Astro.Dockerfile != "" && image.err == nil {
		runtime = image.build
	}
	res.build, res.lookupErr = c.d.AstroBuild(ctx, airflow.Pin, runtime)
	offline := res.lookupErr != nil && !errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild) && !stalePin(m)
	res.offline = offline
	if offline {
		res.build = recordedAstroBuild(m)
	}
	pins := c.runtimePins(ctx, m, image, cut, &res)
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
			// Still a write: it takes out what an older astro wrote into
			// constraint-dependencies, which a seed must not read.
			res.recordRefused, err = scaffold.SetSeedRecord(dir, wrap, record)
			return err
		}
		res.recordRefused, err = scaffold.SetAstroBuildSeed(dir, wrap, res.build, record)
		return err
	})
	res.writeErr = writeErr
	if writeErr == nil && pins != nil && len(pins.Pins) > 0 {
		recorded, recordErr := c.applySeed(ctx, dir, m, pins, seed, &res)
		changed = changed || recorded
		if recordErr != nil {
			res.writeErr = recordErr
		}
	}
	res.compare(m, path, changed, offline)
	return res
}

// applySeed seeds the lock of the project in dir with pins, and records the
// seed, when seed asks or there is no lock; otherwise it counts how far the
// lock is from pins. It reports whether it changed the manifest.
func (c *cli) applySeed(ctx context.Context, dir string, m *manifest.Manifest, pins *scaffold.RuntimePins, seed bool, res *astroBuildWrite) (bool, error) {
	if root := workspaceRoot(dir); root != "" {
		res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: this project is a member of the uv workspace at %s, whose uv.lock is at its root, and astro seeds only a project's own lock",
			pins.Build, root)
		return false, nil
	}
	if !seed && hasLock(dir) {
		if res.drift = lockDrift(dir, pins); res.drift > 0 {
			res.driftBuild = pins.Build
		}
		return false, nil
	}
	if c.d.Lock == nil {
		return false, nil
	}
	n, narrowed, err := c.seedLock(ctx, dir, pins)
	res.seedNarrowed = narrowed
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
		refused, err = scaffold.SetSeedRecord(dir, wrap, &res.seed)
		return err
	})
	res.recordRefused = res.recordRefused || refused
	return changed, err
}

// errNoImage is the runtimeImage of a write that has nothing to resolve it
// for: under a cutoff, or with no seed to make and no Dockerfile.
var errNoImage = errors.New("the runtime image was not resolved")

// cutoff is which cutoff holds the project's uv, if any: a runtime's package
// list is of the present, and versions newer than a cutoff would leave uv
// nothing to lock, so a project under one is not seeded.
type cutoff int

const (
	noCutoff cutoff = iota
	// projectCutoff is the project's own: [tool.uv] exclude-newer, or
	// exclude-newer in its uv.toml. Its lock is not seeded, and the record
	// of an earlier seed comes out.
	projectCutoff
	// envCutoff is UV_EXCLUDE_NEWER, or exclude-newer in the user's or the
	// system's uv.toml: the environment's, which says nothing of the
	// project: its lock and the record stay as they are.
	envCutoff
)

// underCutoff is the cutoff the project in dir resolves as of, a date or a
// duration alike: the one rule for whether a seed may be made.
//
// The user's and the system's uv.toml say nothing of the project either, and
// count as the environment's: uv reads them on this machine only.
func underCutoff(dir string, m *manifest.Manifest) cutoff {
	switch {
	case m.UV.ExcludeNewer != "" || uvTOMLExcludeNewer(filepath.Join(dir, "uv.toml")) != "":
		return projectCutoff
	case strings.TrimSpace(os.Getenv("UV_EXCLUDE_NEWER")) != "":
		return envCutoff
	}
	for _, path := range userUVConfigs() {
		if uvTOMLExcludeNewer(path) != "" {
			return envCutoff
		}
	}
	return noCutoff
}

// userUVConfigs are the uv.toml files uv reads beyond the project's, in the
// order it does: UV_CONFIG_FILE alone when it is set; else the user's
// ($XDG_CONFIG_HOME/uv/uv.toml, or ~/.config/uv/uv.toml) and the system's
// (/etc/uv/uv.toml). None under UV_NO_CONFIG, which makes uv read none.
func userUVConfigs() []string {
	if v := strings.TrimSpace(os.Getenv("UV_NO_CONFIG")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return nil
	}
	if f := strings.TrimSpace(os.Getenv("UV_CONFIG_FILE")); f != "" {
		return []string{f}
	}
	var paths []string
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "uv", "uv.toml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "uv", "uv.toml"))
	}
	return append(paths, filepath.Join(string(filepath.Separator), "etc", "uv", "uv.toml"))
}

// uvTOMLExcludeNewer is exclude-newer in the uv.toml at path; "" when there is
// none.
func uvTOMLExcludeNewer(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var settings struct {
		ExcludeNewer any `toml:"exclude-newer"`
	}
	if toml.Unmarshal(data, &settings) != nil || settings.ExcludeNewer == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(settings.ExcludeNewer))
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
// changed, alone. recordOnly is a write of the record alone (a start that
// cannot reach the index), so whatever changed is the record's.
//
// When the file cannot be read back, nothing is attributed: a write that
// changed it says only that it did (updated).
func (res *astroBuildWrite) compare(before *manifest.Manifest, path string, changed, recordOnly bool) {
	after, err := manifest.Load(path)
	if err != nil {
		res.updated = changed
		return
	}
	res.after = after.UV
	res.recordChanged = before.UV.RuntimeBuild != after.UV.RuntimeBuild || before.UV.RuntimePython != after.UV.RuntimePython
	if recordOnly {
		return
	}
	res.airflowChanged = !reflect.DeepEqual(airflowSettings(before), airflowSettings(after))
	// A write that changed neither the Airflow pins nor the record repaired
	// another of the settings that hold Airflow to the build: the index, its
	// sources, or exclude-newer-package. Taking out the runtime pins an older
	// astro wrote is such a write too.
	res.airflowChanged = res.airflowChanged || (changed && !res.recordChanged)
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
// record of an earlier seed out, only on a definitive answer that they would
// no longer fit:
//
//   - the project's own cutoff (projectCutoff): a list is of the present, and
//     versions newer than the cutoff would leave uv nothing to lock. Nothing
//     is resolved or read for them then;
//   - the Airflow pin's own build was taken out (the pin moved off it while
//     the index could not be read);
//   - the project builds on no runtime, or its runtime publishes no list for
//     the Python the project runs;
//   - the list's Airflow is another release than the Airflow pin's, or, with
//     no Astronomer build of the pin (Airflow then comes from PyPI), one the
//     pin does not cover.
//
// nil, which leaves the lock and its record as they are, for everything that
// passes: the index could not be reached, UV_EXCLUDE_NEWER is set in the
// environment, a list could not be read while the requirement still covers
// the Airflow the lock was seeded beside (keepRecordedSeed), or the build
// cannot be told for want of a catalog. res.seedWarn says what happened when
// it is worth a warning; one, at most.
func (c *cli) runtimePins(ctx context.Context, m *manifest.Manifest, image runtimeImage, cut cutoff, res *astroBuildWrite) *scaffold.RuntimePins {
	none := &scaffold.RuntimePins{}
	noBuild := errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild)
	switch {
	case c.d.RuntimePins == nil:
		return nil
	case cut == projectCutoff:
		return none
	case cut == envCutoff:
		if m.UV.RuntimeBuild != "" {
			res.seedWarn = fmt.Errorf("uv.lock stays seeded from Astro Runtime %s: this environment holds uv to a cutoff (UV_EXCLUDE_NEWER, or exclude-newer in your own uv.toml), which a runtime's package list does not follow", m.UV.RuntimeBuild)
		}
		return nil
	case res.build.Airflow == "" && !noBuild && res.offline:
		// The lookup could not be made, and the manifest holds no Astronomer
		// build to keep (Airflow from PyPI): what the lock was seeded with is
		// not the index's to say. The lookup's error is warned of already.
		return nil
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
	case err != nil:
		// The list could not be read: a network error, a timeout, a CDN or
		// mirror that does not answer. The seed there stays while it is still
		// of the Airflow the requirement covers.
		return keepRecordedSeed(m, res,
			fmt.Sprintf("could not read what Astro Runtime %s ships: %v", image.build, err),
			fmt.Errorf("uv.lock is not seeded until what Astro Runtime %s ships can be read: %w", image.build, err))
	}
	// The Airflow pin and the seed have to be of one release, or the lock
	// mixes one Airflow with another's dependencies. A Dockerfile FROM a build
	// the requirement excludes is the case that reaches here. With no
	// Astronomer build of the pin, Airflow comes from PyPI at the release the
	// pin covers, so the build's list applies while it carries one of those.
	carried := releaseIn(freeze, runtimeversions.DistAirflow)
	if noBuild {
		if carried == "" || !manifest.PinCovers(m.Airflow().Pin, carried) {
			return none
		}
	} else if pinned := astroRelease(res.build.Airflow); carried != "" && carried != pinned {
		res.seedWarn = fmt.Errorf("uv.lock is not seeded: Astro Runtime %s carries Airflow %s, and the Airflow requirement holds it to %s; make the two agree",
			image.build, carried, pinned)
		return none
	}
	return &scaffold.RuntimePins{Build: image.build, Python: image.python, Pins: freeze}
}

// keepRecordedSeed leaves the lock and its record as they are when the
// Airflow requirement still covers the Airflow the lock was seeded beside, and
// holds res.build to that Airflow build too, so the lock does not pair one
// release with another's dependencies. It says why with kept. With no seed
// recorded there is nothing to keep, and nothing changes either; a pin that
// moved off the recorded Airflow takes the record out, with dropped as the
// warning.
//
// The warnings are for a write that would seed (res.wouldSeed): a start with a
// lock only counts drift, and has nothing to say about a list it did not need.
func keepRecordedSeed(m *manifest.Manifest, res *astroBuildWrite, kept string, dropped error) *scaffold.RuntimePins {
	if m.UV.RuntimeBuild == "" {
		if res.wouldSeed {
			res.seedWarn = dropped
		}
		return nil
	}
	recorded := recordedAstroBuild(m)
	if recorded.Airflow != "" && !manifest.PinCovers(m.Airflow().Pin, astroRelease(recorded.Airflow)) {
		res.seedWarn = dropped
		return &scaffold.RuntimePins{}
	}
	if recorded.Airflow != "" {
		if res.build.Index != "" {
			recorded.Index = res.build.Index
		}
		res.build = recorded
	}
	if res.wouldSeed {
		res.seedWarn = fmt.Errorf("uv.lock stays seeded from Astro Runtime %s: %s", m.UV.RuntimeBuild, kept)
	}
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
