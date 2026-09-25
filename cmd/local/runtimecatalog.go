package local

import (
	"context"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/version"
)

// catalogOptions is how this CLI reads Astronomer's runtime catalog: cached in
// runtime-versions.json under the cache root it shares with Astro Desktop, so
// one fetch warms both, and named in the request as astro-cli/<version>. The
// User-Agent is the only thing the request says about who is asking.
//
// A cache root that cannot be resolved is not a failure: an empty directory
// means every lookup asks the catalog, which still works. A zero timeout means
// the catalog's default.
func catalogOptions(timeout time.Duration) runtimeversions.Options {
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

// initCatalogTimeout bounds init's catalog fetch. Init is interactive and has a
// fallback ready, so it waits seconds, not the fifteen an Airflow 2 image
// lookup gets.
const initCatalogTimeout = 3 * time.Second

// catalogDefault is the production Deps.AirflowDefault: the runtime catalog's
// newest supported series from a fresh cache, a fetch, a stale cache, or the
// built-in fallback, in that order.
func catalogDefault(ctx context.Context) (series, requiresPython string, src runtimeversions.Source) {
	return runtimeversions.Default(ctx, catalogOptions(initCatalogTimeout))
}
