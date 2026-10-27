package local

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
			// A group the project took out of default-groups lists them; uv
			// installs them only from one it left in.
			verb := "lists"
			if res.groupInstalled {
				verb = "installs"
			}
			of := "the runtime image"
			if build := cmp.Or(res.groupBuild, res.after.RuntimeBuild); build != "" {
				of = "Astro Runtime " + build
			}
			fmt.Fprintf(c.d.Stderr, "%s: [dependency-groups] %s now %s the %d packages %s installs\n",
				manifest.Marker, manifest.RuntimeGroupName, verb, n, of)
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
	// cutoff says nothing is seeded for the project's own cutoff, which
	// keeps the runtime group (runtimeGroup).
	cutoff bool
	// uvMissing says a seed was not made for want of uv: init has nothing to
	// relock with either (relockAfterInit).
	uvMissing bool
	// offline is a write whose Airflow lookup could not be made, which
	// leaves the Airflow build as the manifest has it: nothing it cannot
	// tell for want of the index changes the seed.
	offline bool
	// groupWritten says the runtime group decided on was written, and
	// groupBuild is the build it is of. groupErr says why the group was not
	// brought up to date, when it is worth a warning, and groupLeftOut the
	// image's packages it leaves out for having no wheel here.
	groupWritten bool
	groupBuild   string
	groupErr     error
	groupLeftOut []runtimecatalog.LeftOut
	// groupInstalled says uv installs the group as the write left it: it is
	// in [tool.uv] default-groups (scaffold.RuntimeGroupInstalled).
	groupInstalled bool
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
// the seed, and its group and record (scaffold.SetRuntimeGroup), which also takes out
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
	pins := c.runtimePins(ctx, dir, m, image, cut, &res)
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
	willSeed, now := c.seedPlan(dir, m, pins, group, seed, &res)
	changed, writeErr := watchManifest(dir, func(wrap func(run func() error) error) error {
		var err error
		if offline {
			// Still a write: it takes out what an older astro wrote into
			// constraint-dependencies, which a seed must not read.
			res.refused, err = scaffold.SetRuntimeGroup(dir, wrap, now, record)
			return err
		}
		res.refused, err = scaffold.SetAstroBuildSeed(dir, wrap, res.build, now, record)
		return err
	})
	res.writeErr = writeErr
	res.groupWritten = now != nil && writeErr == nil && !res.refused
	if writeErr == nil && pins != nil && len(pins.Pins) > 0 {
		var later *scaffold.RuntimeGroup
		if now == nil {
			later = group
		}
		changed = c.applySeed(ctx, dir, pins, seedRun{seed: willSeed, init: seed}, later, &res) || changed
	}
	res.compare(m, path, changed, offline)
	return res
}

// seedPlan decides whether the write seeds the lock with pins (init asks;
// otherwise a project with no lock, or a group that gains packages the lock
// may not hold), and which runtime group it writes now, before any seed. A
// seed decides on its group: the group of a build the lock is to be seeded
// from, or one with packages the lock does not hold yet, is written with the
// seed's record as part of the seed (applySeed), in pyproject.toml while the
// lock is seeded and kept only when the seed works, so now is nil then. A
// group that only lost packages the project now lists itself needs no seed.
//
// A start also seeds when the Airflow pin moved off the release of the build
// the lock was seeded from (the lock is another Airflow's). A newer build of
// the same release does not: the lock, its record and the group stay that
// build's, and the drift notice and `astro init` move them together.
//
// A start does not try again a seed that failed or held nothing (or whose
// group could not be written) with the same pyproject.toml, uv.lock, build
// and Python: it would fail the same way (seedSkipped). init always tries.
// A member of a uv workspace is never seeded, and res.seedWarn says why.
func (c *cli) seedPlan(dir string, m *manifest.Manifest, pins *scaffold.RuntimePins, group *scaffold.RuntimeGroup, seed bool, res *astroBuildWrite) (willSeed bool, now *scaffold.RuntimeGroup) {
	seedable := pins != nil && len(pins.Pins) > 0 && c.d.Lock != nil
	if seedable {
		if root := workspaceRoot(dir); root != "" {
			res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: this project is a member of the uv workspace at %s, whose uv.lock is at its root, and astro seeds only a project's own lock",
				pins.Build, root)
			seedable = false
		}
	}
	if !seedable {
		return false, group
	}
	moved := m.UV.RuntimeBuild != "" && pins.Build != m.UV.RuntimeBuild && !recordCovered(m)
	adds := group != nil && scaffold.RuntimeGroupAdds(dir, group)
	if seed || !hasLock(dir) || adds || moved {
		return true, nil
	}
	if group != nil && pins.Build != m.UV.RuntimeBuild {
		// A newer build of the recorded release: the group stays the one the
		// lock was seeded with, as the record does.
		return false, nil
	}
	return false, group
}

