package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
)

// ErrNoRuntimeForAirflow reports an Airflow version no published runtime
// carries. Callers branch with errors.Is.
var ErrNoRuntimeForAirflow = errors.New("no Astro Runtime carries this Airflow version")

// LocalRuntimeImageWith picks the base image local Docker mode builds FROM.
// Airflow 3 resolves exactly as the deploy path does. Airflow 2 is looked up in
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
// This is the local start path only. The deploy path stays on RuntimeImageFor,
// which is Airflow 3 alone.
func LocalRuntimeImageWith(ctx context.Context, airflowVersion, runtime string, o runtimeversions.Options) (string, error) {
	v := strings.TrimSpace(airflowVersion)
	if v == "" {
		return "", errors.New("no Airflow version was given; one is needed to pick a runtime image")
	}
	runtime = strings.TrimSpace(runtime)
	switch major, _, _ := strings.Cut(v, "."); major {
	case "3":
		return RuntimeImageFor(v, runtime)
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
	return fmt.Errorf("Docker mode needs Airflow 2.%d or later, and this project pins %s: the metadata database is migrated with `airflow db migrate`, which does not exist before Airflow 2.%d%s",
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
