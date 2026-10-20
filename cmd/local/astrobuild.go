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
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"could not read what Astro Runtime %s ships, so %s keeps the runtime pins it has: %v", res.pinsBuild, manifest.Marker, res.pinsErr)})
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
		release, _, _ := strings.Cut(strings.TrimSpace(version), "+")
		if release != pin && !strings.HasPrefix(release, pin+".") {
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
	// pinsBuild is the build whose package list could not be read, and
	// pinsErr why: the runtime pins were left as they are.
	pinsBuild string
	pinsErr   error
}

// writeAstroBuild looks up the build for the manifest in dir and writes it,
// or takes the settings out when the index has none, with the runtime pins of
// the image the project builds on (runtimePins). It writes nothing when the
// lookup could not be made, or when there is no lookup to make.
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
	res.build, res.lookupErr = c.d.AstroBuild(ctx, airflow.Pin, airflow.Runtime)
	if res.lookupErr != nil && !errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild) && !stalePin(m) {
		return res
	}
	pins := c.runtimePins(ctx, dir, m, &res)
	switch {
	case pins != nil:
		res.build = imageAirflowBuild(res.build, pins.Pins)
	case m.UV.RuntimeBuild != "":
		// The pins stay, so the Airflow build they were written beside does
		// too: it is the image's, which the index lookup does not know.
		res.build = imageAirflowBuild(res.build, m.UV.ConstraintDependencies)
	}
	_, res.writeErr = watchManifest(dir, func(wrap func(run func() error) error) error {
		return scaffold.SetAstroBuild(dir, wrap, res.build, pins)
	})
	if after, err := manifest.Load(path); err == nil {
		res.after = after.UV
		res.airflowChanged = !reflect.DeepEqual(airflowSettings(m), airflowSettings(after))
		res.pinsChanged = m.UV.RuntimeBuild != after.UV.RuntimeBuild || m.UV.RuntimePython != after.UV.RuntimePython ||
			!slices.Equal(m.UV.RuntimePins, after.UV.RuntimePins)
	}
	return res
}

// runtimePins are the runtime pins to write for the manifest m in dir: the
// package list of the build scaffold.RuntimeBuildFor names. nil, which leaves
// the pins there as they are, when that cannot be told or the list cannot be
// read (res.pinsErr says why, for a list); none when there is no build, or no
// published list for it, which takes them out.
func (c *cli) runtimePins(ctx context.Context, dir string, m *manifest.Manifest, res *astroBuildWrite) *scaffold.RuntimePins {
	if c.d.RuntimePins == nil {
		return nil
	}
	if res.build.Airflow == "" {
		// No Astronomer build of this Airflow, so no runtime carries it.
		return &scaffold.RuntimePins{}
	}
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build, python, err := scaffold.RuntimeBuildFor(dir, m, catalog)
	switch {
	case errors.Is(err, scaffold.ErrNoRuntimeBuild):
		return &scaffold.RuntimePins{}
	case err != nil, python == "":
		return nil
	}
	freeze, err := c.d.RuntimePins(ctx, build, python)
	switch {
	case errors.Is(err, runtimecatalog.ErrFreezeNotPublished):
		return &scaffold.RuntimePins{Build: build, Python: python}
	case errors.Is(err, runtimecatalog.ErrPinnedResolution):
		return nil
	case err != nil:
		res.pinsBuild, res.pinsErr = build, err
		return nil
	}
	return &scaffold.RuntimePins{Build: build, Python: python, Pins: freeze}
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
