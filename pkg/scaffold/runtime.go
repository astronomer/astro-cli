package scaffold

import (
	"errors"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// runtimePath is [tool.astro] runtime, the one Astro Runtime build a project's
// image uses. Neither init nor a conversion writes it; it is the user's
// choice, and these edits only keep it loadable or change it on request.
var runtimePath = []string{"tool", "astro", "runtime"}

// RuntimeChange is what AlignRuntime or SetRuntime did to [tool.astro] runtime.
type RuntimeChange struct {
	// Previous is the runtime the manifest named, "" when it named none.
	Previous string `json:"previous,omitempty"`
	// Runtime is the runtime now written, "" when there is none.
	Runtime string `json:"runtime,omitempty"`
	// Removed reports that the line was deleted.
	Removed bool `json:"removed,omitempty"`
	// Changed reports that the file was written.
	Changed bool `json:"changed"`
}

// alignRuntime keeps [tool.astro] runtime loadable beside the Airflow pin an
// edit is writing, and reports what it did.
//
//   - A build that agrees with pin is left alone. Without a catalog that is as
//     far as the tag can tell: pin's series for an Airflow 3 build, its
//     generation for an Airflow 2 one. With a catalog, the Airflow the build
//     carries has to be one pin covers as well, so an Airflow 2 build of
//     another series, or a build an exact pin excludes, does not stay to fail or
//     warn at the next image build. A build the catalog does not list is judged
//     by its tag.
//   - Beside a declared dockerfile the line picks nothing, and it is deleted.
//   - Otherwise the build no longer agrees with the requirement, or was never a
//     build. It moves to the newest non-yanked build carrying pin when catalog
//     names one, and is deleted when not: a runtime this package cannot vouch
//     for is worse than none, which builds from the newest build of the series.
//
// It never adds a line that was not there.
func alignRuntime(before *manifest.Manifest, ed tomledit.Editor, pin string, catalog *runtimeversions.Catalog) (moved string, removed bool, err error) {
	current := before.Astro.Runtime
	if current == "" {
		return "", false, nil
	}
	if before.Astro.Dockerfile == "" {
		if t, ok := manifest.ParseRuntimeTag(current); ok && t.Build && t.Agrees(pin) && !catalogExcludes(catalog, current, pin) {
			return "", false, nil
		}
		if catalog != nil {
			if next, ok := catalog.NewestRuntimeFor(pin); ok {
				if err := ed.Set(runtimePath, next); err != nil {
					return "", false, err
				}
				return next, false, nil
			}
		}
	}
	return "", ed.Delete(runtimePath), nil
}

// catalogExcludes reports that the catalog says the build carries an Airflow
// pin does not cover, which the tag alone cannot show: another series for an
// Airflow 2 build, or another patch under an exact pin. With no catalog, or a
// build it does not list, nothing is known beyond the tag.
func catalogExcludes(catalog *runtimeversions.Catalog, runtime, pin string) bool {
	if catalog == nil {
		return false
	}
	for _, f := range catalog.CheckRuntime(runtime, pin) {
		if f.Kind == runtimeversions.FindingAirflowExcluded || f.Kind == runtimeversions.FindingSeriesMismatch {
			return true
		}
	}
	return false
}

// errAirflowUnclear refuses a runtime edit on a manifest whose requirement
// states no single Airflow, which a build cannot be checked against.
var errAirflowUnclear = errors.New("the Airflow requirement in [project] dependencies does not state one version, so no runtime build can be checked against it. Pin the requirement first")

// AlignRuntime repairs [tool.astro] runtime, through EditManifest: the fix for
// manifest.CodeRuntimeMismatch, CodeRuntimeInvalid and
// CodeRuntimeWithDockerfile, which the desktop's attention card offers. A build
// that agrees with the requirement (with opts.Catalog, by the Airflow it
// carries too) is left alone; one beside a declared
// dockerfile is deleted; any other moves to the newest non-yanked build
// carrying the requirement's pin when opts.Catalog names one, and is deleted
// when not. The requirement itself never changes: moving it to the build's
// series is SetAirflowVersionWith, the caller's to offer.
//
// A manifest with no runtime line is left as it is. With opts.DryRun the
// change is reported and nothing is written.
func AlignRuntime(dir string, wrap func(run func() error) error, opts AirflowPinOptions) (RuntimeChange, error) {
	var out RuntimeChange
	err := editManifestFor(opts.DryRun, dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		out = RuntimeChange{Previous: before.Astro.Runtime, Runtime: before.Astro.Runtime}
		if out.Previous == "" {
			return nil
		}
		pin := before.Airflow().Pin
		if pin == "" || before.AirflowUnclear() {
			return errAirflowUnclear
		}
		moved, removed, err := alignRuntime(before, ed, pin, opts.Catalog)
		if err != nil {
			return err
		}
		switch {
		case removed:
			out.Runtime, out.Removed, out.Changed = "", true, true
		case moved != "" && moved != out.Previous:
			out.Runtime, out.Changed = moved, true
		}
		return nil
	})
	if err != nil {
		return RuntimeChange{}, err
	}
	return out, nil
}

// SetRuntime writes [tool.astro] runtime, through EditManifest, or deletes it
// when runtime is empty: the edit behind choosing a newer build of the same
// series, which the desktop's upgrade offer makes. The result is held to the
// manifest's own rules, so a build of another series than the requirement, a
// value that is not a build, or a runtime beside a declared dockerfile is
// refused (ErrEditRefused, wrapping the manifest.ValidationError that names
// it) and nothing is written. Whether the catalog lists the build, and whether
// it is yanked, is checked where an image is built, not here.
func SetRuntime(dir string, wrap func(run func() error) error, runtime string) (RuntimeChange, error) {
	var out RuntimeChange
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		out = RuntimeChange{Previous: before.Astro.Runtime, Runtime: runtime}
		if runtime == "" {
			out.Removed = ed.Delete(runtimePath)
			out.Changed = out.Removed
			return nil
		}
		if runtime == out.Previous {
			return nil
		}
		out.Changed = true
		return ed.Set(runtimePath, runtime)
	})
	if err != nil {
		return RuntimeChange{}, err
	}
	return out, nil
}
