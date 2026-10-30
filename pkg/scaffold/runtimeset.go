package scaffold

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// ErrInvalidRuntimeBuild reports a value SetRuntimeBuildWith refused before
// reading the manifest: not one Astro Runtime build.
var ErrInvalidRuntimeBuild = errors.New("not an Astro Runtime build")

// ErrInvalidPython reports a Python SetRuntimeBuildWith cannot set: not a
// minor like 3.13, one the build does not ship, or any Python for an Airflow 2
// build, whose image runs its own.
var ErrInvalidPython = errors.New("not a Python this runtime build runs")

// pythonMinorRe is a Python minor version, as a caller names one.
var pythonMinorRe = regexp.MustCompile(`^3\.\d+$`)

// onePythonRe is the requires-python SetRuntimeBuildWith writes to choose one
// Python: it admits that minor alone, so the image and the standalone venv
// both run it whatever the build's default (runtimeversions.ProjectPython).
var onePythonRe = regexp.MustCompile(`^==3\.\d+\.\*$`)

// RuntimeBuildChange is what SetRuntimeBuildWith did to a manifest.
type RuntimeBuildChange struct {
	// Previous is the runtime the manifest named, "" when it named none.
	Previous string `json:"previous,omitempty"`
	// Runtime is the build now written.
	Runtime string `json:"runtime"`
	// AirflowVersion is the exact Airflow the catalog says the build carries,
	// "" when the catalog does not list it.
	AirflowVersion string `json:"airflow_version,omitempty"`
	// Airflow is the change to the Airflow requirement, when the build carries
	// an Airflow the requirement did not cover. nil when it stayed.
	Airflow *AirflowPinChange `json:"airflow,omitempty"`
	// RequiresPython is the [project] requires-python now written, when it
	// changed. Empty when it was left alone.
	RequiresPython string `json:"requires_python,omitempty"`
	// Python is the Python the image of the build runs for the requires-python
	// written, "" when the catalog cannot tell.
	Python string `json:"python,omitempty"`
	// Changed reports that the file was written.
	Changed bool `json:"changed"`
}

// SetRuntimeBuildWith picks one Astro Runtime build for the project in dir,
// through EditManifest: it writes [tool.astro] runtime, and moves what has to
// move with it so the manifest stays loadable and the build is the one used.
//
//   - The Airflow requirement moves when the build carries an Airflow it does
//     not cover, with SetAirflowVersionWith's edit: to the build's exact
//     Airflow when the requirement pinned an exact one, else to its series.
//     Which Airflow a build carries comes from opts.Catalog; without it, or
//     for a build it does not list, an Airflow 3 build's tag still names its
//     series, and an Airflow 2 build is refused, since its tag names none.
//   - python, when given, is the Python the image and the standalone venv
//     run: requires-python becomes "==python.*". It has to be one the build
//     ships, when the catalog lists the build. Without it the build's default
//     is used: a requires-python that admits the default stays, one that
//     chose another single Python (the shape python writes) moves to the
//     series' bound, and one that admits no Python the build ships is refused.
//
// A build beside a declared dockerfile is refused: the Dockerfile's FROM line
// picks the image there. Whether the build is yanked, or listed at all, is the
// caller's to report (runtimeversions.Catalog.CheckRuntime): a build published
// after the catalog was cached is an ordinary reason to name one it lacks.
func SetRuntimeBuildWith(dir string, wrap func(run func() error) error, build, python string, opts AirflowPinOptions) (RuntimeBuildChange, error) {
	e, err := newRuntimeBuildEdit(build, python, opts)
	if err != nil {
		return RuntimeBuildChange{}, err
	}
	var out RuntimeBuildChange
	err = EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		var err error
		out, err = e.apply(before, ed)
		return err
	})
	if err != nil {
		return RuntimeBuildChange{}, err
	}
	return out, nil
}

// runtimeBuildEdit is one SetRuntimeBuildWith, checked before the manifest is
// read: the build, the Python asked for, and what the catalog says of the build.
type runtimeBuildEdit struct {
	build, python string
	tag           manifest.RuntimeTag
	// rt is the catalog's entry for build, and listed reports that there is
	// one naming the Airflow it carries.
	rt     runtimeversions.Runtime
	listed bool
	opts   AirflowPinOptions
}

func newRuntimeBuildEdit(build, python string, opts AirflowPinOptions) (*runtimeBuildEdit, error) {
	e := &runtimeBuildEdit{build: strings.TrimSpace(build), python: python, opts: opts}
	tag, ok := manifest.ParseRuntimeTag(e.build)
	if !ok || !tag.Build {
		return nil, fmt.Errorf("%w: %q is not one build like 3.3-10 or 13.11.0", ErrInvalidRuntimeBuild, e.build)
	}
	e.tag = tag
	if opts.Catalog != nil {
		e.rt, e.listed = opts.Catalog.Runtime(e.build)
		e.listed = e.listed && e.rt.AirflowVersion != ""
	}
	switch {
	case python == "":
	case !pythonMinorRe.MatchString(python):
		return nil, fmt.Errorf("%w: %q is not a Python version like 3.13", ErrInvalidPython, python)
	case tag.Major == "2":
		return nil, fmt.Errorf("%w: an Airflow 2 runtime runs the Python its image ships", ErrInvalidPython)
	case e.listed && len(e.rt.PythonVersions) > 0 && !slices.Contains(e.rt.PythonVersions, python):
		return nil, fmt.Errorf("%w: runtime %s ships Python %s", ErrInvalidPython, e.build, strings.Join(e.rt.PythonVersions, ", "))
	}
	return e, nil
}

