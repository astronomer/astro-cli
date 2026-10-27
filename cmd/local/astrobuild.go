package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

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
	case res.lookupErr != nil && res.changed:
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"Airflow %s comes from PyPI for now: the Airflow pin moved, and Astronomer's index could not be read for the new build: %v", res.pin, res.lookupErr)})
	case res.lookupErr != nil:
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf(
			"could not check %s against Astronomer's newest build of Airflow, so it is used as it stands: %v", manifest.Marker, res.lookupErr)})
	}
	if res.writeErr != nil {
		emitWarning(r, event{Event: "warning", Text: fmt.Sprintf("could not update %s for Astronomer's build of Airflow: %v", manifest.Marker, res.writeErr)})
	}
	if res.runtime != "" {
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] now holds what Astro Runtime %s constrains\n", manifest.Marker, res.runtime)
	}
	if res.runtimeGone != "" {
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer holds what Astro Runtime %s constrains\n", manifest.Marker, res.runtimeGone)
	}
	// The Airflow line is said whenever the Airflow pins moved, beside a
	// runtime line or not; a write that touched only the runtime's lines says
	// only that.
	onlyRuntime := (res.runtime != "" || res.runtimeGone != "") && !res.airflowChanged
	switch {
	case !res.changed || onlyRuntime:
	case res.build.Airflow != "":
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] now installs Astronomer's Airflow %s\n", manifest.Marker, res.build.Airflow)
	default:
		fmt.Fprintf(c.d.Stderr, "%s: [tool.uv] no longer points Airflow at Astronomer's index\n", manifest.Marker)
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
	pin     string
	build   runtimeversions.AstroBuild
	changed bool
	// airflowChanged reports that the write moved the +astro Airflow pins.
	airflowChanged bool
	// runtime is the build whose constraints the write newly recorded, and
	// runtimeGone the one whose record it took out; "" when neither happened.
	runtime, runtimeGone string
	lookupErr            error
	writeErr  error
}

// writeAstroBuild looks up the build for the manifest in dir and writes it,
// or takes the settings out when the index has none. It writes nothing when
// the lookup could not be made, or when there is no lookup to make.
func (c *cli) writeAstroBuild(ctx context.Context, dir string) astroBuildWrite {
	if c.d.AstroBuild == nil {
		return astroBuildWrite{}
	}
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return astroBuildWrite{}
	}
	airflow := m.Airflow()
	res := astroBuildWrite{pin: airflow.Pin}
	res.build, res.lookupErr = c.d.AstroBuild(ctx, airflow.Pin, airflow.Runtime)
	if res.lookupErr != nil && !errors.Is(res.lookupErr, runtimeversions.ErrNoAstroBuild) && !stalePin(m) {
		return res
	}
	rc := c.runtimeConstraints(ctx, dir, m)
	res.changed, res.writeErr = watchManifest(dir, func(wrap func(run func() error) error) error {
		return scaffold.SetAstroBuildWith(dir, wrap, res.build, rc)
	})
	if !res.changed || res.writeErr != nil {
		return res
	}
	// What moved, from what was written: no second read of the file.
	var before, after []string
	for _, c := range m.UV.ConstraintDependencies {
		if manifest.AstroPin(c) {
			before = append(before, c)
		}
	}
	if res.build.Airflow != "" {
		after = res.build.Pins()
	}
	res.airflowChanged = !slices.Equal(before, after)
	if rc != nil && rc.Build != m.UV.RuntimeBuild {
		if rc.Build != "" {
			res.runtime = rc.Build
		} else {
			res.runtimeGone = m.UV.RuntimeBuild
		}
	}
	return res
}

// runtimeConstraints is what the project's runtime image's own install holds
// packages to, for the build and Python the project builds on
// (scaffold.RuntimeBuildFor): written beside the Airflow pins, so the lockfile
// resolves Airflow's database driver, and the rest the runtime constrains, as
// the image does. nil leaves what is written as it is: no way to read it, a
// build or a file that cannot be read now. A zero one takes it out: a cutoff
// (the runtime's versions may postdate it, and uv would find no solution), a
// project on no runtime build, or one that publishes no such file (an Airflow
// 2 runtime before 11.0.0).
func (c *cli) runtimeConstraints(ctx context.Context, dir string, m *manifest.Manifest) *scaffold.RuntimeConstraints {
	if c.d.RuntimeConstraints == nil {
		return nil
	}
	if underCutoff(dir, m) {
		return &scaffold.RuntimeConstraints{}
	}
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build, python, err := scaffold.RuntimeBuildFor(dir, m, catalog)
	switch {
	case errors.Is(err, scaffold.ErrNoRuntimeBuild):
		return &scaffold.RuntimeConstraints{}
	case err != nil || python == "":
		return nil
	}
	lines, err := c.d.RuntimeConstraints(ctx, build, python)
	switch {
	case errors.Is(err, runtimecatalog.ErrNotPublished):
		return &scaffold.RuntimeConstraints{}
	case err != nil:
		return nil
	}
	return &scaffold.RuntimeConstraints{Build: build, Python: python, Lines: lines}
}

// underCutoff reports a uv resolve held to a date: the project's own
// exclude-newer, in [tool.uv] or its uv.toml, or UV_EXCLUDE_NEWER in the
// environment.
func underCutoff(dir string, m *manifest.Manifest) bool {
	return m.UV.ExcludeNewer != "" || strings.TrimSpace(os.Getenv("UV_EXCLUDE_NEWER")) != "" ||
		uvTOMLExcludeNewer(filepath.Join(dir, "uv.toml"))
}

// uvTOMLExcludeNewer reports a uv.toml at path that sets exclude-newer. A
// file that is missing or does not parse sets none.
func uvTOMLExcludeNewer(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg map[string]any
	if toml.Unmarshal(data, &cfg) != nil {
		return false
	}
	v, ok := cfg["exclude-newer"]
	return ok && v != nil && strings.TrimSpace(fmt.Sprint(v)) != ""
}
