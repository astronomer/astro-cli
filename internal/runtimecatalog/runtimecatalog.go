// Package runtimecatalog is how this CLI reads Astronomer's runtime catalog
// (pkg/runtimeversions): where the copy is cached and what the request says
// about who is asking. One place, because astro init, the Docker-mode image
// lookup, and the runtime-build check that start, deploy and package run all
// read the same catalog, and should read it the same way.
package runtimecatalog

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/version"
)

// Options is how this CLI reads the catalog: cached in runtime-versions.json
// under the cache root it shares with Astro Desktop, so one fetch warms both,
// and named in the request as astro-cli/<version>. The User-Agent is the only
// thing the request says about who is asking.
//
// A cache root that cannot be resolved is not a failure: an empty directory
// means every lookup asks the catalog, which still works. A zero timeout means
// the catalog's default.
func Options(timeout time.Duration) runtimeversions.Options {
	dir, err := localrt.CacheRoot()
	if err != nil {
		dir = ""
	}
	return runtimeversions.Options{
		CacheDir:  dir,
		Timeout:   timeout,
		UserAgent: "astro-cli/" + version.Current(),
	}
}

// astroBuildTimeout bounds each request of an AstroBuild lookup. It runs
// before every standalone start and after init, up to three requests in a
// row, and a network that drops packets should cost seconds, not a minute:
// offline, the start goes on with the manifest as it stands.
const astroBuildTimeout = 4 * time.Second

// AstroBuild looks up the Astronomer build of Airflow a deployment of this pin
// and [tool.astro] runtime build runs, as of the UV_EXCLUDE_NEWER cutoff when
// one is set, so the build agrees with everything else uv resolves then.
func AstroBuild(ctx context.Context, airflowPin, runtime string) (runtimeversions.AstroBuild, error) {
	return runtimeversions.LookupAstroBuild(ctx, Options(astroBuildTimeout), airflowPin, runtime,
		runtimeversions.ParseExcludeNewer(os.Getenv("UV_EXCLUDE_NEWER")))
}

// CheckRuntime checks a manifest's [tool.astro] runtime against its Airflow
// pin with the catalog, as runtimeversions.CheckRuntime does, at the default
// timeout: whatever calls it is about to build an image, which takes longer
// than the wait. It never fails for want of the catalog, and an empty runtime
// costs nothing.
func CheckRuntime(ctx context.Context, runtime, airflowPin string) ([]runtimeversions.Finding, error) {
	return runtimeversions.CheckRuntime(ctx, Options(0), runtime, airflowPin)
}

// catalogTimeout bounds Catalog's fetch. Every caller has a fallback for a
// missing catalog, so it waits seconds, as init does.
const catalogTimeout = 3 * time.Second

// ImageCatalog reads the catalog for a deploy or `astro package astro` about
// to build a generated Airflow 2 image, at the catalog's default timeout: an
// Airflow 2 base with no [tool.astro] runtime is named only by the catalog,
// and has no fallback, and the build it precedes takes far longer than the
// wait. nil when no fresh cache, fetch or stale cache works, as Catalog.
func ImageCatalog(ctx context.Context) *runtimeversions.Catalog {
	c, _, err := runtimeversions.Load(ctx, Options(0))
	if err != nil {
		return nil
	}
	return c
}

// ForBuild is the catalog reader for an image build of this Airflow pin:
// ImageCatalog for Airflow 2, whose base needs the catalog, and Catalog for
// anything else, which only picks a Python and has the runtime's default to
// fall back on, so an offline build waits seconds rather than the default
// timeout.
func ForBuild(airflowPin string) func(context.Context) *runtimeversions.Catalog {
	if pin := strings.TrimSpace(airflowPin); pin == "2" || strings.HasPrefix(pin, "2.") {
		return ImageCatalog
	}
	return Catalog
}

// ForDockerfileCheck is Catalog for scaffold.CheckDockerfileAirflowWith, which
// calls it only for a declared Dockerfile on one Airflow 2 build, whose Airflow
// the tag does not name. The check's callers run before any request context
// exists, so it reads under its own timeout, as Catalog does.
func ForDockerfileCheck() *runtimeversions.Catalog {
	return Catalog(context.Background())
}

// Catalog reads the catalog for an edit that moves the Airflow pin, and for a
// standalone plan choosing its Python: a fresh cache, a fetch, a stale cache,
// in that order. nil when none of those works, which the edit takes as "no
// catalog" and the plan as the built-in Python rule, rather than a failure.
// A generated image build reads it through ImageCatalog.
func Catalog(ctx context.Context) *runtimeversions.Catalog {
	c, _, err := runtimeversions.Load(ctx, Options(catalogTimeout))
	if err != nil {
		return nil
	}
	return c
}