// apply is the edit itself.
func (e *runtimeBuildEdit) apply(before *manifest.Manifest, ed tomledit.Editor) (RuntimeBuildChange, error) {
	if before.Astro.Dockerfile != "" {
		return RuntimeBuildChange{}, fmt.Errorf("this project declares %s in [tool.astro] dockerfile, so its FROM line picks the runtime. Change the FROM line instead", before.Astro.Dockerfile)
	}
	out := RuntimeBuildChange{Previous: before.Astro.Runtime, Runtime: e.build, AirflowVersion: e.rt.AirflowVersion}
	current := before.Airflow().Pin
	if before.AirflowUnclear() {
		current = ""
	}
	pin, err := runtimeAirflowPin(current, e.build, e.tag, e.rt.AirflowVersion, e.opts.Catalog != nil)
	if err != nil {
		return RuntimeBuildChange{}, err
	}
	rp := before.Project.RequiresPython
	if pin != current {
		change, err := setAirflowVersion(before, ed, pin, e.opts)
		if err != nil {
			return RuntimeBuildChange{}, err
		}
		// The runtime line it moved or deleted is written below.
		change.Runtime, change.RuntimeRemoved = "", false
		out.Airflow = &change
		if change.RequiresPython != "" {
			rp = change.RequiresPython
		}
	}
	if e.build != before.Astro.Runtime || out.Airflow != nil {
		if err := ed.Set(runtimePath, e.build); err != nil {
			return RuntimeBuildChange{}, err
		}
	}
	next, python, err := e.requiresPython(rp, pin)
	if err != nil {
		return RuntimeBuildChange{}, err
	}
	out.Python = python
	if out.Airflow != nil {
		// The result's own RequiresPython says what was written, which a
		// python given overrides; the pin change's would contradict it.
		out.Airflow.RequiresPython = ""
	}
	if next != rp {
		if err := ed.Set([]string{"project", "requires-python"}, next); err != nil {
			return RuntimeBuildChange{}, err
		}
	}
	if next != before.Project.RequiresPython {
		out.RequiresPython = next
	}
	out.Changed = e.build != before.Astro.Runtime || out.Airflow != nil || out.RequiresPython != ""
	return out, nil
}

// requiresPython is the requires-python to write beside the build, given rp,
// the one the manifest holds after any pin move, and the Python the build's
// image then runs, "" when the catalog cannot tell.
func (e *runtimeBuildEdit) requiresPython(rp, pin string) (next, python string, err error) {
	next = rp
	def := strings.TrimSpace(e.rt.DefaultPythonVersion)
	switch {
	case e.python != "":
		next = "==" + e.python + ".*"
	case e.listed && onePythonRe.MatchString(rp):
		if p, decided := e.rt.ImagePython(rp); decided && p != def {
			next = pythonBoundFor(pin, e.opts.Catalog)
		}
	}
	if !e.listed {
		return next, "", nil
	}
	p, decided := e.rt.ImagePython(next)
	switch {
	case !decided:
		return next, def, nil
	case p == "":
		return "", "", fmt.Errorf("%w: requires-python %s admits none of the Pythons runtime %s ships (%s). Pass --python to pick one",
			ErrInvalidPython, next, e.build, strings.Join(e.rt.PythonVersions, ", "))
	}
	return next, p, nil
}

// runtimeAirflowPin is the Airflow pin a project moves to beside build: its
// current pin when that covers the Airflow the build carries, else the build's
// exact Airflow when the pin was exact, else the build's series. carried is ""
// when the catalog does not list the build; an Airflow 3 tag then names its
// series, and an Airflow 2 tag names none, so it is refused.
func runtimeAirflowPin(current, build string, tag manifest.RuntimeTag, carried string, haveCatalog bool) (string, error) {
	if carried == "" {
		if tag.Major == "2" {
			if !haveCatalog {
				return "", fmt.Errorf("the runtime catalog could not be read, so which Airflow runtime %s carries cannot be told. Try again online", build)
			}
			return "", fmt.Errorf("the runtime catalog lists no build %s, so which Airflow it carries cannot be told", build)
		}
		if current != "" && tag.Agrees(current) {
			return current, nil
		}
		return tag.Series, nil
	}
	if current != "" && runtimeversions.PinCovers(current, carried) {
		return current, nil
	}
	if strings.Count(current, ".") == 2 {
		return carried, nil
	}
	major, rest, _ := strings.Cut(carried, ".")
	minor, _, _ := strings.Cut(rest, ".")
	return major + "." + minor, nil
}
