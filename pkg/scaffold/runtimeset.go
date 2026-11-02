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
var ErrInvalidRuntimeBuild = errors.New("not an Astro Runtime version")

// ErrInvalidPython reports a Python SetRuntimeBuildWith cannot set: not a
// minor like 3.13, one the build does not ship, or any Python for an Airflow 2
// build, whose image runs its own.
var ErrInvalidPython = errors.New("not a Python this runtime version runs")

// pythonMinorRe is a Python minor version, as a caller names one.
var pythonMinorRe = regexp.MustCompile(`^3\.\d+$`)

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
//     Which Airflow a build carries comes from opts.Catalog. A build it does
//     not list is taken within the requirement's series (it may be newer than
//     the copy read), and refused outside it. Without a catalog an Airflow 3
//     build's tag names its series, and an Airflow 2 build is refused, since
//     its tag names none.
//   - python, when given, is the Python the image and the standalone venv
//     run: requires-python becomes "==python.*". It has to be one the build
//     ships, when the catalog lists the build. Without it requires-python
//     stays as it is: a range runs the build's default when it admits it, and
//     a single Python (the shape python writes) stays chosen when the build
//     ships it. One that admits no Python the build ships is refused, naming
//     the ones it does, since only python moves the project to another.
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
		return nil, fmt.Errorf("%w: %q is not one runtime version like 3.3-10 or 13.11.0", ErrInvalidRuntimeBuild, e.build)
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
		return RuntimeBuildChange{}, DockerfilePicksRuntime(before.Astro.Dockerfile)
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
	next, python, err := e.requiresPython(rp)
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

// DockerfilePicksRuntime is why a project that declares dockerfile, a path,
// takes no runtime build: its Dockerfile's FROM line picks the image.
func DockerfilePicksRuntime(dockerfile string) error {
	return fmt.Errorf("this project declares %s in [tool.astro] dockerfile, so its FROM line picks the runtime. Change the FROM line instead", dockerfile)
}

// requiresPython is the requires-python to write beside the build, given rp,
// the one the manifest holds after any pin move, and the Python the build's
// image then runs, "" when the catalog cannot tell.
func (e *runtimeBuildEdit) requiresPython(rp string) (next, python string, err error) {
	next = rp
	if e.python != "" {
		next = "==" + e.python + ".*"
	}
	if !e.listed {
		return next, "", nil
	}
	python, err = RuntimePython(&e.rt, next, e.build)
	if err != nil {
		return "", "", fmt.Errorf("%w. Pass --python to pick one", err)
	}
	return next, python, nil
}

// RuntimePython is the Python the image of build, whose catalog entry is rt,
// runs for requiresPython: the build's default when requires-python admits it
// or does not decide, else the newest it ships that requires-python admits.
// One that admits none of them is ErrInvalidPython, naming the ones it ships.
func RuntimePython(rt *runtimeversions.Runtime, requiresPython, build string) (string, error) {
	p, decided := rt.ImagePython(requiresPython)
	switch {
	case !decided:
		return strings.TrimSpace(rt.DefaultPythonVersion), nil
	case p == "":
		return "", fmt.Errorf("%w: requires-python %s admits none of the Pythons runtime %s ships (%s)",
			ErrInvalidPython, requiresPython, build, strings.Join(rt.PythonVersions, ", "))
	}
	return p, nil
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
				return "", fmt.Errorf("the runtime catalog could not be read, so which Airflow runtime %s ships cannot be told. Try again online", build)
			}
			return "", fmt.Errorf("the runtime catalog lists no runtime version %s, so which Airflow it ships cannot be told", build)
		}
		if current != "" && tag.Agrees(current) {
			return current, nil
		}
		// Moving the requirement on an unlisted build's word is a guess the
		// catalog could have answered: refused when it was read, and made
		// from the tag only offline.
		if haveCatalog {
			return "", fmt.Errorf("the runtime catalog lists no runtime version %s, so the Airflow requirement is not moved to Airflow %s for it. "+
				"Check the version, or move the requirement first with astro local upgrade airflow", build, tag.Series)
		}
		return tag.Series, nil
	}
	if current != "" && runtimeversions.PinCovers(current, carried) {
		return current, nil
	}
	if (manifest.Airflow{Pin: current}).Exact() {
		return carried, nil
	}
	major, rest, _ := strings.Cut(carried, ".")
	minor, _, _ := strings.Cut(rest, ".")
	return major + "." + minor, nil
}
