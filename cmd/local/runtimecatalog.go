package local

import (
	"context"
	"time"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// initCatalogTimeout bounds init's catalog fetch. Init is interactive and has a
// fallback ready, so it waits seconds, not the fifteen an Airflow 2 image
// lookup gets.
const initCatalogTimeout = 3 * time.Second

// catalogDefault is the production Deps.AirflowDefault: the runtime catalog's
// newest supported series from a fresh cache, a fetch, a stale cache, or the
// built-in fallback, in that order.
func catalogDefault(ctx context.Context) (series, requiresPython string, src runtimeversions.Source) {
	return runtimeversions.Default(ctx, runtimecatalog.Options(initCatalogTimeout))
}
