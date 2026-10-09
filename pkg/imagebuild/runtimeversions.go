package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// Airflow 3 ships as Astro Runtime 3, whose tag is the Airflow version, so
// RuntimeImage builds that reference from the pin alone. Airflow 2 does not:
// its images are tagged by runtime version (11.12.0), and the Airflow release
// each one carries is only knowable from Astronomer's runtime catalog. So an
// Airflow 2 base image needs a lookup, and a lookup needs the network — which
// is why local Docker mode resolves through here and the pure RuntimeImage
// stays as it is. The catalog itself, its cache and its fallback to a stale
// copy belong to pkg/runtimeversions.
//
// The Airflow 2 images answer to the same ONBUILD contract the build already
// relies on: COPY packages.txt, install-system-packages, COPY requirements.txt,
// install-python-dependencies. Nothing else about the build changes.

const (
	// airflow2ImageRepo hosts the Astro Runtime images for Airflow 2.
	airflow2ImageRepo = "quay.io/astronomer/astro-runtime"
	// minAirflow2Minor is the oldest Airflow 2 local Docker mode runs. The
	// compose file's database service runs `airflow db migrate`, which arrived
	// in Airflow 2.7 and replaced `db upgrade`; below that the service exits 2
	// and every component then waits forever on a migration that never
	// completed. Everything under the floor is Astro Runtime 8 or older, out of
	// maintenance since 2023, so this refuses nothing that is still supported.
	minAirflow2Minor = 7
	// pythonCatalogTimeout bounds the catalog fetch an Airflow 3 start makes to
	// pick the image's Python. That choice has a fallback (the runtime's
	// default Python), so it waits seconds, as the CLI's runtimecatalog.Catalog
	// does for deploy and package.
	pythonCatalogTimeout = 3 * time.Second
)

// ErrNoRuntimeForAirflow reports an Airflow version no published runtime
// carries. Callers branch with errors.Is.
var ErrNoRuntimeForAirflow = errors.New("no Astro Runtime carries this Airflow version")

// LocalRuntimeImageWith picks the base image local Docker mode builds FROM.
// Airflow 3 resolves exactly as the deploy path does, requires-python included
// (RuntimeImageForPython). Airflow 2 is looked up in
// the runtime catalog, and the newest runtime carrying that Airflow wins, down
// to the 2.7 floor. Yanked runtimes are skipped.
//
// o says where the catalog is cached, how long a fetch may take, and what the
// request's User-Agent says. The caller names all three: this package has no
// business knowing where a consumer's cache lives. An empty CacheDir means no
// cache at all — every call fetches, which still works and only costs the
// request.
//
// runtime is the manifest's [tool.astro] runtime, or "" for none. When set, the
// base is that build and nothing is looked up: an Airflow 3 build under
// RuntimeImageRepo, as RuntimeImageFor names it, and an Airflow 2 one under
// quay.io/astronomer/astro-runtime. The pin is still checked, the 2.7 floor
// included, because the manifest has already held the build to the pin's
// generation and series as far as the tag shows. What the tag cannot show
// (the catalog's word on an Airflow 2 build's series, a yanked build) is
// runtimeversions.CheckRuntime's, which the caller runs where it can say so.
//
// This is the local start path only. The deploy path stays on
// RuntimeImageForPython, which is Airflow 3 alone.
func LocalRuntimeImageWith(ctx context.Context, airflowVersion, runtime, requiresPython string, o runtimeversions.Options) (string, error) {
	v := strings.TrimSpace(airflowVersion)
	if v == "" {
		return "", errors.New("no Airflow version was given; one is needed to pick a runtime image")
	}
	runtime = strings.TrimSpace(runtime)
	switch major, _, _ := strings.Cut(v, "."); major {
	case "3":
		return RuntimeImageForPython(v, runtime, requiresPython, func() *runtimeversions.Catalog {
			// Unlike the Airflow 2 lookup below, a catalog that cannot be read
			// here only means the default Python, so the wait is short.
			o := o
			if o.Timeout <= 0 || o.Timeout > pythonCatalogTimeout {
				o.Timeout = pythonCatalogTimeout
			}
			catalog, _, err := runtimeversions.Load(ctx, o)
			if err != nil {
				return nil
			}
			return catalog
		})
	case "2":
		if err := checkAirflow2Floor(v); err != nil {
			return "", err
		}
		if runtime != "" {
			return airflow2ImageRepo + ":" + runtime, nil
		}
		catalog, _, err := runtimeversions.Load(ctx, o)
		if err != nil {
			return "", err
		}
		runtime, ok := catalog.NewestRuntimeFor(v)
		if !ok {
			return "", fmt.Errorf("%w: %s%s", ErrNoRuntimeForAirflow, v, seriesHint(v))
		}
		return airflow2ImageRepo + ":" + runtime, nil
	default:
		return "", fmt.Errorf("Docker mode runs Airflow 2 or Airflow 3, not %q", v)
	}
}