// seedRun is what seedPlan decided for applySeed: seed, whether the write
// seeds the lock; init, whether it is `astro init`'s, which tries a seed a
// start would skip.
type seedRun struct {
	seed, init bool
}

// recordCovered reports whether the Airflow pin still covers the release of
// the Astronomer build the manifest holds Airflow to beside the seed record,
// or there is none to tell by.
func recordCovered(m *manifest.Manifest) bool {
	recorded := recordedAstroBuild(m).Airflow
	return recorded == "" || manifest.PinCovers(m.Airflow().Pin, astroRelease(recorded))
}

// applySeed seeds the lock of the project in dir with pins when seed asks,
// with group, the runtime group that waited for it, and the seed's record
// written into pyproject.toml as part of the seed, so the lock holds the
// group's packages at the image's versions; otherwise it counts how far the
// lock is from pins. A seed that fails leaves the lock, the record and the
// group as they were before it, and says why in res.seedWarn. It reports
// whether it changed the manifest.
func (c *cli) applySeed(ctx context.Context, dir string, pins *scaffold.RuntimePins, run seedRun, group *scaffold.RuntimeGroup, res *astroBuildWrite) bool {
	// A start does not repeat a seed that came to nothing with these inputs
	// (seedSkipped); init always tries.
	if !run.seed || (!run.init && seedSkipped(dir, pins)) {
		if hasLock(dir) {
			if res.drift = lockDrift(dir, pins); res.drift > 0 {
				res.driftBuild = pins.Build
			}
		}
		return false
	}
	path := filepath.Join(dir, manifest.Marker)
	before, _ := os.ReadFile(path) //nolint:errcheck // unread, any change counts as one
	record := scaffold.Seed{Build: pins.Build, Python: pins.Python}
	var refused bool
	n, err := c.seedLock(ctx, dir, pins, func() error {
		var err error
		refused, err = scaffold.SetRuntimeGroup(dir, nil, group, &record)
		return err
	})
	// A seed that fails or holds nothing put the project back as it was, so
	// the same inputs are remembered and a start does not try it again; one
	// that worked but could not write its group (refused) is remembered as
	// it left the project, which its next start would otherwise seed again.
	switch {
	case errors.Is(err, uv.ErrNotFound):
		// Installing uv changes nothing a start can see, so this one is not
		// remembered.
		res.uvMissing = true
		res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: uv is not installed", pins.Build)
		return false
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: %w", pins.Build, err)
		return false
	case err != nil:
		rememberSeedSkip(dir, pins)
		res.seedWarn = fmt.Errorf("uv.lock is not seeded with what Astro Runtime %s ships: %w", pins.Build, err)
		return false
	case n == 0:
		rememberSeedSkip(dir, pins)
		return false
	}
	if refused {
		rememberSeedSkip(dir, pins)
	}
	res.seeded, res.seed = n, record
	res.refused = res.refused || refused
	res.groupWritten = res.groupWritten || (group != nil && !refused)
	after, readErr := os.ReadFile(path)
	return readErr != nil || !bytes.Equal(before, after)
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
	res.groupInstalled = scaffold.RuntimeGroupInstalled(filepath.Dir(path))
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
func (c *cli) runtimePins(ctx context.Context, dir string, m *manifest.Manifest, image runtimeImage, cut cutoff, res *astroBuildWrite) *scaffold.RuntimePins {
	none := &scaffold.RuntimePins{}
	noBuild := errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild)
	switch {
	case c.d.RuntimePins == nil:
		return nil
	case cut == projectCutoff:
		res.cutoff = true
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
// release with another's dependencies. It says why with kept. With no seed
// recorded there is nothing to keep, and nothing changes either; a pin that
// moved off the recorded Airflow takes the record out, with dropped as the
// warning. A seed beside Airflow from PyPI (no Astronomer build recorded)
// stays.
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
	if recorded.Airflow != "" && !manifest.PinCovers(m.Airflow().Pin, recorded.Airflow) {
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