// checkAirflow2Floor refuses an Airflow 2 pin older than Docker mode runs,
// before the lookup, so the answer is a sentence rather than a database service
// that exits 2 in a container. A pin that names no minor ("2") is not refused:
// it means the newest Airflow 2, which is well above the floor.
func checkAirflow2Floor(version string) error {
	_, rest, found := strings.Cut(version, ".")
	if !found {
		return nil
	}
	minorText, _, _ := strings.Cut(rest, ".")
	minor, err := strconv.Atoi(minorText)
	if err != nil || minor >= minAirflow2Minor {
		return nil
	}
	return fmt.Errorf("Docker mode needs Airflow 2.%d or later, and this project pins %s: the metadata database is migrated with airflow db migrate, which does not exist before Airflow 2.%d%s",
		minAirflow2Minor, version, minAirflow2Minor, seriesHint(version))
}

// seriesHint is the fix for an exact pin that was likely meant as a series:
// "==2" and "==2.10" name the releases 2.0.0 and 2.10.0 under PEP 440, not
// the newest of a line, and the manifest reads them that way. A pin ending in
// .0 is the one that shape produces, so it is the one given the hint. Empty
// for any other pin.
func seriesHint(pin string) string {
	parts := strings.Split(pin, ".")
	if len(parts) != 3 || parts[2] != "0" {
		return ""
	}
	series := parts[0] + "." + parts[1]
	if parts[1] == "0" {
		series = parts[0]
	}
	return fmt.Sprintf(". %s is exactly Airflow %s; for the newest Airflow %s, pin apache-airflow==%s.*", pin, pin, series, series)
}

// StandalonePython is the interpreter a standalone venv of a manifest asks uv
// for, given its Airflow pin, [tool.astro] runtime ("" for none) and [project]
// requires-python: the Python a generated image of the same manifest runs
// (runtimeversions.ProjectPython), so a project runs one Python in standalone
// mode, in Docker mode and on a deployment. For `requires-python = '>=3.12'`,
// which `astro init` writes, that is the runtime build's default rather than
// whichever compatible interpreter uv finds first. The CLI and Astro Desktop
// both build their venvs from it.
//
// When ProjectPython cannot decide (no catalog, as offline; Airflow 2, whose
// builds list no Python; no requires-python; a specifier it does not read),
// this is airflowrt.PythonFallback, the rule that needs no catalog: "" when
// requires-python is set, so uv picks within it, and a version otherwise.
//
// A requires-python that admits none of the build's Pythons comes back as
// *runtimeversions.PythonNotShippedError beside that fallback, rather than
// instead of it. An image is refused over it, but a venv need not be: uv can
// still satisfy requires-python with another interpreter, and the project may
// well run under it, so the caller warns and builds the venv.
func StandalonePython(airflowVersion, runtime, requiresPython string, catalog func() *runtimeversions.Catalog) (string, error) {
	python, _, err := runtimeversions.ProjectPython(airflowVersion, runtime, requiresPython, catalog)
	if python == "" {
		python = airflowrt.PythonFallback(requiresPython, airflowVersion)
	}
	return python, err
}

// RuntimeImageForPython is RuntimeImageFor for a generated build, which also
// has to run the Python runtimeversions.ProjectPython chooses for the project:
// the one a standalone venv of the same manifest runs too.
//
// A runtime build ships several Pythons, and its tag runs the default one;
// runtime:<build>-python-X.Y runs another. There is no such tag for a series,
// so a Python other than the default needs the exact build, which
// ProjectPython names. When it chooses the default, or has nothing to decide
// on (no catalog, no build of the pin, no Python listed, a requires-python it
// does not read), the base is RuntimeImageFor's, unchanged: an offline build
// runs the default Python, as it did before this looked. A build that ships no
// Python requires-python admits is refused before anything is built.
//
// catalog reads the runtime catalog, and returns nil when it cannot. It is
// called only when requires-python is set.
func RuntimeImageForPython(airflowVersion, runtime, requiresPython string, catalog func() *runtimeversions.Catalog) (string, error) {
	base, err := RuntimeImageFor(airflowVersion, runtime)
	if err != nil {
		return base, err
	}
	python, build, err := runtimeversions.ProjectPython(airflowVersion, runtime, requiresPython, catalog)
	if err != nil {
		return "", fmt.Errorf("%w. Change requires-python to admit one of them, or build from a runtime that ships one it admits", err)
	}
	if python == "" || build == "" {
		return base, nil
	}
	return RuntimeImageRepo + ":" + build + "-python-" + python, nil
}
